package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/user/golovebox/core"
	"github.com/user/golovebox/orchestrator/agent"
	"github.com/user/golovebox/orchestrator/dag"
	"github.com/user/golovebox/orchestrator/tools"
)

// MaxIterations re-exports the ReAct loop iteration cap so app-layer UIs (e.g. the
// Telegram handler) can show "iter/N" without importing the agent sub-package.
const MaxIterations = agent.MaxIterations

// Approve resolves a checkpoint/blocked node, letting execution continue.
func (e *Engine) Approve(nodeID string) { e.cm.Approve(nodeID) }

// Reject fails a checkpoint/blocked node with the given reason.
func (e *Engine) Reject(nodeID, reason string) { e.cm.Reject(nodeID, reason) }

// Run executes the DAG to completion. Workflow nodes (sync_repo/branch/push/pr) are
// dispatched directly via the git/github tools; task nodes run through the ReAct loop,
// emitting core.Progress per iteration. notify observes node state transitions
// (SSE/CLI). dag.json/node_states.json/run_meta.json are persisted atomically in
// rc.WorkDir by the executor.
func (e *Engine) Run(ctx context.Context, d *dag.DAG, rc core.RunConfig, notify dag.NotifyFunc, prog core.Progress) error {
	exec := dag.NewExecutor(d, dag.ExecutorConfig{}, e.dispatch(d, rc, prog), notify, e.cm, rc.WorkDir)
	return exec.Run(ctx)
}

// Resume loads dag.json from rc.WorkDir and re-runs interrupted nodes.
func (e *Engine) Resume(ctx context.Context, rc core.RunConfig, notify dag.NotifyFunc, prog core.Progress) error {
	data, err := os.ReadFile(filepath.Join(rc.WorkDir, "dag.json"))
	if err != nil {
		return fmt.Errorf("orchestrator: read dag.json: %w", err)
	}
	d, err := dag.FromJSON(data)
	if err != nil {
		return fmt.Errorf("orchestrator: parse dag.json: %w", err)
	}
	exec := dag.NewExecutor(d, dag.ExecutorConfig{}, e.dispatch(d, rc, prog), notify, e.cm, rc.WorkDir)
	return exec.Resume(ctx)
}

// RunSingleTask runs the ReAct loop for a free-form task with no surrounding DAG.
// The progress events are tagged with the node ID "task-0".
func (e *Engine) RunSingleTask(ctx context.Context, task string, rc core.RunConfig, prog core.Progress) (string, error) {
	return e.runTask(ctx, "task-0", "", "", task, rc, prog)
}

// RunIssueTask runs the full GitHub-issue flow: fetch the issue, clone the repo,
// index its README into memory, derive a step-by-step task, then run the ReAct loop.
func (e *Engine) RunIssueTask(ctx context.Context, owner, repo string, issueNum int, rc core.RunConfig, prog core.Progress) (string, error) {
	issue, err := tools.GetIssue(ctx, rc.GitHubToken, owner, repo, issueNum)
	if err != nil {
		return "", fmt.Errorf("get issue: %w", err)
	}

	destPath := "/root/" + repo
	if cloneErr := tools.CloneRepo(ctx, e.sb, rc.GitHubToken, owner, repo, destPath); cloneErr != nil {
		return "", fmt.Errorf("clone: %w", cloneErr)
	}
	if e.memory != nil {
		if readme, readErr := e.sb.GetFile(ctx, destPath+"/README.md"); readErr == nil {
			_ = e.memory.Index(ctx, "readme", string(readme))
		}
	}

	task, err := agent.PlanFromIssue(ctx, e.completer, issue, owner, repo)
	if err != nil {
		return "", fmt.Errorf("plan: %w", err)
	}
	return e.runTask(ctx, fmt.Sprintf("issue-%d", issueNum), owner, repo, task, rc, prog)
}

// runTask builds the tool registry for owner/repo and runs the ReAct loop, adapting
// the loop's per-iteration callback into a core.Progress tagged with nodeID.
func (e *Engine) runTask(ctx context.Context, nodeID, owner, repo, task string, rc core.RunConfig, prog core.Progress) (string, error) {
	registry := e.buildRegistry(owner, repo, rc.GitHubToken)
	loop := agent.New(e.completer, registry, e.memory)

	var pf agent.ProgressFunc
	if prog != nil {
		pf = func(iter int, action, params, obs, prompt, reply string) {
			prog(nodeID, iter, action, params, obs, prompt, reply)
		}
	}
	return loop.Run(ctx, task, pf)
}

