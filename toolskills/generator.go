package toolskills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CompleteFn is a function that sends a prompt to an LLM and returns the reply.
// Callers inject this to avoid a direct dependency on internal/llm.
type CompleteFn func(ctx context.Context, prompt string) (string, error)

// Generate asks the LLM to create a new skill file from a plain-language description,
// saves it in dir, and returns the parsed Skill.
func Generate(ctx context.Context, complete CompleteFn, description, dir string) (*Skill, error) {
	prompt := fmt.Sprintf(`Generate a golovebox skill file for the following purpose:
"%s"

Output ONLY a complete .md file with TOML frontmatter between --- delimiters. No explanation, no markdown fences around the whole file.

Format:
---
name = "short_snake_case_name"
description = "One sentence description"
tools = ["shell", "read_file", "write_file"]
examples = ["Example usage sentence"]
---
Detailed prompt instructions for the agent...

## Exemplos
- Example 1
- Example 2
`, description)

	reply, err := complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("skills: generate: %w", err)
	}

	content := strings.TrimSpace(reply)
	content = strings.TrimPrefix(content, "```markdown")
	content = strings.TrimPrefix(content, "```md")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	if !strings.HasPrefix(content, "---") {
		return nil, fmt.Errorf("skills: generate: LLM did not produce valid frontmatter (got: %.100s)", content)
	}

	tmp, err := os.CreateTemp("", "skill-*.md")
	if err != nil {
		return nil, fmt.Errorf("skills: generate: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return nil, err
	}
	tmp.Close()

	s, err := parseSkillFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("skills: generate: parse generated skill: %w", err)
	}
	if s.Name == "" {
		return nil, fmt.Errorf("skills: generate: LLM produced skill with empty name")
	}

	safeName := sanitiseName(s.Name)
	destPath := filepath.Join(dir, safeName+".md")
	if err := os.WriteFile(destPath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("skills: generate: write %s: %w", destPath, err)
	}
	s.FilePath = destPath
	return s, nil
}

var notSafe = regexp.MustCompile(`[^a-z0-9_-]`)

func sanitiseName(name string) string {
	s := strings.ToLower(name)
	s = notSafe.ReplaceAllString(s, "_")
	if s == "" {
		s = "unnamed_skill"
	}
	return s
}
