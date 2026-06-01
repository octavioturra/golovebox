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

// --- Metalanguagem ---

// Intent is the AST produced by parsing a DSL spec file.
type Intent struct {
	Source    string
	Raw       string
	Steps     []Step
	TechDebts []string
	Repo      string
	Branch    string
}

// Step is a single semantic action extracted from a spec.
type Step struct {
	Kind   StepKind
	Text   string
	Title  string
	OrElse string
	Line   int
}

// StepKind identifies the semantic type of a Step.
type StepKind string

const (
	KindBranch     StepKind = "branch"
	KindCheckpoint StepKind = "checkpoint"
	KindEdit       StepKind = "edit"
	KindNotTodo    StepKind = "not_todo"
	KindNotify     StepKind = "notify"
	KindPR         StepKind = "pr"
	KindPush       StepKind = "push"
	KindTest       StepKind = "test"
	KindTryElse    StepKind = "try_else"
	KindWait       StepKind = "wait"
)

// Parser parses DSL spec source text into an Intent.
type Parser interface {
	Parse(src string) (Intent, error)
}
