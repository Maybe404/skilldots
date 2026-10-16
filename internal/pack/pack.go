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
	"github.com/Maybe404/skilldots/internal/manifest"
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
	// AltDirs are identical copies of the same skill elsewhere in the
	// repository.
	AltDirs []string `json:",omitempty"`
	// Variants are other directories with a SKILL.md of the same name but
	// different content (e.g. per-agent adaptations). Only Dir is used.
	Variants []string `json:",omitempty"`
}

// Rejected is a SKILL.md that cannot be used, with the reason.
type Rejected struct {
	Name   string `json:"name"`
	Dir    string `json:"dir"`
	Reason string `json:"reason"`
}

func (r Rejected) String() string { return r.Dir + ": " + r.Reason }

// testDirs are never scanned: fixtures routinely contain throwaway skills.
var testDirs = map[string]bool{"test": true, "tests": true, "testdata": true, "fixtures": true, "__fixtures__": true, "__tests__": true}

// structural path segments carry no meaning for categorisation. Hidden
// segments (".agents", ".claude", …) are structural too.
var structural = map[string]bool{"skills": true, "skill": true, "plugins": true, "plugin": true}

// Scan discovers every SKILL.md under root, ignoring Skip and test
// directories and nested skills inside another skill. Hidden directories
// (other than .agents) usually hold a repository's own tooling, so they
// are only searched when nothing else has a skill.
//
// Repositories often ship the same skill copied into several agent
// directories; such copies are collapsed to one, preferring the canonical
// skills/ or .agents/skills/ location. Identical copies are listed in
// AltDirs, differing ones in Variants.
//
// The category is the skill's parent path without structural segments, so
// it depends on that skill's own location only: adding a directory
// upstream never changes the category of an existing skill.
//
// Skills whose name could not safely be used as a directory name are
// returned in rejected and never in skills.
func Scan(root string) (skills []Skill, rejected []Rejected, err error) {
	dirs, err := walk(root, false)
	if err == nil && len(dirs) == 0 {
		dirs, err = walk(root, true)
	}
	if err != nil {
		return nil, nil, err
	}

	byName := map[string][]Skill{}
	var order []string
	for _, d := range dirs {
		meta, err := skillmd.Read(d)
		if err != nil {
			return nil, nil, err
		}
		rel, _ := filepath.Rel(root, d)
		if meta.EmptyName {
			rejected = append(rejected, Rejected{Name: "", Dir: filepath.ToSlash(rel), Reason: "skipped: frontmatter name is empty"})
			continue
		}
		if err := manifest.CheckName(meta.Name); err != nil {
			rejected = append(rejected, Rejected{Name: meta.Name, Dir: filepath.ToSlash(rel), Reason: "skipped: frontmatter " + err.Error()})
			continue
		}
		cat := Category(filepath.ToSlash(rel))
		if err := manifest.CheckCategory(cat); err != nil {
			rejected = append(rejected, Rejected{Name: meta.Name, Dir: filepath.ToSlash(rel), Reason: "skipped: " + err.Error()})
			continue
		}
		if _, ok := byName[meta.Name]; !ok {
			order = append(order, meta.Name)
		}
		byName[meta.Name] = append(byName[meta.Name], Skill{Name: meta.Name, Description: meta.Description, License: meta.License, Dir: d, Category: cat})
	}

	for _, name := range order {
		group := byName[name]
		best := 0
		for i := range group {
			if rank(root, group[i].Dir) < rank(root, group[best].Dir) {
				best = i
			}
		}
		kept := group[best]
		if len(group) > 1 {
			h, _ := fsutil.HashTree(kept.Dir)
			for i, s := range group {
				if i == best {
					continue
				}
				if hs, _ := fsutil.HashTree(s.Dir); hs == h && h != "" {
					kept.AltDirs = append(kept.AltDirs, s.Dir)
				} else {
					kept.Variants = append(kept.Variants, s.Dir)
				}
			}
		}
		skills = append(skills, kept)
	}
	return skills, rejected, nil
}

// walk returns the skill directories under root, sorted.
func walk(root string, hidden bool) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || p == root {
			return nil
		}
		name := d.Name()
		if fsutil.Skip[name] || testDirs[name] || (!hidden && strings.HasPrefix(name, ".") && name != ".agents") {
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
	return dirs, nil
}

// Category derives a category from a skill dir's repo-relative path: the
// parent segments minus structural and hidden ones.
func Category(rel string) string {
	segs := strings.Split(rel, "/")
	if len(segs) > 0 {
		segs = segs[:len(segs)-1] // the skill dir itself
	}
	var out []string
	for _, s := range segs {
		if s != "" && s != "." && !structural[s] && !strings.HasPrefix(s, ".") {
			out = append(out, s)
		}
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
	case strings.HasPrefix(rel, "."):
		return 100 + strings.Count(rel, "/")
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
