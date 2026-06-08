package coretest

import (
	"context"
	"testing"

	"github.com/user/golovebox/core"
)

// SandboxContract is a behavioural contract suite that any core.Sandbox
// implementation must satisfy. Run it against both mocks and the real VM:
//
//	// In unit tests (Tier 1):
//	coretest.SandboxContract(t, func() core.Sandbox { return coretest.NewFakeSandbox() })
//
//	// In integration tests (Tier 3, //go:build integration):
//	coretest.SandboxContract(t, func() core.Sandbox { return realVM })
func SandboxContract(t *testing.T, newSB func() core.Sandbox) {
	t.Helper()
	ctx := context.Background()

	t.Run("Ready", func(t *testing.T) {
		sb := newSB()
		if !sb.Ready(ctx) {
			t.Fatal("Ready() = false, want true for a healthy sandbox")
		}
	})

	t.Run("Exec_success", func(t *testing.T) {
		sb := newSB()
		out, err := sb.Exec(ctx, "echo hello")
		if err != nil {
			t.Fatalf("Exec: unexpected error: %v", err)
		}
		if out.ExitCode != 0 {
			t.Fatalf("Exec: ExitCode = %d, want 0", out.ExitCode)
		}
	})

	t.Run("PutFile_GetFile_roundtrip", func(t *testing.T) {
		sb := newSB()
		want := []byte("contract test content\n")
		if err := sb.PutFile(ctx, "/tmp/contract_test.txt", want); err != nil {
			t.Fatalf("PutFile: %v", err)
		}
		got, err := sb.GetFile(ctx, "/tmp/contract_test.txt")
		if err != nil {
			t.Fatalf("GetFile: %v", err)
		}
		if string(got) != string(want) {
			t.Fatalf("GetFile roundtrip: got %q, want %q", got, want)
		}
	})

	t.Run("GetFile_missing_returns_error", func(t *testing.T) {
		sb := newSB()
		_, err := sb.GetFile(ctx, "/tmp/no_such_file_contract_test")
		if err == nil {
			t.Fatal("GetFile on missing path: want error, got nil")
		}
	})
}
