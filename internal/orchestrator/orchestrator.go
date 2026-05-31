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
	} else if o.cfg != nil && o.cfg.Workflow.DefaultRepo != "" {
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
	if o.cfg != nil && o.cfg.Workflow.DefaultRepo != "" {
		syncNode := &dag.Node{
			ID:   "sync_repo",
			Type: dag.TypeSyncRepo,
			Task: fmt.Sprintf("Sincronizar %s em %s", o.cfg.Workflow.DefaultRepo, o.cfg.Workflow.ClonePath),
		}
		if err := d.AddNode(syncNode); err != nil {
			return nil, fmt.Errorf("orchestrator: add sync_repo: %w", err)
		}
		// All LLM nodes will depend on sync_repo — added below.
	}

	for _, nd := range nodes {
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
		if o.cfg != nil && o.cfg.Workflow.DefaultRepo != "" && nd.ID != "sync_repo" {
			if len(nd.Dependencies) == 0 {
				n.Dependencies = []string{"sync_repo"}
			}
		}
		if err := d.AddNode(n); err != nil {
			return nil, fmt.Errorf("orchestrator: add node %q: %w", nd.ID, err)
		}
	}
	for _, nd := range nodes {
		// Add sync_repo edge for root nodes.
		if o.cfg != nil && o.cfg.Workflow.DefaultRepo != "" && nd.ID != "sync_repo" {
			if len(nd.Dependencies) == 0 {
				if err := d.AddEdge("sync_repo", nd.ID); err != nil {
					return nil, fmt.Errorf("orchestrator: sync_repo edge to %s: %w", nd.ID, err)
				}
			}
		}
		for _, dep := range nd.Dependencies {
			if err := d.AddEdge(dep, nd.ID); err != nil {
				return nil, fmt.Errorf("orchestrator: add edge %s→%s: %w", dep, nd.ID, err)
			}
		}
	}

	return d, nil
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
- NEW BRANCH <name> annotations → node type "branch", annotation field = branch name
- PUSH annotations → node type "push"
- PR ["title"] annotations → node type "pr", annotation field = PR title (may be empty)
- NOT_TODO items must NOT appear as nodes
- Nodes that are independent of each other must NOT have dependencies between them (they run in parallel)
- Each node needs a clear, actionable "task" string describing exactly what the agent should do
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
