// Package skillmd reads the frontmatter of a SKILL.md file.
package skillmd

import (
	"bufio"
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
	size, closed := 0, false
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
			closed = true
			break
		}
		if size += len(line) + 1; size > maxFront {
			break
		}
		front = append(front, line)
	}
	// Without its closing "---" the "frontmatter" runs into the body:
	// only its top-level "key: value" lines are read, as before, never
	// the body's indented lines as part of a value.
	m, ok := Meta{}, false
	if closed {
		m, ok = parseYAML(strings.Join(front, "\n"), front)
	}
	if !ok {
		m = parseLines(front, closed)
	}
	m.Name = strings.TrimSpace(oneLine(m.Name))
	m.Description = safetext.Clean(oneLine(m.Description), 0)
	m.License = safetext.Clean(oneLine(m.License), 0)
	if m.Name == "" {
		m.Name = filepath.Base(dir)
	}
	return m, nil
}

// parseYAML reads the keys skilldots uses from YAML frontmatter; ok is
// false when it is not a YAML mapping. The document is read as a node
// tree and only the top-level mapping is looked at: decoding into a Go
// map checks every key against every other, which upstream text with
// thousands of repeated keys turns into seconds of work and gigabytes.
//
// A scalar is taken as written (Node.Value), not resolved to a type:
// "name: 1.0", "010", "0x1F", "1e3", "True", "null", "~" or a date stay
// what they look like, the way the line reader and agents read them.
// EmptyName is set only for a name key whose value is empty. A "#" after
// a space starts a YAML comment and is dropped, as YAML-reading agents
// do; text that is not valid YAML keeps it (parseLines).
func parseYAML(front string, lines []string) (m Meta, ok bool) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(front), &doc); err != nil || doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return Meta{}, false
	}
	top := doc.Content[0]
	if top.Kind != yaml.MappingNode {
		return Meta{}, false
	}
	hasName := false
	var nameNode *yaml.Node
	for i := 0; i+1 < len(top.Content); i += 2 {
		k, v := top.Content[i], top.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			continue
		}
		// A key given twice: the last one, as the line reader takes it.
		switch k.Value {
		case "name":
			hasName, nameNode = true, v
		case "description":
			m.Description = scalar(v)
		case "license":
			m.License = scalar(v)
		}
	}
	if hasName {
		if s, isScalar := scalarOK(nameNode); isScalar {
			m.Name = s
			m.EmptyName = strings.TrimSpace(s) == ""
		} else {
			// A list or a mapping is not a name: what the line says, which
			// is no valid skill name, so the skill is refused, not renamed.
			m.Name = parseLines(lines, true).Name
		}
	}
	return m, true
}

// scalar is a YAML value as text, as written; anything else (a list, a
// mapping) is not a name or a description.
func scalar(n *yaml.Node) string {
	s, _ := scalarOK(n)
	return s
}

func scalarOK(n *yaml.Node) (string, bool) {
	if n != nil && n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	if n == nil || n.Kind != yaml.ScalarNode {
		return "", false
	}
	return n.Value, true
}

// parseLines is the forgiving reader for frontmatter that is not YAML:
// "key: value" lines, with indented lines continuing the value (a block
// scalar marker such as ">" or "|-" starts an empty one) when cont is set;
// a blank line does not end a value.
func parseLines(front []string, cont bool) Meta {
	m := Meta{}
	cur := ""
	for _, line := range front {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if t := strings.TrimSpace(line); cont && t != "" && (cur == "description" || cur == "license") {
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
