package dag

import (
	"testing"
	"time"
)

func TestCheckpointManager_ApproveBeforeWait(t *testing.T) {
	t.Parallel()
	cm := NewCheckpointManager()
	cm.Approve("n1")
	ch := cm.Wait("n1")
	select {
	case r := <-ch:
		if !r.Approved {
			t.Fatalf("Approved = false, want true")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Wait blocked after pre-approval")
	}
}

func TestCheckpointManager_WaitThenApprove(t *testing.T) {
	t.Parallel()
	cm := NewCheckpointManager()
	ch := cm.Wait("n2")
	go func() { cm.Approve("n2") }()
	select {
	case r := <-ch:
		if !r.Approved {
			t.Fatalf("Approved = false, want true")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Wait timed out")
	}
}

func TestCheckpointManager_RejectCarriesReason(t *testing.T) {
	t.Parallel()
	cm := NewCheckpointManager()
	cm.Reject("n3", "not good enough")
	ch := cm.Wait("n3")
	select {
	case r := <-ch:
		if r.Approved {
			t.Fatal("Approved = true after Reject")
		}
		if r.Reason != "not good enough" {
			t.Fatalf("Reason = %q, want %q", r.Reason, "not good enough")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Wait blocked after pre-rejection")
	}
}

func TestCheckpointManager_DoubleApproveSafe(t *testing.T) {
	t.Parallel()
	cm := NewCheckpointManager()
	cm.Approve("n4")
	cm.Approve("n4") // second call must not panic or block
	ch := cm.Wait("n4")
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("blocked after double-approve")
	}
}
