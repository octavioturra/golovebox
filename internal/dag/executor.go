package dag

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DispatchFunc executes a node's task and returns the result or an error.
type DispatchFunc func(ctx context.Context, node *Node) (string, error)

// NotifyFunc is called on every node state transition (for SSE/Telegram).
type NotifyFunc func(node *Node)

// ExecutorConfig controls executor parallelism and polling behaviour.
type ExecutorConfig struct {
	MaxParallel  int           // concurrent nodes; default 3
	PollInterval time.Duration // ready-check frequency; default 2s
}

func (c *ExecutorConfig) applyDefaults() {
	if c.MaxParallel <= 0 {
		c.MaxParallel = 3
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 2 * time.Second
	}
}

// Executor drives a DAG to completion by dispatching ready nodes.
type Executor struct {
	dag      *DAG
	cfg      ExecutorConfig
	dispatch DispatchFunc
	notify   NotifyFunc
	cm       *CheckpointManager
	runDir   string
	mu       sync.Mutex
}

// NewExecutor creates an Executor ready to run.
func NewExecutor(d *DAG, cfg ExecutorConfig, dispatch DispatchFunc, notify NotifyFunc, cm *CheckpointManager, runDir string) *Executor {
	cfg.applyDefaults()
	return &Executor{dag: d, cfg: cfg, dispatch: dispatch, notify: notify, cm: cm, runDir: runDir}
}

// Run executes the DAG, blocking until all nodes are complete or ctx is cancelled.
func (e *Executor) Run(ctx context.Context) error {
	if err := e.persist(); err != nil {
		return fmt.Errorf("executor: initial persist: %w", err)
	}

	sem := make(chan struct{}, e.cfg.MaxParallel)
	var wg sync.WaitGroup
	ticker := time.NewTicker(e.cfg.PollInterval)
	defer ticker.Stop()

	for {
		if e.dag.IsComplete() {
			break
		}
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		case <-ticker.C:
		}

		for _, node := range e.dag.Ready() {
			// Claim the node before spawning goroutine to prevent double-dispatch.
			e.mu.Lock()
			if node.State != StatePending {
				e.mu.Unlock()
				continue
			}
			node.State = StateRunning
			now := time.Now()
			node.StartedAt = &now
			e.mu.Unlock()

			select {
			case sem <- struct{}{}:
			default:
				// Semaphore full — revert state and retry on next tick.
				e.mu.Lock()
				node.State = StatePending
				node.StartedAt = nil
				e.mu.Unlock()
				continue
			}

			wg.Add(1)
			go func(n *Node) {
				defer wg.Done()
				defer func() { <-sem }()
				e.runNode(ctx, n)
			}(node)
		}
	}

	wg.Wait()
	return e.writeRunSummary()
}

// Resume resets interrupted nodes (StateRunning → StatePending) and re-runs.
func (e *Executor) Resume(ctx context.Context) error {
	e.mu.Lock()
	for _, n := range e.dag.Nodes {
		if n.State == StateRunning {
			n.State = StatePending
			n.StartedAt = nil
		}
	}
	e.mu.Unlock()
	return e.Run(ctx)
}

func (e *Executor) runNode(ctx context.Context, n *Node) {
	e.notifyAndPersist(n)

	switch n.Type {
	case TypeCheckpoint:
		// Human must approve before execution continues.
		e.setStateWaiting(n)
		result := <-e.cm.Wait(n.ID)
		if result.Approved {
			e.setStateDone(n, "approved")
		} else {
			e.setStateError(n, "rejected: "+result.Reason)
		}
		return
	}

	result, err := e.dispatch(ctx, n)
	if err != nil {
		e.setStateError(n, err.Error())
	} else {
		e.setStateDone(n, result)
	}
}

func (e *Executor) setStateWaiting(n *Node) {
	e.mu.Lock()
	n.State = StateWaitingHuman
	e.mu.Unlock()
	e.notifyAndPersist(n)
}

func (e *Executor) setStateDone(n *Node, result string) {
	e.mu.Lock()
	n.State = StateDone
	n.Result = result
	now := time.Now()
	n.CompletedAt = &now
	e.mu.Unlock()
	e.notifyAndPersist(n)
}

func (e *Executor) setStateError(n *Node, msg string) {
	e.mu.Lock()
	n.State = StateError
	n.Error = msg
	now := time.Now()
	n.CompletedAt = &now
	e.mu.Unlock()
	e.notifyAndPersist(n)
}

func (e *Executor) notifyAndPersist(n *Node) {
	if e.notify != nil {
		e.notify(n)
	}
	_ = e.persist()
}

// persist writes dag.json and node_states.json atomically via temp-then-rename.
func (e *Executor) persist() error {
	if e.runDir == "" {
		return nil
	}

	dagJSON, err := e.dag.ToJSON()
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(e.runDir, "dag.json"), dagJSON); err != nil {
		return err
	}

	states := make(map[string]map[string]string, len(e.dag.Nodes))
	e.dag.mu.RLock()
	for id, n := range e.dag.Nodes {
		states[id] = map[string]string{
			"state":  string(n.State),
			"result": n.Result,
			"error":  n.Error,
		}
	}
	e.dag.mu.RUnlock()

	statesJSON, err := json.Marshal(states)
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(e.runDir, "node_states.json"), statesJSON)
}

func (e *Executor) writeRunSummary() error {
	if e.runDir == "" {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("# Run Summary\n\n")
	sb.WriteString(fmt.Sprintf("Run: %s\n\n", e.dag.RunID))
	sb.WriteString("| Node | Type | State | Result |\n")
	sb.WriteString("|---|---|---|---|\n")
	e.dag.mu.RLock()
	for _, n := range e.dag.Nodes {
		result := n.Result
		if n.Error != "" {
			result = "ERROR: " + n.Error
		}
		if len(result) > 80 {
			result = result[:80] + "..."
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", n.ID, n.Type, n.State, result))
	}
	e.dag.mu.RUnlock()
	return os.WriteFile(filepath.Join(e.runDir, "run_summary.md"), []byte(sb.String()), 0o644)
}

func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
