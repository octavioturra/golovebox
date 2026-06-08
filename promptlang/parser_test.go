package promptlang_test

import (
	"testing"

	"github.com/user/golovebox/core"
	"github.com/user/golovebox/promptlang"
)

func TestNew_ParseImplementsInterface(t *testing.T) {
	var _ core.Parser = promptlang.New()
}

func TestParse_Keywords(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantKind core.StepKind
		wantText string
		wantTitle string
	}{
		{
			name:     "ATTENTION_HERE",
			src:      "ATTENTION_HERE: review before merging",
			wantKind: core.KindCheckpoint,
			wantText: "review before merging",
		},
		{
			name:     "PAUSE_TO_REVIEW",
			src:      "PAUSE_TO_REVIEW: check output",
			wantKind: core.KindCheckpoint,
			wantText: "check output",
		},
		{
			name:     "RUN_TEST",
			src:      "RUN_TEST: go test ./...",
			wantKind: core.KindTest,
			wantText: "go test ./...",
		},
		{
			name:     "NOTIFY_ME",
			src:      "NOTIFY_ME: build done",
			wantKind: core.KindNotify,
			wantText: "build done",
		},
	}

	p := promptlang.New()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			intent, err := p.Parse(tc.src)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if len(intent.Steps) == 0 {
				t.Fatalf("expected 1 step, got 0")
			}
			s := intent.Steps[0]
			if s.Kind != tc.wantKind {
				t.Errorf("Kind: got %q, want %q", s.Kind, tc.wantKind)
			}
			if tc.wantText != "" && s.Text != tc.wantText {
				t.Errorf("Text: got %q, want %q", s.Text, tc.wantText)
			}
			if tc.wantTitle != "" && s.Title != tc.wantTitle {
				t.Errorf("Title: got %q, want %q", s.Title, tc.wantTitle)
			}
		})
	}
}

func TestParse_TryOrElse(t *testing.T) {
	src := "TRY run the test suite\nOR_ELSE: skip tests and continue"
	p := promptlang.New()
	intent, err := p.Parse(src)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(intent.Steps) != 1 {
		t.Fatalf("expected 1 step (try_else), got %d", len(intent.Steps))
	}
	s := intent.Steps[0]
	if s.Kind != core.KindTryElse {
		t.Errorf("Kind: got %q, want %q", s.Kind, core.KindTryElse)
	}
	if s.Text == "" {
		t.Error("Text (try action) should not be empty")
	}
	if s.OrElse == "" {
		t.Error("OrElse (fallback) should not be empty")
	}
}

func TestParse_NotTodoGoesToTechDebts(t *testing.T) {
	src := "NOT_TODO: refactor auth module"
	p := promptlang.New()
	intent, err := p.Parse(src)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(intent.Steps) != 0 {
		t.Errorf("NOT_TODO should not produce Steps, got %d", len(intent.Steps))
	}
	if len(intent.TechDebts) != 1 || intent.TechDebts[0] != "refactor auth module" {
		t.Errorf("TechDebts: got %v", intent.TechDebts)
	}
}

func TestParseDir(t *testing.T) {
	specs, err := promptlang.ParseDir(t.TempDir())
	if err != nil {
		t.Fatalf("ParseDir on empty dir: %v", err)
	}
	if len(specs) != 0 {
		t.Errorf("expected 0 specs, got %d", len(specs))
	}
}
