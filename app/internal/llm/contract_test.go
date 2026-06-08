package llm

import "github.com/user/golovebox/core"

// Compile-time assertion: *completer satisfies core.Completer.
var _ core.Completer = (*completer)(nil)
