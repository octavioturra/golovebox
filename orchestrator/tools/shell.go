package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/user/golovebox/core"
)

// Shell executes cmd in the VM via the sandbox and returns combined output.
// Non-zero exit codes are surfaced in the returned string rather than as errors,
// so the agent loop always receives the full command output as its observation.
func Shell(ctx context.Context, sb core.Sandbox, cmd string) string {
	o, execErr := sb.Exec(ctx, cmd)
	var sb2 strings.Builder
	if o.Stdout != "" {
		sb2.WriteString(o.Stdout)
	}
	if o.Stderr != "" {
		if sb2.Len() > 0 {
			sb2.WriteString("\n")
		}
		sb2.WriteString("STDERR: " + o.Stderr)
	}
	if execErr != nil {
		if sb2.Len() > 0 {
			sb2.WriteString("\n")
		}
		sb2.WriteString(fmt.Sprintf("Exit error: %v", execErr))
	}
	return sb2.String()
}
