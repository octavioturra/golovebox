package agent

import (
	"context"
	"strings"
	"testing"
)

// scriptedCompleter is a core.Completer that returns canned replies in order,
// recording every prompt it was given so the transcript can be asserted.
type scriptedCompleter struct {
	replies []string
	prompts []string
	i       int
}

func (s *scriptedCompleter) Complete(_ context.Context, prompt string) (string, error) {
	s.prompts = append(s.prompts, prompt)
	if s.i >= len(s.replies) {
		// Default to a terminal action so a buggy loop doesn't spin to MaxIterations.
		return "Action: done\nParameters:\n  result: fallback", nil
	}
	r := s.replies[s.i]
	s.i++
	return r, nil
}

func newEchoRegistry() *Registry {
	r := NewRegistry()
	r.Register(Tool{
		Name:        "shell",
		Description: "echo",
		Execute: func(_ context.Context, p map[string]string) (string, error) {
			return "ran: " + p["cmd"], nil
		},
	})
	return r
}

func TestLoop_ToolThenDone(t *testing.T) {
	c := &scriptedCompleter{replies: []string{
		"Thought: run it\nAction: shell\nParameters:\n  cmd: echo hi",
		"Action: done\nParameters:\n  result: finished",
	}}
	var progressCalls int
	var lastAction, lastObs string
	loop := New(c, newEchoRegistry(), nil)
	result, err := loop.Run(context.Background(), "do the thing", func(_ int, action, _, obs, _, _ string) {
		progressCalls++
		lastAction, lastObs = action, obs
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "finished" {
		t.Fatalf("result = %q, want %q", result, "finished")
	}
	if progressCalls != 1 {
		t.Fatalf("progress called %d times, want 1", progressCalls)
	}
	if lastAction != "shell" || !strings.Contains(lastObs, "ran: echo hi") {
		t.Fatalf("progress saw action=%q obs=%q", lastAction, lastObs)
	}
	// The second prompt must accumulate the first reply + its observation (transcript).
	if len(c.prompts) != 2 {
		t.Fatalf("got %d prompts, want 2", len(c.prompts))
	}
	if !strings.Contains(c.prompts[0], "Task:\ndo the thing") {
		t.Fatalf("first prompt missing task: %q", c.prompts[0])
	}
	if !strings.Contains(c.prompts[1], "Action: shell") || !strings.Contains(c.prompts[1], "Observation: ran: echo hi") {
		t.Fatalf("second prompt missing transcript history: %q", c.prompts[1])
	}
}

func TestLoop_ErrorAction(t *testing.T) {
	c := &scriptedCompleter{replies: []string{
		"Action: error\nParameters:\n  reason: boom",
	}}
	loop := New(c, newEchoRegistry(), nil)
	_, err := loop.Run(context.Background(), "t", nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want error containing 'boom', got %v", err)
	}
}

func TestLoop_HeredocParam(t *testing.T) {
	c := &scriptedCompleter{replies: []string{
		"Action: shell\nParameters:\n  cmd: <<EOF\nline1\nline2\nEOF",
		"Action: done\nParameters:\n  result: ok",
	}}
	var gotCmd string
	r := NewRegistry()
	r.Register(Tool{Name: "shell", Execute: func(_ context.Context, p map[string]string) (string, error) {
		gotCmd = p["cmd"]
		return "", nil
	}})
	loop := New(c, r, nil)
	if _, err := loop.Run(context.Background(), "t", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCmd != "line1\nline2" {
		t.Fatalf("heredoc cmd = %q, want %q", gotCmd, "line1\nline2")
	}
}
