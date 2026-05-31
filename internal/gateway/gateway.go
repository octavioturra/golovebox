// Package gateway wires external triggers (Telegram, future Slack/Email) to the agent loop.
package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/user/golovebox/internal/agent"
	"github.com/user/golovebox/internal/config"
	"github.com/user/golovebox/internal/llm"
	"github.com/user/golovebox/internal/memory"
	"github.com/user/golovebox/internal/sandbox"
	"github.com/user/golovebox/internal/tools"
)

// Handler is implemented by each gateway (Telegram, Slack, Email).
type Handler interface {
	Start(ctx context.Context) error
	Stop() error
}

// Gateway coordinates shared resources and routes incoming tasks to the agent loop.
type Gateway struct {
	pool     *sandbox.Pool
	llm      *llm.Client
	cfg      *config.Config
	handlers []Handler
	mu       sync.Mutex
}

// New creates a Gateway with shared SSH pool, LLM client, and config.
func New(pool *sandbox.Pool, llmClient *llm.Client, cfg *config.Config) *Gateway {
	return &Gateway{pool: pool, llm: llmClient, cfg: cfg}
}

// RegisterHandler adds a handler that will be started by Run.
func (g *Gateway) RegisterHandler(h Handler) {
	g.mu.Lock()
	g.handlers = append(g.handlers, h)
	g.mu.Unlock()
}

// Run starts all registered handlers in goroutines and blocks until ctx is cancelled.
func (g *Gateway) Run(ctx context.Context) error {
	g.mu.Lock()
	handlers := make([]Handler, len(g.handlers))
	copy(handlers, g.handlers)
	g.mu.Unlock()

	if len(handlers) == 0 {
		<-ctx.Done()
		return nil
	}

	var wg sync.WaitGroup
	for _, h := range handlers {
		wg.Add(1)
		go func(h Handler) {
			defer wg.Done()
			if err := h.Start(ctx); err != nil && ctx.Err() == nil {
				slog.Error("gateway handler error", "error", err)
			}
		}(h)
	}
	wg.Wait()
	return nil
}

// Stop calls Stop on every registered handler.
func (g *Gateway) Stop() error {
	g.mu.Lock()
	handlers := make([]Handler, len(g.handlers))
	copy(handlers, g.handlers)
	g.mu.Unlock()

	for _, h := range handlers {
		_ = h.Stop()
	}
	return nil
}

// RunTask executes the full agent workflow for a single GitHub issue:
// GetIssue → CloneRepo → index README → PlanFromIssue → ReAct loop.
// progress is forwarded to agent.Loop.Run; pass nil for silent operation.
func (g *Gateway) RunTask(ctx context.Context, owner, repo string, issueNum int, progress agent.ProgressFunc) (string, error) {
	issue, err := tools.GetIssue(ctx, g.cfg.GitHubToken, owner, repo, issueNum)
	if err != nil {
		return "", fmt.Errorf("get issue: %w", err)
	}

	sc, err := g.pool.Acquire(ctx)
	if err != nil {
		return "", fmt.Errorf("ssh acquire: %w", err)
	}

	destPath := "/root/" + repo
	cloneErr := tools.CloneRepo(sc, g.cfg.GitHubToken, owner, repo, destPath)

	var mem *memory.Memory
	if cloneErr == nil {
		memDir, memDirErr := g.cfg.MemoryDir()
		if memDirErr == nil {
			embFn := memory.NewEmbedFnFromConfig(g.cfg.LLMProvider, g.cfg.LLMBaseURL, g.cfg.APIKey)
			if m, newErr := memory.New(memDir, embFn); newErr == nil {
				mem = m
				if readme, readErr := sandbox.ReadFile(sc, destPath+"/README.md"); readErr == nil {
					_ = mem.Index(ctx, "readme", string(readme))
				}
			}
		}
	}

	g.pool.Release(sc)

	if cloneErr != nil {
		return "", fmt.Errorf("clone: %w", cloneErr)
	}

	registry := g.buildRegistry(owner, repo, mem)

	task, err := agent.PlanFromIssue(ctx, g.llm, issue, owner, repo)
	if err != nil {
		return "", fmt.Errorf("plan: %w", err)
	}

	loop := agent.New(g.llm, registry, mem)
	return loop.Run(ctx, task, progress)
}