// dispatch returns the DispatchFunc that routes workflow nodes to the git/github
// tools and task nodes to the ReAct loop.
func (e *Engine) dispatch(d *dag.DAG, rc core.RunConfig, prog core.Progress) dag.DispatchFunc {
	return func(ctx context.Context, node *dag.Node) (string, error) {
		switch node.Type {
		case dag.TypeSyncRepo:
			return tools.SyncRepo(ctx, e.sb, rc.GitHubToken, rc.Repo, rc.ClonePath, rc.DefaultBranch)

		case dag.TypeBranch:
			branchName := node.Annotation
			if branchName == "" {
				branchName = node.Task
			}
			out, err := tools.ExecBranch(ctx, e.sb, rc.ClonePath, branchName)
			if err == nil {
				e.setRunMeta(rc.WorkDir, "current_branch", branchName)
			}
			return out, err

		case dag.TypePush:
			branch := e.getRunMeta(rc.WorkDir, "current_branch")
			return tools.ExecPush(ctx, e.sb, rc.GitHubToken, rc.ClonePath, branch)

		case dag.TypePR:
			parts := strings.SplitN(rc.Repo, "/", 2)
			if len(parts) != 2 {
				return "", fmt.Errorf("repo not set or invalid: %q", rc.Repo)
			}
			owner, repo := parts[0], parts[1]
			branch := e.getRunMeta(rc.WorkDir, "current_branch")
			task := readTask(rc.WorkDir)
			titleParam := node.Annotation
			if titleParam == "" {
				t := task
				if len(t) > 60 {
					t = t[:60] + "..."
				}
				titleParam = "feat: " + t
			}
			body := fmt.Sprintf("## O que foi feito\n\n%s\n\n---\n*Gerado pelo golovebox run `%s`*", task, d.RunID)
			return tools.ExecPR(ctx, rc.GitHubToken, owner, repo, branch, rc.DefaultBranch, titleParam, body)
		}

		// Default: task node runs through the ReAct loop, tagged with the node ID.
		return e.runTask(ctx, node.ID, "", "", node.Task, rc, prog)
	}
}

// buildRegistry creates the agent tool registry for a specific owner/repo, wired to
// the injected sandbox and memory.
func (e *Engine) buildRegistry(owner, repo, token string) *agent.Registry {
	r := agent.NewRegistry()
	sb := e.sb
	mem := e.memory

	r.Register(agent.Tool{
		Name:        "shell",
		Description: "Execute a shell command in the VM",
		Parameters:  map[string]string{"cmd": "shell command to execute"},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			return tools.Shell(fCtx, sb, params["cmd"]), nil
		},
	})

	r.Register(agent.Tool{
		Name:        "read_file",
		Description: "Read a file from the VM",
		Parameters:  map[string]string{"path": "absolute path to the file"},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			data, err := tools.ReadFile(fCtx, sb, params["path"])
			return string(data), err
		},
	})

	r.Register(agent.Tool{
		Name:        "write_file",
		Description: "Write content to a file in the VM",
		Parameters:  map[string]string{"path": "absolute path to the file", "content": "file content"},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			if err := tools.WriteFile(fCtx, sb, params["path"], []byte(params["content"])); err != nil {
				return "", err
			}
			return "file written", nil
		},
	})

	r.Register(agent.Tool{
		Name:        "list_dir",
		Description: "List files in a directory in the VM",
		Parameters:  map[string]string{"path": "absolute path to the directory"},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			entries, err := tools.ListDir(fCtx, sb, params["path"])
			return strings.Join(entries, "\n"), err
		},
	})

	r.Register(agent.Tool{
		Name:        "search_memory",
		Description: "Search the memory store for relevant context",
		Parameters:  map[string]string{"query": "search query text"},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			if mem == nil {
				return "", nil
			}
			results, err := mem.Search(fCtx, params["query"], 3)
			return strings.Join(results, "\n---\n"), err
		},
	})

	r.Register(agent.Tool{
		Name:        "github_list_issues",
		Description: "List open issues in the GitHub repository",
		Parameters:  map[string]string{},
		Execute: func(fCtx context.Context, _ map[string]string) (string, error) {
			issues, err := tools.ListIssues(fCtx, token, owner, repo)
			if err != nil {
				return "", err
			}
			var sb strings.Builder
			for _, iss := range issues {
				sb.WriteString(fmt.Sprintf("#%d: %s\n", iss.GetNumber(), iss.GetTitle()))
			}
			return sb.String(), nil
		},
	})

	r.Register(agent.Tool{
		Name:        "github_get_issue",
		Description: "Get a specific GitHub issue by number",
		Parameters:  map[string]string{"number": "issue number"},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			num, err := strconv.Atoi(params["number"])
			if err != nil {
				return "", fmt.Errorf("invalid issue number: %s", params["number"])
			}
			iss, err := tools.GetIssue(fCtx, token, owner, repo, num)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Title: %s\nBody:\n%s", iss.GetTitle(), iss.GetBody()), nil
		},
	})

	r.Register(agent.Tool{
		Name:        "github_open_pr",
		Description: "Open a Pull Request on GitHub",
		Parameters: map[string]string{
			"head":  "source branch name",
			"base":  "target branch (usually main)",
			"title": "PR title",
			"body":  "PR description",
		},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			// Empty base lets OpenPR resolve the repo's actual default branch.
			return tools.OpenPR(fCtx, token, owner, repo,
				params["head"], params["base"], params["title"], params["body"])
		},
	})

	return r
}

// --- run_meta.json helpers (flat map[string]string, compatible with web.RunStore) ---

func (e *Engine) setRunMeta(workDir, key, value string) {
	if workDir == "" {
		return
	}
	path := filepath.Join(workDir, "run_meta.json")
	meta := readRunMeta(path)
	meta[key] = value
	if data, err := json.Marshal(meta); err == nil {
		_ = atomicWriteFile(path, data)
	}
}

func (e *Engine) getRunMeta(workDir, key string) string {
	if workDir == "" {
		return ""
	}
	return readRunMeta(filepath.Join(workDir, "run_meta.json"))[key]
}

func readRunMeta(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[string]string)
	}
	m := make(map[string]string)
	_ = json.Unmarshal(data, &m)
	return m
}

func atomicWriteFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readTask(workDir string) string {
	data, _ := os.ReadFile(filepath.Join(workDir, "task.txt"))
	return strings.TrimSpace(string(data))
}
