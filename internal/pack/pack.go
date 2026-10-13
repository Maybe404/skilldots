// Package pack fetches upstream skill repositories into the cache and
// discovers the skills they contain.
package pack

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/skillmd"
)

// Source is a parsed `skd add` argument.
type Source struct {
	// Repo is the canonical "owner/repo" used as the pack key.
	Repo string
	// URL is what git clones.
	URL string
	Ref string
}

var ghShort = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// ParseSource accepts owner/repo, github:owner/repo, https://github.com/...,
// git@github.com:... and any other git URL. An @ref suffix pins a ref.
func ParseSource(s string) (Source, error) {
	src := Source{}
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "@"); i > 0 && !strings.Contains(s[i:], "/") && !strings.HasPrefix(s, "git@") {
		src.Ref = s[i+1:]
		s = s[:i]
	} else if strings.HasPrefix(s, "git@") {
		if i := strings.LastIndex(s, "@"); i > 3 && !strings.Contains(s[i:], ":") {
			src.Ref = s[i+1:]
			s = s[:i]
		}
	}
	s = strings.TrimPrefix(s, "github:")
	s = strings.TrimSuffix(s, "/")
	switch {
	case ghShort.MatchString(s):
		src.Repo = strings.TrimSuffix(s, ".git")
		src.URL = "https://github.com/" + src.Repo + ".git"
	case strings.HasPrefix(s, "https://github.com/"), strings.HasPrefix(s, "http://github.com/"):
		p := strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
		parts := strings.Split(strings.TrimPrefix(p, "github.com/"), "/")
		if len(parts) < 2 {
			return src, fmt.Errorf("cannot parse GitHub URL %q", s)
		}
		src.Repo = parts[0] + "/" + strings.TrimSuffix(parts[1], ".git")
		src.URL = "https://github.com/" + src.Repo + ".git"
	case strings.HasPrefix(s, "git@github.com:"):
		p := strings.TrimSuffix(strings.TrimPrefix(s, "git@github.com:"), ".git")
		src.Repo = p
		src.URL = s
	default:
		// Generic git URL: key by host/path.
		p := s
		for _, pre := range []string{"https://", "http://", "ssh://", "git://"} {
			p = strings.TrimPrefix(p, pre)
		}
		p = strings.TrimSuffix(strings.ReplaceAll(p, ":", "/"), ".git")
		if !strings.Contains(p, "/") {
			return src, fmt.Errorf("unrecognised source %q", s)
		}
		src.Repo = p
		src.URL = s
	}
	return src, nil
}

// CachePath is the clone location for a repo key.
func CachePath(repo string) string {
	return filepath.Join(library.CacheDir(), strings.ReplaceAll(repo, "/", "__"))
}

// Fetch clones or updates the cache clone and returns its path and HEAD commit.
func Fetch(src Source) (dir, commit string, err error) {
	dir = CachePath(src.Repo)
	if !fsutil.Exists(filepath.Join(dir, ".git")) {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return "", "", err
		}
		args := []string{"clone", "-q", "--depth", "1"}
		if src.Ref != "" {
			args = append(args, "--branch", src.Ref)
		}
		args = append(args, src.URL, dir)
		if _, err := library.Git("", args...); err != nil {
			os.RemoveAll(dir)
			return "", "", err
		}
	} else {
		ref := src.Ref
		if ref == "" {
			ref = "HEAD"
		}
		if _, err := library.Git(dir, "fetch", "-q", "--depth", "1", "origin", ref); err != nil {
			return "", "", err
		}
		if _, err := library.Git(dir, "reset", "-q", "--hard", "FETCH_HEAD"); err != nil {
			return "", "", err
		}
	}
	commit, err = library.Git(dir, "rev-parse", "HEAD")
	return dir, commit, err
}

// Skill is one skill discovered in a repository.
type Skill struct {
	Name        string
	Description string
	License     string
	// Dir is the absolute skill directory.
	Dir string
	// Category groups skills inside a pack ("" when flat).
	Category string
	// AltDirs are other copies of the same skill found in the repository.
	AltDirs []string
}

