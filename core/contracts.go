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
	KindCommit     StepKind = "commit"
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
// Promoted from toolskills.CompleteFn now that derivator is the second consumer.
type Completer interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// --- Derivation ---

// PromptPriority classifies a prompt node within the graph.
type PromptPriority string

const (
	PriorityCritical  PromptPriority = "critical"
	PriorityEnabling  PromptPriority = "enabling"
	PriorityAccessory PromptPriority = "accessory"
)

// PromptNode is a single prompt within a PromptGraph.
type PromptNode struct {
	ID        string
	Text      string
	Priority  PromptPriority
	DependsOn []string
}

// PromptGraph is the output of a Deriver — a prioritised graph of prompts.
type PromptGraph struct {
	Nodes []PromptNode
}

// Deriver turns a seed text into a prioritised PromptGraph.
type Deriver interface {
	Derive(ctx context.Context, seed string) (PromptGraph, error)
}

// --- Orchestration ---

// Progress is an observability callback invoked once per ReAct iteration.
// The app implements it (NodeLog persistence + SSE broadcast); the orchestrator
// only calls it. It replaces the agent loop's direct coupling to web.NodeLog.
type Progress func(nodeID string, iter int, action, params, obs, prompt, reply string)

// RunConfig carries the per-run values the app injects into the orchestrator,
// so the engine never imports internal/config. Tokens and paths come from the
// composition root, which resolves any precedence (e.g. workflow.default_repo →
// default_repo) before populating Repo.
type RunConfig struct {
	Repo          string // owner/repo, already resolved by the app
	DefaultBranch string // base branch for PRs
	GitHubToken   string
	WorkDir       string // run directory, e.g. .golovebox/runs/<id>/
	ClonePath     string // repo path inside the VM, e.g. /root/repo
	RunMode       string // "build_only" restricts the planning prompt; "" = unrestricted
}
