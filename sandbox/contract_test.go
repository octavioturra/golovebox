package sandbox

import "github.com/user/golovebox/core"

// Compile-time assertion: *VM satisfies core.Sandbox.
var _ core.Sandbox = (*VM)(nil)
