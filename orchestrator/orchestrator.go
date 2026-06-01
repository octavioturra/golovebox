// Package orchestrator implements core.CodingAgent — DAG orchestration of coding tasks.
// This module is a thin, dependency-free adapter over the core interfaces.
// The full orchestration logic remains in the app's internal/orchestrator package.
package orchestrator

import (
	"context"

	"github.com/user/golovebox/core"
)

// Orchestrator implements core.CodingAgent by delegating to a registered executor.
type Orchestrator struct {
	executor func(ctx context.Context, spec core.Spec) (core.Report, error)
}

// New creates an Orchestrator with the given executor function.
// Pass nil to get a no-op stub (useful for testing).
func New(executor func(ctx context.Context, spec core.Spec) (core.Report, error)) core.CodingAgent {
	if executor == nil {
		executor = func(_ context.Context, spec core.Spec) (core.Report, error) {
			return core.Report{
				NodeID:  spec.ID,
				Success: true,
				Output:  "stub: no executor registered",
			}, nil
		}
	}
	return &Orchestrator{executor: executor}
}

// Delegate executes a coding spec and returns the report.
func (o *Orchestrator) Delegate(ctx context.Context, spec core.Spec, _ string) (core.Report, error) {
	return o.executor(ctx, spec)
}
