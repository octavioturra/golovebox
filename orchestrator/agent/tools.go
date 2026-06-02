package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type Tool struct {
	Name        string
	Description string
	Parameters  map[string]string // param name → description
	Execute     func(ctx context.Context, params map[string]string) (string, error)
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Descriptions formats all registered tools for inclusion in the system prompt.
func (r *Registry) Descriptions() string {
	var sb strings.Builder
	sb.WriteString("Available tools:\n")

	// Sort for deterministic output
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		t := r.tools[name]
		sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Name, t.Description))
		if len(t.Parameters) > 0 {
			sb.WriteString("  Parameters:\n")
			// Sort params too
			pnames := make([]string, 0, len(t.Parameters))
			for p := range t.Parameters {
				pnames = append(pnames, p)
			}
			sort.Strings(pnames)
			for _, p := range pnames {
				sb.WriteString(fmt.Sprintf("    %s: %s\n", p, t.Parameters[p]))
			}
		}
	}
	return sb.String()
}
