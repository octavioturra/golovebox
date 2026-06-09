package web

import (
	"context"

	"github.com/user/golovebox/core"
)

// fakeSandboxNotReady is a core.Sandbox stub that always reports not-ready.
type fakeSandboxNotReady struct{}

func (fakeSandboxNotReady) Exec(_ context.Context, _ string) (core.Output, error) {
	return core.Output{}, nil
}
func (fakeSandboxNotReady) PutFile(_ context.Context, _ string, _ []byte) error {
	return nil
}
func (fakeSandboxNotReady) GetFile(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (fakeSandboxNotReady) Ready(_ context.Context) bool { return false }
