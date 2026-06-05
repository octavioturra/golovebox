// Package orchestrator is the workflow engine: it turns parsed spec intents into a
// DAG, executes it (workflow nodes via git/github tools, task nodes via the ReAct
// loop), and drives single-task and GitHub-issue runs. It depends only on core
// (substrate, LLM, run config, progress) plus go-github — never on the app's
// config/web packages. The composition root injects all concrete substrates.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/golovebox/core"
	"github.com/user/golovebox/orchestrator/dag"
	"github.com/user/golovebox/orchestrator/memory"
)

// Engine is the workflow engine. It holds the injected substrate (sandbox, LLM
// completer, memory) and an internal CheckpointManager shared across runs.
type Engine struct {
	sb        core.Sandbox
	completer core.Completer
	memory    *memory.Memory
	cm        *dag.CheckpointManager
}

// New creates an Engine. memory may be nil (semantic search is then a no-op).
func New(sb core.Sandbox, completer core.Completer, mem *memory.Memory) *Engine {
	return &Engine{
		sb:        sb,
		completer: completer,
		memory:    mem,
		cm:        dag.NewCheckpointManager(),
	}
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
// NOT_TODO items are written to tech_debt.md inside rc.WorkDir (if provided).
func (e *Engine) Plan(ctx context.Context, runID string, intents []core.Intent, rc core.RunConfig) (*dag.DAG, error) {
	if len(intents) == 0 {
		return nil, fmt.Errorf("orchestrator: no specs provided")
	}

	// Collect tech debts and write to file.
	var debts []string
	for _, intent := range intents {
		debts = append(debts, intent.TechDebts...)
	}
	if len(debts) > 0 && rc.WorkDir != "" {
		artifactsDir := filepath.Join(rc.WorkDir, "artifacts")
		_ = os.MkdirAll(artifactsDir, 0o755)
		var sb strings.Builder
		sb.WriteString("# Tech Debts (NOT_TODO)\n\n")
		for _, d := range debts {
			sb.WriteString("- " + d + "\n")
		}
		_ = os.WriteFile(filepath.Join(artifactsDir, "tech_debt.md"), []byte(sb.String()), 0o644)
	}

	var repoCtx string
	if rc.Repo != "" {
		clonePath := rc.ClonePath
		if clonePath == "" {
			clonePath = "/root/" + repoPathFromRepo(rc.Repo)
		}
		repoCtx = fmt.Sprintf(
			"- DefaultRepo: %s\n- GitHubToken disponível: %v\n- Repositório pode já estar em %s na VM\n",
			rc.Repo,
			rc.GitHubToken != "",
			clonePath,
		)
	}

	prompt := buildPlanningPrompt(intents, rc, repoCtx)
	reply, err := e.completer.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: llm: %w", err)
	}

	nodes, err := parseNodeDescriptors(reply)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: parse plan: %w", err)
	}

	d := dag.New(runID)

	// Inject sync_repo as the first node when a default repo is configured.
	if rc.Repo != "" {
		syncNode := &dag.Node{
			ID:   "sync_repo",
			Type: dag.TypeSyncRepo,
			Task: fmt.Sprintf("Sincronizar %s em %s", rc.Repo, rc.ClonePath),
		}
		if err := d.AddNode(syncNode); err != nil {
			return nil, fmt.Errorf("orchestrator: add sync_repo: %w", err)
		}
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
		if rc.Repo != "" {
			if len(nd.Dependencies) == 0 {
				n.Dependencies = []string{"sync_repo"}
			}
		}
		if err := d.AddNode(n); err != nil {
			return nil, fmt.Errorf("orchestrator: add node %q: %w", nd.ID, err)
		}
	}
	for _, nd := range nodes {
		if nd.ID == "sync_repo" || dag.NodeType(nd.Type) == dag.TypeSyncRepo {
			continue
		}
		if rc.Repo != "" {
			if len(nd.Dependencies) == 0 {
				if err := d.AddEdge("sync_repo", nd.ID); err != nil {
					return nil, fmt.Errorf("orchestrator: sync_repo edge to %s: %w", nd.ID, err)
				}
			}
		}
		for _, dep := range nd.Dependencies {
			if dep == "sync_repo" {
				continue
			}
			if err := d.AddEdge(dep, nd.ID); err != nil {
				return nil, fmt.Errorf("orchestrator: add edge %s→%s: %w", dep, nd.ID, err)
			}
		}
	}

	return d, nil
}


