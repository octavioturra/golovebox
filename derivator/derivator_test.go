package derivator_test

import (
	"context"
	"testing"

	"github.com/user/golovebox/core"
	"github.com/user/golovebox/derivator"
)

type mockCompleter struct{ reply string; err error }

func (m *mockCompleter) Complete(_ context.Context, _ string) (string, error) {
	return m.reply, m.err
}

const canonicalReply = `[
  {"id":"analyse","text":"Analyse the requirements","priority":"critical","depends_on":[]},
  {"id":"design","text":"Design the API","priority":"enabling","depends_on":["analyse"]},
  {"id":"docs","text":"Write documentation","priority":"accessory","depends_on":["design"]}
]`

func TestDeriveParsesPrioritiesAndDeps(t *testing.T) {
	d := derivator.New(&mockCompleter{reply: canonicalReply})
	g, err := d.Derive(context.Background(), "Build an HTTP API")
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(g.Nodes))
	}

	assertNode(t, g, "analyse", core.PriorityCritical, []string{})
	assertNode(t, g, "design", core.PriorityEnabling, []string{"analyse"})
	assertNode(t, g, "docs", core.PriorityAccessory, []string{"design"})
}

func TestDeriveStripsMarkdownFences(t *testing.T) {
	fenced := "```json\n" + canonicalReply + "\n```"
	d := derivator.New(&mockCompleter{reply: fenced})
	g, err := d.Derive(context.Background(), "seed")
	if err != nil {
		t.Fatalf("Derive with fences: %v", err)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(g.Nodes))
	}
}

func TestDeriveMalformedReplyErrors(t *testing.T) {
	d := derivator.New(&mockCompleter{reply: "not json at all"})
	if _, err := d.Derive(context.Background(), "seed"); err == nil {
		t.Fatal("expected error on malformed reply, got nil")
	}
}

func TestDeriveUnknownPriorityErrors(t *testing.T) {
	bad := `[{"id":"x","text":"x","priority":"unknown","depends_on":[]}]`
	d := derivator.New(&mockCompleter{reply: bad})
	if _, err := d.Derive(context.Background(), "seed"); err == nil {
		t.Fatal("expected error on unknown priority, got nil")
	}
}

func assertNode(t *testing.T, g core.PromptGraph, id string, prio core.PromptPriority, deps []string) {
	t.Helper()
	for _, n := range g.Nodes {
		if n.ID != id {
			continue
		}
		if n.Priority != prio {
			t.Errorf("node %q: priority got %q, want %q", id, n.Priority, prio)
		}
		if len(n.DependsOn) != len(deps) {
			t.Errorf("node %q: deps got %v, want %v", id, n.DependsOn, deps)
			return
		}
		for i, d := range deps {
			if n.DependsOn[i] != d {
				t.Errorf("node %q: dep[%d] got %q, want %q", id, i, n.DependsOn[i], d)
			}
		}
		return
	}
	t.Errorf("node %q not found in graph", id)
}
