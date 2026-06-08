package promptlang

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Annotation is a semantic marker extracted from a spec file.
type Annotation struct {
	Keyword  KeywordType
	Argument string // everything after "KEYWORD: " on the same line
	Line     int
	Raw      string // original full line
}

// ParsedSpec holds the parsed content of a single spec file.
type ParsedSpec struct {
	FilePath    string
	Content     string       // full original content
	Annotations []Annotation
	TechDebts   []string // content extracted from NOT_TODO lines
}

// ParseContent parses prompt DSL from a string (content) rather than a file.
func ParseContent(content string) (*ParsedSpec, error) {
	spec := &ParsedSpec{
		FilePath: "",
		Content:  content,
	}
	if err := parseInto(spec); err != nil {
		return nil, err
	}
	return spec, nil
}

// ParseFile reads a Markdown file and extracts keyword annotations.
func ParseFile(path string) (*ParsedSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("promptlang: read %s: %w", path, err)
	}

	spec := &ParsedSpec{
		FilePath: path,
		Content:  string(data),
	}
	if err := parseInto(spec); err != nil {
		return nil, fmt.Errorf("promptlang: scan %s: %w", path, err)
	}
	return spec, nil
}

// ParseDir reads all .md files in dir and returns their parsed specs.
func ParseDir(dir string) ([]*ParsedSpec, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("promptlang: readdir %s: %w", dir, err)
	}
	var specs []*ParsedSpec
	var firstErr error
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		s, err := ParseFile(filepath.Join(dir, e.Name()))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		specs = append(specs, s)
	}
	return specs, firstErr
}

// parseInto fills spec.Annotations and spec.TechDebts from spec.Content.
func parseInto(spec *ParsedSpec) error {
	scanner := bufio.NewScanner(strings.NewReader(spec.Content))
	lineNum := 0
	var tryBuffer []string // accumulates TRY...OR_ELSE multi-line blocks
	var tryStartLine int

	flushTry := func(orElseLine string) {
		if len(tryBuffer) == 0 {
			return
		}
		tryArg := strings.Join(tryBuffer, " ")
		orElseArg := strings.TrimSpace(strings.TrimPrefix(orElseLine, string(KwOrElse)))
		orElseArg = strings.TrimPrefix(orElseArg, ":")
		orElseArg = strings.TrimSpace(orElseArg)

		spec.Annotations = append(spec.Annotations, Annotation{
			Keyword:  KwTry,
			Argument: tryArg,
			Line:     tryStartLine,
			Raw:      strings.Join(append(tryBuffer, orElseLine), "\n"),
		})
		spec.Annotations = append(spec.Annotations, Annotation{
			Keyword:  KwOrElse,
			Argument: orElseArg,
			Line:     lineNum,
			Raw:      orElseLine,
		})
		tryBuffer = nil
	}

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Accumulating a TRY block
		if tryBuffer != nil {
			if strings.HasPrefix(trimmed, string(KwOrElse)) {
				flushTry(trimmed)
			} else {
				tryBuffer = append(tryBuffer, trimmed)
			}
			continue
		}

		ann, ok := parseLine(lineNum, line, trimmed)
		if !ok {
			continue
		}

		switch ann.Keyword {
		case KwNotTodo:
			spec.TechDebts = append(spec.TechDebts, ann.Argument)
		case KwTry:
			// Start accumulating multi-line TRY block
			tryBuffer = []string{ann.Argument}
			tryStartLine = lineNum
		default:
			spec.Annotations = append(spec.Annotations, ann)
		}
	}

	return scanner.Err()
}

// noArgKeywords matches keywords that stand alone on a line (no argument or no colon required).
var noArgKeywords = map[KeywordType]bool{
	KwCommit: true, // "COMMIT" alone → auto-generated message
}

// spaceArgKeywords matches keywords that take an argument after a space (no colon).
var spaceArgKeywords = map[KeywordType]bool{
	KwTry:    true,
	KwCommit: true, // "COMMIT <message>"
}

// parseLine attempts to match a single trimmed line against all known keywords.
func parseLine(lineNum int, raw, trimmed string) (Annotation, bool) {
	// WHEN ... DO ... (single-line pattern)
	if strings.HasPrefix(trimmed, string(KwWhen)+" ") {
		parts := strings.SplitN(trimmed, " "+string(KwDo)+" ", 2)
		if len(parts) == 2 {
			whenArg := strings.TrimSpace(strings.TrimPrefix(parts[0], string(KwWhen)))
			doArg := strings.TrimSpace(parts[1])
			return Annotation{Keyword: KwWhen, Argument: whenArg + " DO " + doArg, Line: lineNum, Raw: raw}, true
		}
	}

	for _, kw := range allKeywords {
		kwStr := string(kw)

		// Exact match for no-arg keywords (e.g. PUSH alone on a line)
		if noArgKeywords[kw] && trimmed == kwStr {
			return Annotation{Keyword: kw, Argument: "", Line: lineNum, Raw: raw}, true
		}

		// "KEYWORD: arg" — colon form
		prefix := kwStr + ":"
		if strings.HasPrefix(trimmed, prefix) {
			arg := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			return Annotation{Keyword: kw, Argument: arg, Line: lineNum, Raw: raw}, true
		}

		// "KEYWORD arg" — space form (no colon)
		if spaceArgKeywords[kw] {
			prefix2 := kwStr + " "
			if strings.HasPrefix(trimmed, prefix2) {
				arg := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix2))
				return Annotation{Keyword: kw, Argument: arg, Line: lineNum, Raw: raw}, true
			}
		}
	}
	return Annotation{}, false
}
