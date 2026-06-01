// Package derivator implements core.Deriver — transforms a seed prompt into a PromptGraph.
// This is a thin stub that will be fleshed out in a future phase.
package derivator

import (
	"context"
	"fmt"

	"github.com/user/golovebox/core"
)

// Derivator implements core.Deriver using an LLM to expand a seed prompt.
type Derivator struct {
	// Future fields: LLM client, config, etc.
}

// New creates a no-op Derivator stub.
func New() core.Deriver {
	return &Derivator{}
}

// Derive converts a seed prompt into a PromptGraph.
// Currently returns a single-node graph wrapping the seed.
func (d *Derivator) Derive(_ context.Context, seed string) (core.PromptGraph, error) {
	if seed == "" {
		return core.PromptGraph{}, fmt.Errorf("derivator: seed must not be empty")
	}
	return core.PromptGraph{
		Seed: seed,
		Nodes: []core.PromptNode{
			{
				ID:       "root",
				Prompt:   seed,
				Priority: 0,
				Deps:     nil,
			},
		},
	}, nil
}
