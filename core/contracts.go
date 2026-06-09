// Package core defines the shared domain types and interfaces for golovebox.
// It has zero external dependencies — only stdlib context.
package core

import "context"

// --- Substrate ---

// Output is the result of a Sandbox command execution.
type Output struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Sandbox is the interface for isolated command execution.
// Exec/PutFile/GetFile/Ready cover all uses in the agent loop and workflow nodes.
type Sandbox interface {
	Exec(ctx context.Context, cmd string) (Output, error)
	PutFile(ctx context.Context, path string, data []byte) error
	GetFile(ctx context.Context, path string) ([]byte, error)
	Ready(ctx context.Context) bool
}

// --- Skills ---

// SkillMeta is the frontmatter metadata of a skill file (.golovebox/skills/<x>.md).
type SkillMeta struct {
	Name        string
	Description string
	Tools       []string
	Provision   []string // shell commands to run in the VM to set up the skill
}

// Provisioner prepares the substrate for a skill by running its Provision commands.
type Provisioner interface {
	Provision(ctx context.Context, skill SkillMeta) error
}

// --- LLM ---

// Completer is the minimal LLM contract: one prompt in, one reply out.
type Completer interface {
	Complete(ctx context.Context, prompt string) (string, error)
}