func buildPlanningPrompt(intents []core.Intent, rc core.RunConfig, repoCtx string) string {
	var sb strings.Builder
	sb.WriteString(`You are an execution planner. Read the specs below and produce a JSON execution plan.

Rules:
- ATTENTION_HERE and PAUSE_TO_REVIEW annotations → node type "checkpoint" (human gate)
- RUN_TEST annotations → node type "gate" (run test suite)
- NOTIFY_ME annotations → node type "notify"
- WHEN/DO annotations → node type "wait_event"
- TRY/OR_ELSE annotations → node type "try_else"
- NOT_TODO items must NOT appear as nodes
- IMPORTANT: Branch creation, git commit, git push, and PR are handled automatically by the system
  before and after the DAG runs. Do NOT include branch, commit, push, or PR nodes in your plan.
  The agent already operates on the correct branch — focus only on the actual work.

- Nodes that are independent of each other must NOT have dependencies between them (they run in parallel)
- Each node needs a clear, actionable "task" string describing exactly what the agent should do
- The "task" field is the FULL prompt the agent receives — it MUST be self-contained. Restate the user's
  original objective and any concrete details (file paths, contents, styling, behavior) needed to
  execute the node correctly. Never assume the agent remembers the broader request from previous nodes.
  BAD:  "Create index.html"
  GOOD: "Create /root/repo/index.html containing: <!DOCTYPE html>, <html>, <head> with charset utf-8,
         <body> with a single <div> styled black (background-color:#000), centered horizontally and
         vertically via CSS flexbox on the body (display:flex; justify-content:center; align-items:center;
         min-height:100vh; margin:0). The div should be ~200x200px."
- Each node "id" MUST be snake_case and describe the action performed (e.g. "create_auth_handler", "run_unit_tests"). NEVER use generic names like "step-1", "step-2", "task-1", "node-1".
- Use DefaultRepo from execution context when available
- A sync_repo step is auto-prepended by the system — do NOT include it in your plan

Return ONLY a JSON array, no markdown fences, no explanation:
[{"id":"string","type":"task|checkpoint|gate|notify|wait_event|try_else","task":"string","dependencies":["id",...],"annotation":"string (optional)"}]

`)

	if repoCtx != "" {
		sb.WriteString("## Execution Context\n\n")
		sb.WriteString(repoCtx)
		sb.WriteString("\n\n")
	}

	if rc.RunMode == "build_only" {
		sb.WriteString(`RESTRIÇÃO run_mode=build_only:
Não inclua nenhum node que execute servidores, processos em background, ou comandos que mantenham
processo rodando (npm run dev, go run, python app.py, docker run, etc.).
Se o spec pedir para "servir" ou "rodar" a aplicação, trate como NOT_TODO.

`)
	}

	for _, intent := range intents {
		sb.WriteString(fmt.Sprintf("## Spec: %s\n\n", intent.Source))
		sb.WriteString(intent.Raw)
		sb.WriteString("\n\nAnnotations in this spec:\n")
		for _, s := range intent.Steps {
			switch s.Kind {
			case core.KindTryElse:
				sb.WriteString(fmt.Sprintf("  - TRY: %s OR_ELSE: %s (line %d)\n", s.Text, s.OrElse, s.Line))
			default:
				sb.WriteString(fmt.Sprintf("  - %s: %s (line %d)\n", s.Kind, s.Text, s.Line))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// parseNodeDescriptors extracts the JSON array from the LLM reply.
func parseNodeDescriptors(reply string) ([]nodeDescriptor, error) {
	s := strings.TrimSpace(reply)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

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
		dag.TypeSyncRepo, dag.TypeCommit:
		return true
	}
	return false
}
