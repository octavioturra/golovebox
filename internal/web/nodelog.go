package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// NodeLogEntry is a single ReAct iteration recorded for a node.
type NodeLogEntry struct {
	Iteration   int       `json:"iter"`
	Action      string    `json:"action"`
	Params      string    `json:"params"`
	Observation string    `json:"obs"`
	Prompt      string    `json:"prompt,omitempty"`
	Reply       string    `json:"reply,omitempty"`
	Timestamp   time.Time `json:"ts"`
}

// NodeLog buffers ReAct iterations for a single node in memory and persists
// them line-by-line to a .jsonl file (append-only).
type NodeLog struct {
	mu      sync.Mutex
	entries []NodeLogEntry
	nodeID  string
	runDir  string
}

func newNodeLog(nodeID, runDir string) *NodeLog {
	return &NodeLog{nodeID: nodeID, runDir: runDir}
}

// Append records a new iteration, updates the in-memory slice, and appends to disk.
func (nl *NodeLog) Append(iter int, action, params, obs, prompt, reply string) {
	entry := NodeLogEntry{
		Iteration:   iter,
		Action:      action,
		Params:      params,
		Observation: obs,
		Prompt:      prompt,
		Reply:       reply,
		Timestamp:   time.Now(),
	}
	nl.mu.Lock()
	nl.entries = append(nl.entries, entry)
	nl.mu.Unlock()
	_ = nl.persistLine(entry)
}

// Entries returns a snapshot of all entries accumulated so far.
func (nl *NodeLog) Entries() []NodeLogEntry {
	nl.mu.Lock()
	defer nl.mu.Unlock()
	out := make([]NodeLogEntry, len(nl.entries))
	copy(out, nl.entries)
	return out
}

func (nl *NodeLog) persistLine(entry NodeLogEntry) error {
	dir := filepath.Join(nl.runDir, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, nl.nodeID+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, _ := json.Marshal(entry)
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// loadNodeLogFromDisk reads a .jsonl file and returns all valid entries.
// Used when the in-memory buffer is unavailable (e.g. after process restart).
func loadNodeLogFromDisk(nodeID, runDir string) ([]NodeLogEntry, error) {
	path := filepath.Join(runDir, "logs", nodeID+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []NodeLogEntry{}, nil
		}
		return nil, err
	}
	var entries []NodeLogEntry
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var e NodeLogEntry
		if json.Unmarshal(line, &e) == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}
