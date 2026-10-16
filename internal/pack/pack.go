// Package pack fetches upstream skill repositories into the cache and
// discovers the skills they contain.
package pack

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/safetext"
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
	// It becomes the pack's name, printed by every command.
	if safetext.Has(s) {
		return src, fmt.Errorf("source %q has control characters: not a repository", s)
	}
	// git would take a leading "-" as an option (--upload-pack=…).
	if strings.HasPrefix(s, "-") {
		return src, fmt.Errorf("source %q starts with \"-\": not a repository", s)
	}
	if i := strings.LastIndex(s, "@"); i > 0 && !strings.Contains(s[i:], "/") && !strings.HasPrefix(s, "git@") {
		src.Ref = s[i+1:]
		s = s[:i]
	} else if strings.HasPrefix(s, "git@") {
		if i := strings.LastIndex(s, "@"); i > 3 && !strings.Contains(s[i:], ":") {
			src.Ref = s[i+1:]
			s = s[:i]
		}
	}
	if strings.HasPrefix(src.Ref, "-") {
		return src, fmt.Errorf("ref %q starts with \"-\": not a branch or tag", src.Ref)
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
// The repo key comes from skills.toml and may be anything: both
// separators become "__", so the clone is always one entry of the cache
// directory (a failed clone is removed, and must never be anything else).
func CachePath(repo string) string {
	name := strings.NewReplacer("/", "__", `\`, "__").Replace(repo)
	if name == "" || name == "." || name == ".." {
		name = "_" + name
	}
	return filepath.Join(library.CacheDir(), name)
}

var (
	holdMu sync.Mutex
	holds  = map[string]chan struct{}{}
	// owned are the repos whose file lock this process holds right now:
	// only then can a git lock file in the clone be a dead git's leftover
	// rather than another skd's.
	owned = map[string]bool{}
)

// holdPoll is how often HoldContext retries a lock another process has.
const holdPoll = 50 * time.Millisecond

// Hold locks repo's cache clone for this caller, against other goroutines
// (the dashboard checks upstream in the background while an update may
// run) and other skd processes (a file lock beside the clone), and
// returns the unlock function. Fetch, Scan of a clone and anything copied
// out of it belong inside one Hold. It is not reentrant.
func Hold(repo string) (unlock func()) {
	unlock, _ = HoldContext(context.Background(), repo)
	return unlock
}

// HoldContext is Hold that gives up when ctx is done. When the lock file
// cannot be created or locked (no cache directory yet, read-only cache, a
// file system without locks) only the in-process lock is taken: git
// reports any clash itself, and Fetch leaves git's lock files alone.
func HoldContext(ctx context.Context, repo string) (unlock func(), err error) {
	holdMu.Lock()
	sem := holds[repo]
	if sem == nil {
		sem = make(chan struct{}, 1)
		holds[repo] = sem
	}
	holdMu.Unlock()
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return func() {}, ctx.Err()
	}
	release := func() { <-sem }
	path := CachePath(repo) + ".lock"
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return release, nil
	}
	for {
		ok, err := tryLock(f)
		if err != nil {
			f.Close()
			return release, nil
		}
		if ok {
			setOwned(repo, true)
			return func() { setOwned(repo, false); unlockFile(f); f.Close(); release() }, nil
		}
		select {
		case <-time.After(holdPoll):
		case <-ctx.Done():
			f.Close()
			release()
			return func() {}, ctx.Err()
		}
	}
}

func setOwned(repo string, v bool) {
	holdMu.Lock()
	defer holdMu.Unlock()
	if v {
		owned[repo] = true
	} else {
		delete(owned, repo)
	}
}

// ownsClone reports whether this process holds repo's file lock.
func ownsClone(repo string) bool {
	holdMu.Lock()
	defer holdMu.Unlock()
	return owned[repo]
}

// Fetch clones or updates the cache clone and returns its path and HEAD
// commit. Callers Hold the repository.
func Fetch(src Source) (dir, commit string, err error) {
	return FetchContext(context.Background(), src)
}

// FetchContext is Fetch that stops git when ctx is done.
func FetchContext(ctx context.Context, src Source) (dir, commit string, err error) {
	dir = CachePath(src.Repo)
	if !fsutil.Exists(filepath.Join(dir, ".git")) {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return "", "", err
		}
		if _, err := library.GitContext(ctx, "", cloneArgs(src, dir)...); err != nil {
			if fsutil.Within(library.CacheDir(), dir) {
				os.RemoveAll(dir) // a clone git left half done, in skd's cache
			}
			return "", "", err
		}
	} else {
		ref := src.Ref
		if ref == "" {
			ref = "HEAD"
		}
		update := func() error {
			if _, err := library.GitContext(ctx, dir, fetchArgs(ref)...); err != nil {
				return err
			}
			_, err := library.GitContext(ctx, dir, "reset", "-q", "--hard", "FETCH_HEAD")
			return err
		}
		// git stops on a lock file it finds in the clone. With the clone's
		// file lock held no other skd is using it, but a git run by hand
		// still can: a lock file is only taken as left by a git killed
		// half-way once it is old. A younger one is waited for. Each
		// attempt can stop on another lock file, so git runs again for
		// each, a few times at most.
		err := update()
		for try := 0; err != nil; try++ {
			locks := lockPaths(dir, err.Error())
			if ctx.Err() != nil || try == lockRetries || len(locks) == 0 || !ownsClone(src.Repo) || !clearLocks(ctx, locks) {
				return "", "", err
			}
			err = update()
		}
	}
	commit, err = library.GitContext(ctx, dir, "rev-parse", "HEAD")
	return dir, commit, err
}

// cloneArgs and fetchArgs are the git arguments of Fetch. "--" ends the
// options: nothing after it is one, whatever the URL or ref says.
func cloneArgs(src Source, dir string) []string {
	args := []string{"clone", "-q", "--depth", "1"}
	if src.Ref != "" {
		args = append(args, "--branch", src.Ref)
	}
	return append(args, "--", src.URL, dir)
}

func fetchArgs(ref string) []string {
	return []string{"fetch", "-q", "--depth", "1", "--", "origin", ref}
}

// staleAge is how old a git lock file must be to count as left behind: git
// holds one only while it writes, and a git killed on cancel (SIGTERM)
// removes its own.
const staleAge = 30 * time.Second

// lockPoll is how often clearLocks looks again at a lock file in use.
const lockPoll = 200 * time.Millisecond

// lockRetries is how many times Fetch runs git again after clearing lock
// files.
const lockRetries = 5

// quotedLock finds the quoted path in git's "Unable to create
// '<path>.lock': File exists". The path is printed as is in every
// language; the quotes are ASCII in git's own messages, the others are
// there for translations that use their own.
var quotedLock = regexp.MustCompile(`'([^']+\.lock)'|"([^"]+\.lock)"|“([^”]+\.lock)”|‘([^’]+\.lock)’|«\s*([^»]+?\.lock)\s*»`)

// lockPaths returns the lock files inside dir's .git that git's error msg
// names. Paths elsewhere (a remote's own repository, say) are left out.
func lockPaths(dir, msg string) []string {
	gitDir := filepath.Join(dir, ".git")
	realGit, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range quotedLock.FindAllStringSubmatch(msg, -1) {
		var p string
		for _, g := range m[1:] {
			if g != "" {
				p = filepath.FromSlash(g)
				break
			}
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p) // git runs in dir
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(p))
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(realGit, parent)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		local := filepath.Join(gitDir, rel, filepath.Base(p))
		if !seen[local] {
			seen[local] = true
			out = append(out, local)
		}
	}
	return out
}