// testDirs are never scanned: fixtures routinely contain throwaway skills.
var testDirs = map[string]bool{"test": true, "tests": true, "testdata": true, "fixtures": true, "__fixtures__": true, "__tests__": true}

// structural path segments carry no meaning for categorisation.
var structural = map[string]bool{"skills": true, "skill": true, ".agents": true, "plugins": true, "plugin": true}

// Scan discovers every SKILL.md under root, ignoring Skip, hidden (except
// .agents) and test directories, and nested skills inside another skill.
// Repositories often ship the same skill copied into several agent
// directories; such duplicates are collapsed to one, preferring the
// canonical skills/ or .agents/skills/ location, with the others in AltDirs.
func Scan(root string) ([]Skill, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || p == root {
			return nil
		}
		name := d.Name()
		if fsutil.Skip[name] || testDirs[name] || (strings.HasPrefix(name, ".") && name != ".agents") {
			return filepath.SkipDir
		}
		if skillmd.IsSkillDir(p) {
			dirs = append(dirs, p)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if skillmd.IsSkillDir(root) {
		dirs = append(dirs, root)
	}
	sort.Strings(dirs)

	var all []Skill
	for _, d := range dirs {
		meta, err := skillmd.Read(d)
		if err != nil {
			return nil, err
		}
		rel, _ := filepath.Rel(root, d)
		all = append(all, Skill{Name: meta.Name, Description: meta.Description, License: meta.License, Dir: d,
			Category: rawCategory(filepath.ToSlash(rel), meta.Name)})
	}

	// Collapse duplicates by name.
	best := map[string]int{}
	var kept []Skill
	for _, s := range all {
		i, ok := best[s.Name]
		if !ok {
			best[s.Name] = len(kept)
			kept = append(kept, s)
			continue
		}
		if rank(root, s.Dir) < rank(root, kept[i].Dir) {
			s.AltDirs = append(append([]string{}, kept[i].AltDirs...), kept[i].Dir)
			kept[i] = s
		} else {
			kept[i].AltDirs = append(kept[i].AltDirs, s.Dir)
		}
	}

	// Drop the category prefix every skill shares.
	if len(kept) > 0 {
		prefix := strings.Split(kept[0].Category, "/")
		for _, s := range kept[1:] {
			p := strings.Split(s.Category, "/")
			n := 0
			for n < len(prefix) && n < len(p) && prefix[n] == p[n] {
				n++
			}
			prefix = prefix[:n]
		}
		if pre := strings.Join(prefix, "/"); pre != "" {
			for i := range kept {
				kept[i].Category = strings.TrimPrefix(strings.TrimPrefix(kept[i].Category, pre), "/")
			}
		}
	}
	return kept, nil
}

// rawCategory derives a category from a skill dir's repo-relative path by
// dropping structural segments and a trailing segment equal to the name.
func rawCategory(rel, name string) string {
	segs := strings.Split(rel, "/")
	if len(segs) > 0 {
		segs = segs[:len(segs)-1] // the skill dir itself
	}
	var out []string
	for _, s := range segs {
		if !structural[s] {
			out = append(out, s)
		}
	}
	if len(out) > 0 && out[len(out)-1] == name {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "/")
}

// rank orders duplicate locations: canonical roots first, then shallower.
func rank(root, dir string) int {
	rel, _ := filepath.Rel(root, dir)
	rel = filepath.ToSlash(rel)
	switch {
	case strings.HasPrefix(rel, "skills/"):
		return 0
	case strings.HasPrefix(rel, ".agents/skills/"):
		return 1
	}
	return 10 + strings.Count(rel, "/")
}

// Find returns the skill with the given name or "category/name".
func Find(skills []Skill, key string) *Skill {
	for i := range skills {
		s := &skills[i]
		if s.Name == key || (s.Category != "" && s.Category+"/"+s.Name == key) {
			return s
		}
	}
	return nil
}

// Categories returns the distinct categories in order of first appearance.
func Categories(skills []Skill) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range skills {
		if !seen[s.Category] {
			seen[s.Category] = true
			out = append(out, s.Category)
		}
	}
	return out
}
