package web

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/user/golovebox/internal/dag"
)

// RunStore manages run directories under .golovebox/runs/.
type RunStore struct {
	runsDir    string
	logsMu     sync.Mutex
	nodeLogs   map[string]*NodeLog // key: runID+"/"+nodeID
	cancelMu   sync.Mutex
	cancelMap  map[string]context.CancelFunc
}

// NewRunStore creates a RunStore, making sure runsDir exists.
func NewRunStore(runsDir string) (*RunStore, error) {
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return nil, fmt.Errorf("runstore: mkdir %s: %w", runsDir, err)
	}
	return &RunStore{
		runsDir:   runsDir,
		nodeLogs:  make(map[string]*NodeLog),
		cancelMap: make(map[string]context.CancelFunc),
	}, nil
}

// RegisterRun stores a cancel func for an active run.
func (rs *RunStore) RegisterRun(runID string, cancel context.CancelFunc) {
	rs.cancelMu.Lock()
	rs.cancelMap[runID] = cancel
	rs.cancelMu.Unlock()
}

// UnregisterRun removes the cancel func when a run finishes.
func (rs *RunStore) UnregisterRun(runID string) {
	rs.cancelMu.Lock()
	delete(rs.cancelMap, runID)
	rs.cancelMu.Unlock()
}

// CancelRun cancels a run if it is still active.
func (rs *RunStore) CancelRun(runID string) bool {
	rs.cancelMu.Lock()
	cancel, ok := rs.cancelMap[runID]
	rs.cancelMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// GetNodeLog returns log entries for a node. Returns in-memory entries if available
// and non-empty; otherwise falls back to reading the .jsonl file from disk.
func (rs *RunStore) GetNodeLog(runID, nodeID string) []NodeLogEntry {
	key := runID + "/" + nodeID
	rs.logsMu.Lock()
	nl, exists := rs.nodeLogs[key]
	rs.logsMu.Unlock()

	if exists {
		entries := nl.Entries()
		if len(entries) > 0 {
			return entries
		}
	}
	// Fall back to disk (e.g. after process restart or when buffer is empty).
	entries, _ := loadNodeLogFromDisk(nodeID, rs.RunDir(runID))
	if entries == nil {
		return []NodeLogEntry{}
	}
	return entries
}

// NodeLogFor returns the in-memory NodeLog for the given run+node, creating it on first call.
func (rs *RunStore) NodeLogFor(runID, nodeID string) *NodeLog {
	key := runID + "/" + nodeID
	rs.logsMu.Lock()
	defer rs.logsMu.Unlock()
	if nl, ok := rs.nodeLogs[key]; ok {
		return nl
	}
	nl := newNodeLog(nodeID, rs.RunDir(runID))
	rs.nodeLogs[key] = nl
	return nl
}

// NewRun creates the directory tree for a new run and returns the run ID.
// Format: run-<YYYYMMDD>-<6 random chars>
func (rs *RunStore) NewRun() (string, error) {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	suffix := make([]byte, 6)
	for i := range suffix {
		suffix[i] = letters[rand.Intn(len(letters))]
	}
	runID := fmt.Sprintf("run-%s-%s", time.Now().Format("20060102"), string(suffix))

	runDir := rs.RunDir(runID)
	for _, sub := range []string{"specs", "logs", "artifacts"} {
		if err := os.MkdirAll(filepath.Join(runDir, sub), 0o755); err != nil {
			return "", fmt.Errorf("runstore: mkdir %s: %w", sub, err)
		}
	}
	return runID, nil
}

// RunDir returns the absolute path for a run.
func (rs *RunStore) RunDir(runID string) string {
	return filepath.Join(rs.runsDir, runID)
}

// SaveSpec saves an uploaded spec file into <runDir>/specs/<filename>.
func (rs *RunStore) SaveSpec(runID, filename string, data []byte) error {
	dest := filepath.Join(rs.RunDir(runID), "specs", filepath.Base(filename))
	return os.WriteFile(dest, data, 0o644)
}

// SpecsDir returns the path to the specs sub-directory for a run.
func (rs *RunStore) SpecsDir(runID string) string {
	return filepath.Join(rs.RunDir(runID), "specs")
}

// LoadDAG reads dag.json from the run directory.
func (rs *RunStore) LoadDAG(runID string) (*dag.DAG, error) {
	data, err := os.ReadFile(filepath.Join(rs.RunDir(runID), "dag.json"))
	if err != nil {
		return nil, fmt.Errorf("runstore: read dag.json: %w", err)
	}
	return dag.FromJSON(data)
}

// RunMeta is a lightweight summary of a run for listing purposes.
type RunMeta struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	State     string    `json:"state"`
	Task      string    `json:"task,omitempty"`
}

// ReadTask returns the task string stored in task.txt (empty string if absent).
func (rs *RunStore) ReadTask(runID string) string {
	data, _ := os.ReadFile(filepath.Join(rs.RunDir(runID), "task.txt"))
	return strings.TrimSpace(string(data))
}

// ListRuns returns metadata for all runs, newest first.
func (rs *RunStore) ListRuns() ([]RunMeta, error) {
	entries, err := os.ReadDir(rs.runsDir)
	if err != nil {
		return nil, fmt.Errorf("runstore: readdir: %w", err)
	}
	var runs []RunMeta
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "run-") {
			continue
		}
		info, _ := e.Info()
		meta := RunMeta{ID: e.Name(), State: "done"}
		if info != nil {
			meta.CreatedAt = info.ModTime()
		}
		if t := rs.ReadTask(e.Name()); t != "" {
			if len(t) > 60 {
				t = t[:60] + "..."
			}
			meta.Task = t
		}
		// Infer state from dag.json if present.
		if data, err := os.ReadFile(filepath.Join(rs.runsDir, e.Name(), "dag.json")); err == nil {
			var raw struct {
				Nodes map[string]struct {
					State string `json:"state"`
				} `json:"nodes"`
			}
			if json.Unmarshal(data, &raw) == nil {
				meta.State = inferState(raw.Nodes)
			}
		}
		runs = append(runs, meta)
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})
	return runs, nil
}

func inferState(nodes map[string]struct{ State string `json:"state"` }) string {
	hasRunning, hasError, hasPending, hasWaiting := false, false, false, false
	for _, n := range nodes {
		switch n.State {
		case "running":
			hasRunning = true
		case "error":
			hasError = true
		case "pending":
			hasPending = true
		case "waiting_human":
			hasWaiting = true
		}
	}
	if hasRunning || hasPending || hasWaiting {
		return "running"
	}
	if hasError {
		return "error"
	}
	return "done"
}