// clearLocks waits until each of the lock files locks is gone (the git
// that held it finished) or older than staleAge (nothing is writing it),
// then removes the old ones. It reports false when ctx is done first.
func clearLocks(ctx context.Context, locks []string) bool {
	for {
		young := false
		for _, p := range locks {
			fi, err := os.Lstat(p)
			if err == nil && time.Since(fi.ModTime()) < staleAge {
				young = true
			}
		}
		if !young {
			break
		}
		select {
		case <-time.After(lockPoll):
		case <-ctx.Done():
			return false
		}
	}
	for _, p := range locks {
		if fi, err := os.Lstat(p); err == nil && time.Since(fi.ModTime()) >= staleAge {
			os.Remove(p)
		}
	}
	return true
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

// String is r for a person: the directory is upstream's and may hold
// anything, so it is cleaned (safetext) like the rest.
func (r Rejected) String() string { return safetext.Clean(r.Dir+": "+r.Reason, 0) }

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
	skills, rejected = dropCaseClashes(root, skills, rejected)
	skills, rejected = dropNested(root, skills, rejected)
	return skills, rejected, nil
}

// VendorPath is where a skill lives inside its pack's vendor directory.
func (s Skill) VendorPath() string {
	if s.Category == "" {
		return s.Name
	}
	return s.Category + "/" + s.Name
}

// dropNested rejects skills whose vendor path would sit inside another
// skill's directory (category "foo" next to a skill named "foo"): each
// update would replace one with the other.
func dropNested(root string, skills []Skill, rejected []Rejected) ([]Skill, []Rejected) {
	owner := map[string]string{}
	for _, s := range skills {
		owner[strings.ToLower(s.VendorPath())] = s.Name
	}
	var out []Skill
	for _, s := range skills {
		segs := strings.Split(strings.ToLower(s.VendorPath()), "/")
		clash := ""
		for i := 1; i < len(segs) && clash == ""; i++ {
			clash = owner[strings.Join(segs[:i], "/")]
		}
		if clash == "" {
			out = append(out, s)
			continue
		}
		rel, _ := filepath.Rel(root, s.Dir)
		rejected = append(rejected, Rejected{Name: s.Name, Dir: filepath.ToSlash(rel),
			Reason: fmt.Sprintf("skipped: its category %q would place it inside the directory of skill %q", s.Category, clash)})
	}
	return out, rejected
}

// dropCaseClashes keeps one of several skills whose names differ only in
// case: on case-insensitive file systems (macOS, Windows defaults) their
// directories and links would overwrite each other.
func dropCaseClashes(root string, skills []Skill, rejected []Rejected) ([]Skill, []Rejected) {
	seen := map[string]int{}
	var out []Skill
	for _, s := range skills {
		k := strings.ToLower(s.Name)
		i, ok := seen[k]
		if !ok {
			seen[k] = len(out)
			out = append(out, s)
			continue
		}
		if rank(root, s.Dir) < rank(root, out[i].Dir) {
			out[i], s = s, out[i]
		}
		rel, _ := filepath.Rel(root, s.Dir)
		rejected = append(rejected, Rejected{Name: s.Name, Dir: filepath.ToSlash(rel),
			Reason: fmt.Sprintf("skipped: name differs only in case from %q (they would overwrite each other on case-insensitive file systems)", out[i].Name)})
	}
	return out, rejected
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
