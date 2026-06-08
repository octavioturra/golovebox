// Package coretest provides shared test doubles and contract suites for
// golovebox modules. Import in *_test.go files only.
package coretest

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/user/golovebox/core"
)

// ExecResponse is a scripted reply for FakeSandbox.Exec.
type ExecResponse struct {
	Stdout   string
	ExitCode int
	Err      error
}

// FakeSandbox is a configurable core.Sandbox for unit tests. It records every
// Exec command in order and returns scripted responses. Unscripted commands
// return ("", 0, nil) unless DefaultExecErr is set.
type FakeSandbox struct {
	mu sync.Mutex

	// ExecResponses maps exact command strings to scripted results.
	ExecResponses  map[string]ExecResponse
	DefaultExecErr error

	// Files is the in-memory filesystem for PutFile/GetFile.
	Files map[string][]byte

	// ReadyResult is returned by Ready (default true).
	ReadyResult bool

	// Calls records every Exec command, in order.
	Calls []string
}

// Verify compile-time that FakeSandbox satisfies core.Sandbox.
var _ core.Sandbox = (*FakeSandbox)(nil)

// NewFakeSandbox returns a FakeSandbox ready to use.
func NewFakeSandbox() *FakeSandbox {
	return &FakeSandbox{
		ExecResponses: make(map[string]ExecResponse),
		Files:         make(map[string][]byte),
		ReadyResult:   true,
	}
}

// Script registers a canned response for a specific command.
func (f *FakeSandbox) Script(cmd, stdout string, exitCode int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ExecResponses[cmd] = ExecResponse{Stdout: stdout, ExitCode: exitCode, Err: err}
}

func (f *FakeSandbox) Exec(_ context.Context, cmd string) (core.Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, cmd)
	if r, ok := f.ExecResponses[cmd]; ok {
		return core.Output{Stdout: r.Stdout, ExitCode: r.ExitCode}, r.Err
	}
	if f.DefaultExecErr != nil {
		return core.Output{ExitCode: 1}, f.DefaultExecErr
	}
	return core.Output{}, nil
}

func (f *FakeSandbox) PutFile(_ context.Context, path string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Files[path] = append([]byte(nil), data...)
	return nil
}

func (f *FakeSandbox) GetFile(_ context.Context, path string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.Files[path]
	if !ok {
		return nil, fmt.Errorf("fakesandbox: %q not found", path)
	}
	return append([]byte(nil), d...), nil
}

func (f *FakeSandbox) Ready(_ context.Context) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ReadyResult
}

// ErrScripted is a sentinel error for testing error paths.
var ErrScripted = errors.New("scripted error")
