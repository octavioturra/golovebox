// Package core defines pure interfaces and domain types for the golovebox system.
// It has zero external dependencies — only stdlib.
package core

import "context"

// Sandbox is the interface for executing commands and transferring files in an isolated environment.
type Sandbox interface {
	Exec(ctx context.Context, cmd string) (Output, error)
	PutFile(ctx context.Context, path string, data []byte) error
	Ready(ctx context.Context) bool
}

// Provisioner installs and configures a skill into a sandbox environment.
type Provisioner interface {
	Provision(ctx context.Context, skill SkillMeta) error
}

// Deriver transforms a seed prompt into a PromptGraph.
type Deriver interface {
	Derive(ctx context.Context, seed string) (PromptGraph, error)
}

// Parser parses a prompt language source string into an Intent.
type Parser interface {
	Parse(src string) (Intent, error)
}

// CodingAgent delegates a coding task to an autonomous agent.
type CodingAgent interface {
	Delegate(ctx context.Context, spec Spec, repoPath string) (Report, error)
}

// Output holds the result of a sandbox command execution.
type Output struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// SkillMeta describes a reusable agent skill.
type SkillMeta struct {
	Name        string
	Description string
	Tools       []string
}

// PromptGraph is a directed acyclic graph of prompts to execute.
type PromptGraph struct {
	Seed  string
	Nodes []PromptNode
}

// PromptNode is a single node in a PromptGraph.
type PromptNode struct {
	ID       string
	Prompt   string
	Priority int
	Deps     []string
}

// Intent is the result of parsing a prompt language source.
type Intent struct {
	Raw         string
	Annotations []Annotation
	TechDebts   []string
}

// Annotation is a semantic marker in a prompt.
type Annotation struct {
	Keyword  string
	Argument string
	Line     int
	Raw      string
}

// Spec describes a coding task to delegate to an agent.
type Spec struct {
	ID   string
	Task string
	Repo string
}

// Report is the result of a delegated coding task.
type Report struct {
	NodeID  string
	Success bool
	Output  string
	Error   string
}
