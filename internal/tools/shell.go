package tools

import (
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/user/golovebox/internal/sandbox"
)

// Shell executes cmd in the VM via SSH and returns combined output.
// Non-zero exit codes are surfaced in the returned string rather than as errors,
// so the agent loop always receives the full command output as its observation.
func Shell(client *ssh.Client, cmd string) string {
	stdout, stderr, execErr := sandbox.Exec(client, cmd)
	var sb strings.Builder
	if stdout != "" {
		sb.WriteString(stdout)
	}
	if stderr != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("STDERR: " + stderr)
	}
	if execErr != nil {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("Exit error: %v", execErr))
	}
	return sb.String()
}
