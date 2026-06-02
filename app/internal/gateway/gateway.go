// Package gateway wires external triggers (Telegram, future Slack/Email) to the
// orchestrator engine. It owns VM health/readiness and handler registration, and
// delegates all agent execution to the injected *orchestrator.Engine.
package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/user/golovebox/core"
	"github.com/user/golovebox/orchestrator"
)

// Handler is implemented by each gateway (Telegram, Slack, Email).
type Handler interface {
	Start(ctx context.Context) error
	Stop() error
}

// Gateway coordinates shared resources and routes incoming tasks to the engine.
type Gateway struct {
	sb       core.Sandbox
	engine   *orchestrator.Engine
	rc       core.RunConfig
	handlers []Handler
	mu       sync.Mutex
}

// New creates a Gateway with a sandbox, the orchestrator engine, and a base RunConfig.
func New(sb core.Sandbox, engine *orchestrator.Engine, rc core.RunConfig) *Gateway {
	return &Gateway{sb: sb, engine: engine, rc: rc}
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

// RunTask resolves a GitHub issue end-to-end via the engine. Pass nil progress for silence.
func (g *Gateway) RunTask(ctx context.Context, owner, repo string, issueNum int, progress core.Progress) (string, error) {
	return g.engine.RunIssueTask(ctx, owner, repo, issueNum, g.rc, progress)
}

// RunSpecTask runs the agent loop for an arbitrary task string via the engine.
func (g *Gateway) RunSpecTask(ctx context.Context, task string, progress core.Progress) (string, error) {
	return g.engine.RunSingleTask(ctx, task, g.rc, progress)
}

// defaultRepo returns the configured repo used as a fallback for triggers.
func (g *Gateway) defaultRepo() string { return g.rc.Repo }

// IsVMReady reports whether the sandbox is reachable.
func (g *Gateway) IsVMReady() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return g.sb.Ready(ctx)
}

// HealthCheckVM runs "echo ok" via the sandbox and returns the output.
func (g *Gateway) HealthCheckVM(ctx context.Context) (string, error) {
	out, err := g.sb.Exec(ctx, "echo ok")
	if err != nil {
		return "", fmt.Errorf("sandbox exec: %w", err)
	}
	combined := out.Stdout
	if out.Stderr != "" {
		combined += "\nSTDERR: " + out.Stderr
	}
	if !strings.Contains(combined, "ok") {
		return "", fmt.Errorf("unexpected output: %q", combined)
	}
	return "SSH echo ok", nil
}
