// Package skillmd reads the frontmatter of a SKILL.md file.
package skillmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/Maybe404/skilldots/internal/safetext"
)

// Meta is the subset of SKILL.md frontmatter skilldots cares about.
type Meta struct {
	Name        string
	Description string
	License     string
	// EmptyName is set when the frontmatter has a name key with no value;
	// Name then holds the directory name fallback.
	EmptyName bool
}

// maxFront bounds the frontmatter read: names and descriptions are short,
// and a SKILL.md is upstream text.
const maxFront = 64 * 1024

// Read parses the frontmatter of dir/SKILL.md. Name falls back to the
// directory name when the frontmatter has none.
//
// The frontmatter is read as YAML, so folded (>), literal (|) and
// multi-line plain descriptions come out whole. Many SKILL.md files are not
// valid YAML (an unquoted "description: Use when: …"); those are read line
// by line instead, as agents read them.
func Read(dir string) (Meta, error) {
	f, err := os.Open(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()

	var front []string
	size := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for i := 0; sc.Scan(); i++ {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if i == 0 {
			if strings.TrimSpace(line) != "---" {
				break
			}
			continue
		}
		if strings.TrimSpace(line) == "---" {
			break
		}
		if size += len(line) + 1; size > maxFront {
			break
		}
		front = append(front, line)
	}
	m, ok := parseYAML(strings.Join(front, "\n"))
	if !ok {
		m = parseLines(front)
	}
	m.Description = safetext.Clean(oneLine(m.Description), 0)
	m.License = safetext.Clean(oneLine(m.License), 0)
	if m.Name == "" {
		m.Name = filepath.Base(dir)
	}
	return m, nil
}

// parseYAML reads the keys skilldots uses from YAML frontmatter; ok is
// false when it is not a YAML mapping.
func parseYAML(front string) (m Meta, ok bool) {
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(front), &raw); err != nil || raw == nil {
		return Meta{}, false
	}
	name, hasName := raw["name"]
	m.Name = scalar(name)
	m.EmptyName = hasName && m.Name == ""
	m.Description = scalar(raw["description"])
	m.License = scalar(raw["license"])
	return m, true
}

// scalar is a YAML value as text: strings, numbers and booleans; anything
// else (a list, a mapping) is not a name or a description.
func scalar(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case int, int64, uint64, float64, bool:
		return fmt.Sprint(v)
	}
	return ""
}

// parseLines is the forgiving reader for frontmatter that is not YAML:
// "key: value" lines, with indented lines continuing the value (a block
// scalar marker such as ">" or "|-" starts an empty one).
func parseLines(front []string) Meta {
	m := Meta{}
	cur := ""
	for _, line := range front {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if t := strings.TrimSpace(line); t != "" && (cur == "description" || cur == "license") {
				if cur == "description" {
					m.Description += " " + t
				} else {
					m.License += " " + t
				}
			}
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			cur = ""
			continue
		}
		cur = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if blockMarker(val) {
			val = ""
		}
		val = strings.Trim(val, `"'`)
		switch cur {
		case "name":
			m.Name = val
			m.EmptyName = val == ""
		case "description":
			m.Description = val
		case "license":
			m.License = val
		}
	}
	return m
}

// blockMarker reports whether v starts a YAML block scalar: > or |, with
// an optional chomping (+ -) and indentation digit.
func blockMarker(v string) bool {
	if v == "" || (v[0] != '>' && v[0] != '|') {
		return false
	}
	return strings.Trim(v[1:], "+-0123456789") == ""
}

// oneLine joins the lines of a value with single spaces: names and
// descriptions are shown on one line in every listing.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// IsSkillDir reports whether dir contains a SKILL.md.
func IsSkillDir(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "SKILL.md"))
	return err == nil && !st.IsDir()
}

// SetName rewrites the frontmatter name of dir/SKILL.md, adding the key
// (or a frontmatter block) when missing. The rest of the file is kept.
func SetName(dir, name string) error {
	p := filepath.Join(dir, "SKILL.md")
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	nameLine := "name: " + name
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return os.WriteFile(p, []byte("---\n"+nameLine+"\n---\n"+string(b)), 0o644)
	}
	for i := 1; i < len(lines); i++ {
		l := lines[i]
		if strings.TrimSpace(l) == "---" {
			break
		}
		if k, _, ok := strings.Cut(l, ":"); ok && !strings.HasPrefix(l, " ") && !strings.HasPrefix(l, "\t") && strings.TrimSpace(k) == "name" {
			lines[i] = nameLine
			return os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o644)
		}
	}
	lines = append(lines[:1], append([]string{nameLine}, lines[1:]...)...)
	return os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o644)
}
