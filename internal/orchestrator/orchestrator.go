// Package orchestrator converts parsed spec files into a DAG execution plan via LLM.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/golovebox/internal/config"
	"github.com/user/golovebox/internal/dag"
	"github.com/user/golovebox/internal/dsl"
	"github.com/user/golovebox/internal/llm"
	"github.com/user/golovebox/internal/memory"
)

// Orchestrator transforms spec files into a DAG of executable nodes.
type Orchestrator struct {
	llm    *llm.Client
	memory *memory.Memory
	cfg    *config.Config
}

// New creates an Orchestrator.
func New(llmClient *llm.Client, mem *memory.Memory, cfg *config.Config) *Orchestrator {
	return &Orchestrator{llm: llmClient, memory: mem, cfg: cfg}
}

// nodeDescriptor is the JSON schema the LLM is asked to produce.
type nodeDescriptor struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	Task         string   `json:"task"`
	Dependencies []string `json:"dependencies"`
	Annotation   string   `json:"annotation,omitempty"`
}

// Plan sends specs to the LLM and constructs a DAG from the resulting JSON plan.
// NOT_TODO items are written to tech_debt.md inside runDir (if provided).
func (o *Orchestrator) Plan(ctx context.Context, runID string, specs []*dsl.ParsedSpec, runDir string) (*dag.DAG, error) {
	if len(specs) == 0 {
		return nil, fmt.Errorf("orchestrator: no specs provided")
	}

	// Collect tech debts and write to file.
	var debts []string
	for _, s := range specs {
		debts = append(debts, s.TechDebts...)
	}
	if len(debts) > 0 && runDir != "" {
		artifactsDir := filepath.Join(runDir, "artifacts")
		_ = os.MkdirAll(artifactsDir, 0o755)
		var sb strings.Builder
		sb.WriteString("# Tech Debts (NOT_TODO)\n\n")
		for _, d := range debts {
			sb.WriteString("- " + d + "\n")
		}
		_ = os.WriteFile(filepath.Join(artifactsDir, "tech_debt.md"), []byte(sb.String()), 0o644)
	}

	var repoCtx string
	if o.cfg != nil && o.cfg.DefaultRepo != "" {
		repoCtx = fmt.Sprintf(
			"- DefaultRepo: %s\n- GitHubToken disponível: %v\n- Repositório pode já estar em /root/%s na VM\n",
			o.cfg.DefaultRepo,
			o.cfg.GitHubToken != "",
			repoPathFromRepo(o.cfg.DefaultRepo),
		)
	} else if o.cfg != nil && workflowRepo(o.cfg) != "" {
		repoCtx = fmt.Sprintf(
			"- DefaultRepo: %s\n- GitHubToken disponível: %v\n- Repositório pode já estar em %s na VM\n",
			o.cfg.Workflow.DefaultRepo,
			o.cfg.GitHubToken != "",
			o.cfg.Workflow.ClonePath,
		)
	}

	prompt := buildPlanningPrompt(specs, o.cfg, repoCtx)
	reply, err := o.llm.Complete(ctx, []llm.Message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("orchestrator: llm: %w", err)
	}

	nodes, err := parseNodeDescriptors(reply)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: parse plan: %w", err)
	}

	d := dag.New(runID)

	// Inject sync_repo as the first node when a default repo is configured.
	if o.cfg != nil && workflowRepo(o.cfg) != "" {
		syncNode := &dag.Node{
			ID:   "sync_repo",
			Type: dag.TypeSyncRepo,
			Task: fmt.Sprintf("Sincronizar %s em %s", workflowRepo(o.cfg), o.cfg.Workflow.ClonePath),
		}
		if err := d.AddNode(syncNode); err != nil {
			return nil, fmt.Errorf("orchestrator: add sync_repo: %w", err)
		}
		// All LLM nodes will depend on sync_repo — added below.
	}

	for _, nd := range nodes {
		// sync_repo is injected directly above — skip any LLM-generated duplicate.
		if nd.ID == "sync_repo" || dag.NodeType(nd.Type) == dag.TypeSyncRepo {
			continue
		}
		nodeType := dag.NodeType(nd.Type)
		if !validNodeType(nodeType) {
			nodeType = dag.TypeTask
		}
		n := &dag.Node{
			ID:           nd.ID,
			Type:         nodeType,
			Task:         nd.Task,
			Dependencies: nd.Dependencies,
			Annotation:   nd.Annotation,
		}
		// If sync_repo is injected, all root nodes (no dependencies) must wait for it.
		if o.cfg != nil && workflowRepo(o.cfg) != "" {
			if len(nd.Dependencies) == 0 {
				n.Dependencies = []string{"sync_repo"}
			}
		}
		if err := d.AddNode(n); err != nil {
			return nil, fmt.Errorf("orchestrator: add node %q: %w", nd.ID, err)
		}
	}
	for _, nd := range nodes {
		// sync_repo is managed by the orchestrator — skip LLM-generated edges for it.
		if nd.ID == "sync_repo" || dag.NodeType(nd.Type) == dag.TypeSyncRepo {
			continue
		}
		// Add sync_repo edge for root nodes.
		if o.cfg != nil && workflowRepo(o.cfg) != "" {
			if len(nd.Dependencies) == 0 {
				if err := d.AddEdge("sync_repo", nd.ID); err != nil {
					return nil, fmt.Errorf("orchestrator: sync_repo edge to %s: %w", nd.ID, err)
				}
			}
		}
		for _, dep := range nd.Dependencies {
			// Skip edges referencing sync_repo since it's already wired via root-node logic.
			if dep == "sync_repo" {
				continue
			}
			if err := d.AddEdge(dep, nd.ID); err != nil {
				return nil, fmt.Errorf("orchestrator: add edge %s→%s: %w", dep, nd.ID, err)
			}
		}
	}

	enforceWorkflowOrdering(d, specs)

	return d, nil
}

