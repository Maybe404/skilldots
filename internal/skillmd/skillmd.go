// Package skillmd reads the frontmatter of a SKILL.md file.
package skillmd

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
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

// Read parses the frontmatter of dir/SKILL.md. Name falls back to the
// directory name when the frontmatter has none.
func Read(dir string) (Meta, error) {
	f, err := os.Open(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()

	m := Meta{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	inFront := false
	for i := 0; sc.Scan(); i++ {
		line := sc.Text()
		if i == 0 {
			if strings.TrimSpace(line) != "---" {
				break
			}
			inFront = true
			continue
		}
		if !inFront {
			break
		}
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		switch strings.TrimSpace(key) {
		case "name":
			m.Name = val
			m.EmptyName = val == ""
		case "description":
			m.Description = val
		case "license":
			m.License = val
		}
	}
	if m.Name == "" {
		m.Name = filepath.Base(dir)
	}
	return m, nil
}

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
