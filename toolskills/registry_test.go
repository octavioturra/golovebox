package toolskills_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/user/golovebox/core"
	"github.com/user/golovebox/toolskills"
)

const sampleSkill = `---
name = "test_skill"
description = "A test skill"
tools = ["shell", "read_file"]
provision = ["echo setup"]
examples = ["Do something useful"]
---
Detailed instructions here.
`

func TestRegistryParseFrontmatter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test_skill.md"), []byte(sampleSkill), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := toolskills.NewRegistry(dir)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	sk, ok := reg.Get("test_skill")
	if !ok {
		t.Fatal("skill not found")
	}
	if sk.Name != "test_skill" {
		t.Errorf("name: got %q, want %q", sk.Name, "test_skill")
	}
	if sk.Description != "A test skill" {
		t.Errorf("description: got %q", sk.Description)
	}
	if len(sk.Tools) != 2 || sk.Tools[0] != "shell" {
		t.Errorf("tools: got %v", sk.Tools)
	}
	if len(sk.Provision) != 1 || sk.Provision[0] != "echo setup" {
		t.Errorf("provision: got %v", sk.Provision)
	}
	if sk.Prompt != "Detailed instructions here." {
		t.Errorf("prompt: got %q", sk.Prompt)
	}
}

// mockSandbox records executed commands and can simulate exit code errors.
type mockSandbox struct {
	commands []string
	failOn   string
}

func (m *mockSandbox) Exec(_ context.Context, cmd string) (core.Output, error) {
	m.commands = append(m.commands, cmd)
	if m.failOn == cmd {
		return core.Output{ExitCode: 1, Stderr: "mock error"}, nil
	}
	return core.Output{Stdout: "ok"}, nil
}
func (m *mockSandbox) PutFile(_ context.Context, _ string, _ []byte) error { return nil }
func (m *mockSandbox) GetFile(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (m *mockSandbox) Ready(_ context.Context) bool                        { return true }

func TestProvisionerRunsCommands(t *testing.T) {
	sb := &mockSandbox{}
	p := toolskills.NewProvisioner(sb)

	skill := core.SkillMeta{
		Name:      "mypkg",
		Provision: []string{"apk add git", "apk add curl"},
	}
	if err := p.Provision(context.Background(), skill); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if len(sb.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(sb.commands))
	}
	if sb.commands[0] != "apk add git" || sb.commands[1] != "apk add curl" {
		t.Errorf("commands: %v", sb.commands)
	}
}

func TestProvisionerFailsOnNonZeroExit(t *testing.T) {
	sb := &mockSandbox{failOn: "apk add missing"}
	p := toolskills.NewProvisioner(sb)

	skill := core.SkillMeta{
		Name:      "bad",
		Provision: []string{"apk add missing"},
	}
	if err := p.Provision(context.Background(), skill); err == nil {
		t.Fatal("expected error on non-zero exit, got nil")
	}
}

func TestProvisionerEmptyProvision(t *testing.T) {
	sb := &mockSandbox{}
	p := toolskills.NewProvisioner(sb)
	if err := p.Provision(context.Background(), core.SkillMeta{Name: "no-ops"}); err != nil {
		t.Fatal(err)
	}
	if len(sb.commands) != 0 {
		t.Errorf("expected 0 commands, got %d", len(sb.commands))
	}
}
