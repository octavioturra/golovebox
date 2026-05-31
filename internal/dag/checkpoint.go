package dag

import "sync"

// CheckpointResult carries the outcome of a human review decision.
type CheckpointResult struct {
	Approved bool
	Reason   string // populated when !Approved
}

// CheckpointManager brokers human-approval signals between the web UI and the executor.
// Both Wait and Approve/Reject are safe to call in any order — the result is buffered
// so a pre-approval (Approve called before Wait) is not lost.
type CheckpointManager struct {
	mu      sync.Mutex
	pending map[string]chan CheckpointResult
}

// NewCheckpointManager returns an initialised manager.
func NewCheckpointManager() *CheckpointManager {
	return &CheckpointManager{pending: make(map[string]chan CheckpointResult)}
}

// Wait returns a channel that delivers the review decision.
// If Approve or Reject was already called for nodeID, the channel is returned
// pre-filled so the caller unblocks immediately.
func (cm *CheckpointManager) Wait(nodeID string) <-chan CheckpointResult {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	ch, ok := cm.pending[nodeID]
	if !ok {
		ch = make(chan CheckpointResult, 1)
		cm.pending[nodeID] = ch
	}
	return ch
}

// Approve unblocks the executor for nodeID with an approved result.
func (cm *CheckpointManager) Approve(nodeID string) {
	cm.mu.Lock()
	ch, ok := cm.pending[nodeID]
	if !ok {
		ch = make(chan CheckpointResult, 1)
		cm.pending[nodeID] = ch
	}
	cm.mu.Unlock()
	// Non-blocking send — channel is buffered (cap 1); if already filled, skip.
	select {
	case ch <- CheckpointResult{Approved: true}:
	default:
	}
}

// Reject unblocks the executor for nodeID with a rejection reason.
func (cm *CheckpointManager) Reject(nodeID, reason string) {
	cm.mu.Lock()
	ch, ok := cm.pending[nodeID]
	if !ok {
		ch = make(chan CheckpointResult, 1)
		cm.pending[nodeID] = ch
	}
	cm.mu.Unlock()
	select {
	case ch <- CheckpointResult{Approved: false, Reason: reason}:
	default:
	}
}