// enforceWorkflowOrdering guarantees that when a spec contains NEW BRANCH / PUSH / PR
// annotations, the DAG ends up with the correct workflow node types AND the right
// dependency chain: sync_repo → branch → edits (parallel) → push → pr.
//
// This is defensive — the LLM is instructed to do this via the prompt, but it
// frequently emits generic `task` nodes named "push_…" or "open_pull_request" with
// missing dependencies. Without this pass the agent loop runs inside those nodes and
// confuses base/head, causing PR 422 errors and files created outside the branch.
func enforceWorkflowOrdering(d *dag.DAG, specs []*dsl.ParsedSpec) {
	// Detect required workflow nodes from spec annotations.
	hasBranchKw, hasPushKw, hasPRKw := false, false, false
	branchName, prTitle := "", ""
	for _, s := range specs {
		for _, a := range s.Annotations {
			switch a.Keyword {
			case dsl.KwNewBranch:
				hasBranchKw = true
				if branchName == "" {
					branchName = a.Argument
				}
			case dsl.KwPush:
				hasPushKw = true
			case dsl.KwPR:
				hasPRKw = true
				if prTitle == "" {
					prTitle = a.Argument
				}
			}
		}
	}
	if !hasBranchKw && !hasPushKw && !hasPRKw {
		return // nothing to enforce
	}

	// Find existing workflow nodes (LLM may already have produced them correctly).
	var branchNode, pushNode, prNode *dag.Node
	for _, n := range d.Nodes {
		switch n.Type {
		case dag.TypeBranch:
			branchNode = n
		case dag.TypePush:
			pushNode = n
		case dag.TypePR:
			prNode = n
		}
	}

	// Upgrade task-typed nodes to workflow types when ID matches by heuristic.
	for _, n := range d.Nodes {
		if n.Type != dag.TypeTask {
			continue
		}
		idLower := strings.ToLower(n.ID)
		if hasBranchKw && branchNode == nil && strings.Contains(idLower, "branch") {
			n.Type = dag.TypeBranch
			if n.Annotation == "" {
				n.Annotation = branchName
			}
			branchNode = n
			continue
		}
		if hasPushKw && pushNode == nil && strings.Contains(idLower, "push") {
			n.Type = dag.TypePush
			pushNode = n
			continue
		}
		if hasPRKw && prNode == nil && (strings.Contains(idLower, "pull_request") ||
			strings.Contains(idLower, "pullrequest") ||
			strings.Contains(idLower, "open_pr") ||
			idLower == "pr" || strings.HasSuffix(idLower, "_pr")) {
			n.Type = dag.TypePR
			if n.Annotation == "" {
				n.Annotation = prTitle
			}
			prNode = n
			continue
		}
	}

	// Inject any still-missing workflow nodes.
	syncRepoExists := d.Nodes["sync_repo"] != nil
	if hasBranchKw && branchNode == nil {
		deps := []string{}
		if syncRepoExists {
			deps = append(deps, "sync_repo")
		}
		branchNode = &dag.Node{
			ID:           "create_branch",
			Type:         dag.TypeBranch,
			Task:         "Create branch " + branchName,
			Annotation:   branchName,
			Dependencies: deps,
		}
		_ = d.AddNode(branchNode)
		if syncRepoExists {
			_ = d.AddEdge("sync_repo", branchNode.ID)
		}
	}
	if hasPushKw && pushNode == nil {
		pushNode = &dag.Node{
			ID:   "push_branch",
			Type: dag.TypePush,
			Task: "Push current branch to remote",
		}
		_ = d.AddNode(pushNode)
	}
	if hasPRKw && prNode == nil {
		prNode = &dag.Node{
			ID:         "open_pr",
			Type:       dag.TypePR,
			Task:       "Open pull request",
			Annotation: prTitle,
		}
		_ = d.AddNode(prNode)
	}

	addDep := func(from, to string) {
		if from == "" || to == "" || from == to {
			return
		}
		toNode := d.Nodes[to]
		fromNode := d.Nodes[from]
		if toNode == nil || fromNode == nil {
			return
		}
		for _, dep := range toNode.Dependencies {
			if dep == from {
				return
			}
		}
		toNode.Dependencies = append(toNode.Dependencies, from)
		_ = d.AddEdge(from, to)
	}

	isWorkflow := func(t dag.NodeType) bool {
		return t == dag.TypeBranch || t == dag.TypePush || t == dag.TypePR || t == dag.TypeSyncRepo
	}

	// All non-workflow nodes (except sync_repo) depend on branch.
	if branchNode != nil {
		for _, n := range d.Nodes {
			if n.ID == branchNode.ID || isWorkflow(n.Type) {
				continue
			}
			addDep(branchNode.ID, n.ID)
		}
	}

	// Push depends on all non-workflow nodes and on branch.
	if pushNode != nil {
		for _, n := range d.Nodes {
			if n.ID == pushNode.ID || isWorkflow(n.Type) {
				continue
			}
			addDep(n.ID, pushNode.ID)
		}
		if branchNode != nil {
			addDep(branchNode.ID, pushNode.ID)
		}
	}

	// PR depends on push (or on branch if no push).
	if prNode != nil {
		switch {
		case pushNode != nil:
			addDep(pushNode.ID, prNode.ID)
		case branchNode != nil:
			addDep(branchNode.ID, prNode.ID)
		}
	}
}

