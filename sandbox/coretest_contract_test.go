package sandbox_test

import (
	"testing"

	"github.com/user/golovebox/core"
	"github.com/user/golovebox/core/coretest"
)

// TestSandboxContract_FakeSandbox verifies that coretest.FakeSandbox itself honours
// the SandboxContract. This proves the mock is honest — consumers can trust it.
func TestSandboxContract_FakeSandbox(t *testing.T) {
	// FakeSandbox.Exec("echo hello") must return ExitCode 0 to satisfy the contract.
	coretest.SandboxContract(t, func() core.Sandbox {
		f := coretest.NewFakeSandbox()
		f.Script("echo hello", "hello\n", 0, nil)
		return f
	})
}
