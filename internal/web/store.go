package web

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/user/golovebox/internal/dag"
)

// RunStore manages run directories under .golovebox/runs/.
type RunStore struct {
	runsDir string
}

// NewRunStore creates a RunStore, making sure runsDir exists.
func NewRunStore(runsDir string) (*RunStore, error) {
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return nil, fmt.Errorf("runstore: mkdir %s: %w", runsDir, err)
	}
	return &RunStore{runsDir: runsDir}, nil
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
