// Package agent holds stubs for Phase 2+ AI, browser, and GitHub integrations.
//
// Note on goclaw (github.com/sausheong/goclaw): all its packages are internal/
// and cannot be imported as a library. In Phase 2 use the Anthropic SDK directly
// (github.com/anthropics/anthropic-sdk-go), which goclaw itself uses internally.
package agent

import (
	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/go-rod/rod"
	"github.com/google/go-github/v60/github"
	chromem "github.com/philippgille/chromem-go"
)

// BrowserAgent wraps go-rod for browser automation (Phase 2+).
type BrowserAgent struct {
	browser *rod.Browser
}

// GitHubClient wraps the GitHub v60 API client (Phase 2+).
type GitHubClient struct {
	inner *github.Client
}

// MemoryStore wraps chromem-go for persistent vector memory (Phase 2+).
type MemoryStore struct {
	db *chromem.DB
}

// LLMClient wraps the official Anthropic SDK for Claude API access (Phase 2+).
type LLMClient struct {
	inner *anthropic.Client
}

// New returns a zero-value placeholder used to anchor Phase 2+ dependencies
// in go.mod during Phase 1 scaffold.
func New() struct {
	Browser BrowserAgent
	GitHub  GitHubClient
	Memory  MemoryStore
	LLM     LLMClient
} {
	return struct {
		Browser BrowserAgent
		GitHub  GitHubClient
		Memory  MemoryStore
		LLM     LLMClient
	}{}
}
