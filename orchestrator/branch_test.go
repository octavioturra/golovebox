package orchestrator

import (
	"strings"
	"testing"
)

func TestGenerateBranchName_format(t *testing.T) {
	name := GenerateBranchName()
	parts := strings.Split(name, "-")
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts separated by -, got %d: %q", len(parts), name)
	}
	if parts[0] != "glb" {
		t.Errorf("expected prefix glb, got %q", parts[0])
	}
	if len(parts[3]) != 4 {
		t.Errorf("expected 4-char hex suffix, got %q", parts[3])
	}
}

func TestGenerateBranchName_noCollision(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		name := GenerateBranchName()
		if seen[name] {
			t.Fatalf("collision at iteration %d: %q", i, name)
		}
		seen[name] = true
	}
}
