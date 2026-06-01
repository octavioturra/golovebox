// Package derivator converts a seed text into a prioritised PromptGraph via LLM.
// It depends only on core — never on internal packages or sandbox.
package derivator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/user/golovebox/core"
)

type deriver struct{ llm core.Completer }

// New returns a core.Deriver backed by the given Completer.
func New(llm core.Completer) core.Deriver {
	return &deriver{llm}
}

// promptNode is the JSON schema the LLM is asked to produce.
type promptNode struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Priority  string `json:"priority"`
	DependsOn []string `json:"depends_on"`
}

const derivePrompt = `You are a prompt graph planner. Given a seed goal, decompose it into a prioritised graph of sub-prompts.

Each node must have:
- id: short snake_case identifier
- text: the full sub-prompt text
- priority: "critical" (must do), "enabling" (unlocks others), or "accessory" (nice to have)
- depends_on: list of node ids that must complete first (empty array if none)

Return ONLY a JSON array, no markdown fences, no explanation:
[{"id":"string","text":"string","priority":"critical|enabling|accessory","depends_on":["id",...]}]

Seed goal:
`

func (d *deriver) Derive(ctx context.Context, seed string) (core.PromptGraph, error) {
	reply, err := d.llm.Complete(ctx, derivePrompt+seed)
	if err != nil {
		return core.PromptGraph{}, fmt.Errorf("derivator: llm: %w", err)
	}

	nodes, err := parseNodes(reply)
	if err != nil {
		return core.PromptGraph{}, fmt.Errorf("derivator: parse: %w", err)
	}

	graph := core.PromptGraph{Nodes: make([]core.PromptNode, 0, len(nodes))}
	for _, n := range nodes {
		prio, err := parsePriority(n.Priority)
		if err != nil {
			return core.PromptGraph{}, fmt.Errorf("derivator: node %q: %w", n.ID, err)
		}
		deps := n.DependsOn
		if deps == nil {
			deps = []string{}
		}
		graph.Nodes = append(graph.Nodes, core.PromptNode{
			ID:        n.ID,
			Text:      n.Text,
			Priority:  prio,
			DependsOn: deps,
		})
	}
	return graph, nil
}

func parseNodes(reply string) ([]promptNode, error) {
	s := strings.TrimSpace(reply)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON array in reply: %.200s", reply)
	}
	s = s[start : end+1]

	var nodes []promptNode
	if err := json.Unmarshal([]byte(s), &nodes); err != nil {
		return nil, fmt.Errorf("unmarshal: %w (input: %.200s)", err, s)
	}
	return nodes, nil
}

func parsePriority(s string) (core.PromptPriority, error) {
	switch core.PromptPriority(s) {
	case core.PriorityCritical, core.PriorityEnabling, core.PriorityAccessory:
		return core.PromptPriority(s), nil
	default:
		return "", fmt.Errorf("unknown priority %q", s)
	}
}
