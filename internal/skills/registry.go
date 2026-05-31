// Package skills manages reusable agent skill definitions stored as Markdown files.
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// Skill describes a reusable agent behaviour loaded from a .md file.
type Skill struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Tools       []string `toml:"tools"`
	Examples    []string `toml:"examples"`
	Prompt      string   `toml:"-"` // body after the closing ---
	FilePath    string   `toml:"-"`
}

// Registry holds all skills loaded from a directory on disk.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]*Skill
	dir    string
}

// NewRegistry loads all .md skills from dir, creating it if absent.
func NewRegistry(dir string) (*Registry, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("skills: mkdir %s: %w", dir, err)
	}
	r := &Registry{dir: dir, skills: make(map[string]*Skill)}
	if err := r.Reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// Get returns a skill by name.
func (r *Registry) Get(name string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	return s, ok
}

// List returns all loaded skills.
func (r *Registry) List() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		out = append(out, s)
	}
	return out
}

// Reload re-reads all .md files from the directory.
func (r *Registry) Reload() error {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return fmt.Errorf("skills: readdir: %w", err)
	}
	fresh := make(map[string]*Skill)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(r.dir, e.Name())
		s, err := parseSkillFile(path)
		if err != nil {
			continue // skip malformed files silently
		}
		fresh[s.Name] = s
	}
	r.mu.Lock()
	r.skills = fresh
	r.mu.Unlock()
	return nil
}

// Descriptions returns a formatted summary of all skills for inclusion in system prompts.
func (r *Registry) Descriptions() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Available skills (prefer these over generic approaches):\n")
	for _, s := range r.skills {
		sb.WriteString(fmt.Sprintf("  - %s: %s (tools: %s)\n", s.Name, s.Description, strings.Join(s.Tools, ", ")))
	}
	return sb.String()
}

// parseSkillFile reads a skill .md file with TOML frontmatter between --- delimiters.
func parseSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)

	// Expect: ---\n<TOML>\n---\n<prompt>
	const delim = "---"
	if !strings.HasPrefix(strings.TrimSpace(content), delim) {
		return nil, fmt.Errorf("no frontmatter in %s", path)
	}

	parts := strings.SplitN(content, delim, 3)
	// parts[0] = "" (before first ---), parts[1] = TOML, parts[2] = prompt body
	if len(parts) < 3 {
		return nil, fmt.Errorf("incomplete frontmatter in %s", path)
	}
	frontmatter := parts[1]
	prompt := strings.TrimSpace(parts[2])

	var s Skill
	if _, err := toml.Decode(frontmatter, &s); err != nil {
		return nil, fmt.Errorf("frontmatter parse %s: %w", path, err)
	}
	if s.Name == "" {
		s.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	s.Prompt = prompt
	s.FilePath = path
	return &s, nil
}