// HealthCheckVM acquires a sandbox connection, runs "echo ok", and returns the output.
// Returns an error if the pool is empty, the connection fails, or the command errors.
func (g *Gateway) HealthCheckVM(ctx context.Context) (string, error) {
	sc, err := g.pool.Acquire(ctx)
	if err != nil {
		return "", fmt.Errorf("pool: %w", err)
	}
	defer g.pool.Release(sc)
	out := tools.Shell(sc, "echo ok")
	if !strings.Contains(out, "ok") {
		return "", fmt.Errorf("unexpected output: %q", out)
	}
	return "SSH echo ok", nil
}

// RunSpecTask runs the agent loop for an arbitrary task string.
// This is the general-purpose counterpart to RunTask (which is issue-specific).
// Pass nil for progress to run silently.
func (g *Gateway) RunSpecTask(ctx context.Context, task string, progress agent.ProgressFunc) (string, error) {
	var mem *memory.Memory
	if memDir, err := g.cfg.MemoryDir(); err == nil {
		embFn := memory.NewEmbedFnFromConfig(g.cfg.LLMProvider, g.cfg.LLMBaseURL, g.cfg.APIKey)
		if m, newErr := memory.New(memDir, embFn); newErr == nil {
			mem = m
		}
	}
	registry := g.buildRegistry("", "", mem)
	loop := agent.New(g.llm, registry, mem)
	return loop.Run(ctx, task, progress)
}

// buildRegistry creates the agent tool registry for a specific owner/repo.
// Each tool closure acquires its own SSH connection from the pool.
func (g *Gateway) buildRegistry(owner, repo string, mem *memory.Memory) *agent.Registry {
	r := agent.NewRegistry()

	r.Register(agent.Tool{
		Name:        "shell",
		Description: "Execute a shell command in the VM",
		Parameters:  map[string]string{"cmd": "shell command to execute"},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			sc, err := g.pool.Acquire(fCtx)
			if err != nil {
				return "", err
			}
			defer g.pool.Release(sc)
			return tools.Shell(sc, params["cmd"]), nil
		},
	})

	r.Register(agent.Tool{
		Name:        "read_file",
		Description: "Read a file from the VM",
		Parameters:  map[string]string{"path": "absolute path to the file"},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			sc, err := g.pool.Acquire(fCtx)
			if err != nil {
				return "", err
			}
			defer g.pool.Release(sc)
			data, err := tools.ReadFile(sc, params["path"])
			return string(data), err
		},
	})

	r.Register(agent.Tool{
		Name:        "write_file",
		Description: "Write content to a file in the VM",
		Parameters:  map[string]string{"path": "absolute path to the file", "content": "file content"},
		Execute: func(fCtx context.Context, params map[string]string) (string, error) {
			sc, err := g.pool.Acquire(fCtx)
			if err != nil {
				return "", err
			}
			defer g.pool.Release(sc)
			if err := tools.WriteFile(sc, params["path"], []byte(params["content"])); err != nil {
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
			sc, err := g.pool.Acquire(fCtx)
			if err != nil {
				return "", err
			}
			defer g.pool.Release(sc)
			entries, err := tools.ListDir(sc, params["path"])
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
			issues, err := tools.ListIssues(fCtx, g.cfg.GitHubToken, owner, repo)
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
			iss, err := tools.GetIssue(fCtx, g.cfg.GitHubToken, owner, repo, num)
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
			base := params["base"]
			if base == "" {
				base = "main"
			}
			return tools.OpenPR(fCtx, g.cfg.GitHubToken, owner, repo,
				params["head"], base, params["title"], params["body"])
		},
	})

	return r
}
