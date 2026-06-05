package dag

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrNeedsHuman signals that a node cannot proceed without human intervention.
// Wrap with %w in tool/git errors; executor converts to waiting_human state.
var ErrNeedsHuman = errors.New("needs human review")

// NodeState represents the lifecycle stage of a DAG node.
type NodeState string

const (
	StatePending      NodeState = "pending"
	StateRunning      NodeState = "running"
	StateDone         NodeState = "done"
	StateError        NodeState = "error"
	StateWaitingHuman NodeState = "waiting_human"
)

// NodeType classifies the kind of work a node performs.
type NodeType string

const (
	TypeTask       NodeType = "task"
	TypeCheckpoint NodeType = "checkpoint" // ATTENTION_HERE, PAUSE_TO_REVIEW
	TypeGate       NodeType = "gate"       // RUN_TEST
	TypeNotify     NodeType = "notify"     // NOTIFY_ME
	TypeWaitEvent  NodeType = "wait_event" // WHEN/DO
	TypeTryElse    NodeType = "try_else"   // TRY/OR_ELSE

	TypeSyncRepo NodeType = "sync_repo" // auto-injected first node when workflow.default_repo set
	TypeCommit   NodeType = "commit"    // COMMIT ["message"]
)

// Node is a single unit of work in the execution graph.
type Node struct {
	ID           string     `json:"id"`
	Type         NodeType   `json:"type"`
	State        NodeState  `json:"state"`
	Task         string     `json:"task"`
	Dependencies []string   `json:"dependencies"`
	Annotation   string     `json:"annotation,omitempty"`
	Result       string     `json:"result,omitempty"`
	Error        string     `json:"error,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// DAG is the directed acyclic graph of nodes for a single run.
type DAG struct {
	ID        string             `json:"id"`
	RunID     string             `json:"run_id"`
	CreatedAt time.Time          `json:"created_at"`
	Nodes     map[string]*Node   `json:"nodes"`
	Edges     [][2]string        `json:"edges"`
	mu        sync.RWMutex       `json:"-"`
}

// New creates an empty DAG for the given run.
func New(runID string) *DAG {
	return &DAG{
		ID:        "dag-" + runID,
		RunID:     runID,
		CreatedAt: time.Now(),
		Nodes:     make(map[string]*Node),
	}
}

// AddNode inserts a node into the DAG. Returns error if the ID is already used.
func (d *DAG) AddNode(n *Node) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.Nodes[n.ID]; exists {
		return fmt.Errorf("dag: node %q already exists", n.ID)
	}
	if n.State == "" {
		n.State = StatePending
	}
	d.Nodes[n.ID] = n
	return nil
}

// AddEdge records a dependency from→to (from must complete before to starts).
func (d *DAG) AddEdge(from, to string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.Nodes[from]; !ok {
		return fmt.Errorf("dag: edge source %q not found", from)
	}
	if _, ok := d.Nodes[to]; !ok {
		return fmt.Errorf("dag: edge target %q not found", to)
	}
	d.Edges = append(d.Edges, [2]string{from, to})
	return nil
}

// Roots returns nodes that have no dependencies.
func (d *DAG) Roots() []*Node {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var roots []*Node
	for _, n := range d.Nodes {
		if len(n.Dependencies) == 0 {
			roots = append(roots, n)
		}
	}
	return roots
}

// Ready returns nodes in StatePending whose every dependency is StateDone.
func (d *DAG) Ready() []*Node {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var ready []*Node
	for _, n := range d.Nodes {
		if n.State != StatePending {
			continue
		}
		allDone := true
		for _, depID := range n.Dependencies {
			dep, ok := d.Nodes[depID]
			if !ok || dep.State != StateDone {
				allDone = false
				break
			}
		}
		if allDone {
			ready = append(ready, n)
		}
	}
	return ready
}

// IsComplete returns true when every node is StateDone or StateError.
func (d *DAG) IsComplete() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, n := range d.Nodes {
		if n.State != StateDone && n.State != StateError {
			return false
		}
	}
	return len(d.Nodes) > 0
}

// ToJSON serialises the DAG to JSON.
func (d *DAG) ToJSON() ([]byte, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return json.Marshal(d)
}

// FromJSON deserialises JSON into the DAG, replacing existing content.
func FromJSON(data []byte) (*DAG, error) {
	var d DAG
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	if d.Nodes == nil {
		d.Nodes = make(map[string]*Node)
	}
	return &d, nil
}
