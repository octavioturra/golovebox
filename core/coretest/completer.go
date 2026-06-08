package coretest

import (
	"context"
	"errors"
	"sync"

	"github.com/user/golovebox/core"
)

// Verify compile-time that FakeCompleter satisfies core.Completer.
var _ core.Completer = (*FakeCompleter)(nil)

// FakeCompleter is a scripted core.Completer for unit tests.
// It returns replies in order; when exhausted it returns FallbackReply.
type FakeCompleter struct {
	mu      sync.Mutex
	replies []string
	i       int

	// FallbackReply is returned when all scripted replies have been consumed.
	// Defaults to an empty string (no error).
	FallbackReply string

	// Err, if non-nil, is returned on every call instead of any reply.
	Err error

	// Prompts records every prompt received, in order.
	Prompts []string
}

// NewFakeCompleter returns a FakeCompleter with the given scripted replies.
func NewFakeCompleter(replies ...string) *FakeCompleter {
	return &FakeCompleter{replies: replies}
}

func (f *FakeCompleter) Complete(_ context.Context, prompt string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Prompts = append(f.Prompts, prompt)
	if f.Err != nil {
		return "", f.Err
	}
	if f.i < len(f.replies) {
		r := f.replies[f.i]
		f.i++
		return r, nil
	}
	return f.FallbackReply, nil
}

// ErrCompleterScripted is a sentinel for completer error paths.
var ErrCompleterScripted = errors.New("scripted completer error")