func buildPlanningPrompt(specs []*dsl.ParsedSpec, cfg *config.Config, repoCtx string) string {
	var sb strings.Builder
	sb.WriteString(`You are an execution planner. Read the specs below and produce a JSON execution plan.

Rules:
- ATTENTION_HERE and PAUSE_TO_REVIEW annotations → node type "checkpoint" (human gate)
- RUN_TEST annotations → node type "gate" (run test suite)
- NOTIFY_ME annotations → node type "notify"
- WHEN/DO annotations → node type "wait_event"
- TRY/OR_ELSE annotations → node type "try_else"
- NEW BRANCH <name> annotations → node type "branch", annotation field = branch name (NEVER type "task")
- PUSH annotations → node type "push" (NEVER type "task" — DO NOT instruct the agent to run git push manually)
- PR ["title"] annotations → node type "pr", annotation field = PR title (NEVER type "task")
- NOT_TODO items must NOT appear as nodes

CRITICAL ORDERING when these workflow annotations exist:
  branch → <all edit/test/notify nodes in parallel between themselves> → push → pr
  (a sync_repo step is auto-prepended by the system — do NOT include it in your plan)
  - Every edit/test node MUST depend on the branch node (do not create files outside the new branch)
  - The push node MUST depend on every edit/test node
  - The pr node MUST depend on the push node
  - DO NOT use generic "task" type to represent push or pr — use the workflow types so base/head are handled correctly by the backend (otherwise PR creation fails with 422)

BAD example (file edit running in parallel with branch creation — file ends up on the wrong branch):
  [{"id":"create_branch","type":"branch","dependencies":[]},
   {"id":"create_index_html","type":"task","dependencies":[]}]

GOOD example:
  [{"id":"create_branch","type":"branch","annotation":"feature/x","dependencies":[]},
   {"id":"create_index_html","type":"task","dependencies":["create_branch"]},
   {"id":"push_branch","type":"push","dependencies":["create_index_html"]},
   {"id":"open_pr","type":"pr","annotation":"feat: x","dependencies":["push_branch"]}]
- Nodes that are independent of each other must NOT have dependencies between them (they run in parallel)
- Each node needs a clear, actionable "task" string describing exactly what the agent should do
- The "task" field is the FULL prompt the agent receives — it MUST be self-contained. Restate the user's
  original objective and any concrete details (file paths, contents, styling, behavior) needed to
  execute the node correctly. Never assume the agent remembers the broader request from previous nodes.
  BAD:  "Push the branch to the remote repository"
  GOOD: "Push the current feature branch to origin. Context: this is part of delivering the user's
         request 'Crie /root/repo/index.html com uma DIV preta centralizada via CSS flexbox'."
  BAD:  "Create index.html"
  GOOD: "Create /root/repo/index.html containing: <!DOCTYPE html>, <html>, <head> with charset utf-8,
         <body> with a single <div> styled black (background-color:#000), centered horizontally and
         vertically via CSS flexbox on the body (display:flex; justify-content:center; align-items:center;
         min-height:100vh; margin:0). The div should be ~200x200px."
- Each node "id" MUST be snake_case and describe the action performed (e.g. "create_auth_handler", "run_unit_tests", "open_pull_request"). NEVER use generic names like "step-1", "step-2", "task-1", "node-1".
- Tasks involving git MUST include: clone the repo (if not already present), configure remote with token via GIT_ASKPASS, create branch, commit changes, push and open PR
- Use DefaultRepo from execution context when available
- The GitHub token is available via GIT_ASKPASS — the agent shell tool already handles authentication

Return ONLY a JSON array, no markdown fences, no explanation:
[{"id":"string","type":"task|checkpoint|gate|notify|wait_event|try_else|branch|push|pr","task":"string","dependencies":["id",...],"annotation":"string (optional)"}]

`)

	if repoCtx != "" {
		sb.WriteString("## Execution Context\n\n")
		sb.WriteString(repoCtx)
		sb.WriteString("\n\n")
	}

	if cfg != nil && cfg.Workflow.RunMode == "build_only" {
		sb.WriteString(`RESTRIÇÃO run_mode=build_only:
Não inclua nenhum node que execute servidores, processos em background, ou comandos que mantenham
processo rodando (npm run dev, go run, python app.py, docker run, etc.).
Se o spec pedir para "servir" ou "rodar" a aplicação, trate como NOT_TODO.

`)
	}

	for _, s := range specs {
		sb.WriteString(fmt.Sprintf("## Spec: %s\n\n", s.FilePath))
		sb.WriteString(s.Content)
		sb.WriteString("\n\nAnnotations in this spec:\n")
		for _, a := range s.Annotations {
			sb.WriteString(fmt.Sprintf("  - %s: %s (line %d)\n", a.Keyword, a.Argument, a.Line))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// parseNodeDescriptors extracts the JSON array from the LLM reply.
func parseNodeDescriptors(reply string) ([]nodeDescriptor, error) {
	// Strip accidental markdown fences.
	s := strings.TrimSpace(reply)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	// Find the JSON array bounds.
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON array found in LLM reply: %.200s", reply)
	}
	s = s[start : end+1]

	var nodes []nodeDescriptor
	if err := json.Unmarshal([]byte(s), &nodes); err != nil {
		return nil, fmt.Errorf("unmarshal: %w (input: %.200s)", err, s)
	}
	return nodes, nil
}

// workflowRepo returns the repo to clone, falling back to top-level DefaultRepo
// when workflow.default_repo is not set — so users with only the top-level
// setting still get sync_repo auto-injection.
func workflowRepo(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if cfg.Workflow.DefaultRepo != "" {
		return cfg.Workflow.DefaultRepo
	}
	return cfg.DefaultRepo
}

func repoPathFromRepo(ownerRepo string) string {
	parts := strings.SplitN(ownerRepo, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ownerRepo
}

func validNodeType(t dag.NodeType) bool {
	switch t {
	case dag.TypeTask, dag.TypeCheckpoint, dag.TypeGate, dag.TypeNotify, dag.TypeWaitEvent, dag.TypeTryElse,
		dag.TypeSyncRepo, dag.TypeBranch, dag.TypePush, dag.TypePR:
		return true
	}
	return false
}
