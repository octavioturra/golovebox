// Package core defines the shared domain types and interfaces for golovebox.
// It has zero external dependencies — not even stdlib beyond primitive types.
package core

// Intent is the AST produced by parsing a DSL spec file.
type Intent struct {
	Source    string   // file path or other identifier
	Raw       string   // original spec content
	Steps     []Step
	TechDebts []string
	Repo      string
	Branch    string
}

// Step is a single semantic action extracted from a spec.
type Step struct {
	Kind   StepKind
	Text   string // action text, condition, or body
	Title  string // branch name, PR title, etc.
	OrElse string // fallback for TRY/OR_ELSE
	Line   int    // source line number
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
