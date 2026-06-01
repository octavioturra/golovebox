package llm

import (
	"context"

	"github.com/user/golovebox/core"
)

// completer wraps *Client to satisfy core.Completer.
type completer struct{ c *Client }

// NewCompleter returns a core.Completer backed by c, wrapping each prompt as a
// single user message.
func NewCompleter(c *Client) core.Completer {
	return &completer{c}
}

func (a *completer) Complete(ctx context.Context, prompt string) (string, error) {
	return a.c.Complete(ctx, []Message{{Role: "user", Content: prompt}})
}
