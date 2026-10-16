package engine

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/manifest"
)

// gitRepo commits everything under dir as a fresh repository.
func gitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "c"}} {
		gitIn(t, dir, args...)
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeSkillMD(t *testing.T, dir, frontmatterName string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + frontmatterName + "\ndescription: d\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// snapshot lists every path under root (relative) with its size, skipping
// the subtrees in skip.
func snapshot(t *testing.T, root string, skip ...string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		for _, s := range skip {
			if p == s {
				return filepath.SkipDir
			}
		}
		if p == root {
			return nil // its mtime moves when the cache dir appears
		}
		info, _ := os.Lstat(p)
		rel, _ := filepath.Rel(root, p)
		out = append(out, fmt.Sprintf("%s %s %d %d", rel, info.Mode(), info.Size(), info.ModTime().UnixNano()))
		return nil
	})
	sort.Strings(out)
	return out
}

// Upstream SKILL.md names are untrusted: none of these may make skilldots
// create, change or delete anything outside the library's vendor/.
func TestSyncRejectsUnsafeNames(t *testing.T) {
	world := t.TempDir()
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(world, "cache"))
	lib, _, _ := newTestLib(t)
	lib.Root = filepath.Join(world, "lib")
	os.MkdirAll(lib.Root, 0o755)

	victim := filepath.Join(world, "victim")
	os.MkdirAll(victim, 0o755)
	os.WriteFile(filepath.Join(victim, "precious.txt"), []byte("precious"), 0o644)

	up := filepath.Join(world, "upstream")
	writeSkillMD(t, filepath.Join(up, "skills", "good"), "good")
	evil := []string{"../../../../victim", "../victim", "a/b", `a\b`, `""`, ".hidden", "..", "/abs"}
	for i, n := range evil {
		writeSkillMD(t, filepath.Join(up, "skills", "evil"+string(rune('a'+i))), n)
	}
	gitRepo(t, up)

	before := snapshot(t, world, filepath.Join(world, "cache"), lib.Root, up)
	p := &manifest.Pack{Repo: up, Use: []string{"*"}}
	lib.Manifest.Packs = append(lib.Manifest.Packs, p)
	rep, err := Sync(lib, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Added) != 1 || rep.Added[0] != "good" {
		t.Errorf("added %v, want [good]", rep.Added)
	}
	if len(rep.Rejected) != len(evil) {
		t.Errorf("rejected %d, want %d: %v", len(rep.Rejected), len(evil), rep.Rejected)
	}

	// Second round: upstream changes, and the lock is tampered with.
	os.WriteFile(filepath.Join(up, "skills", "good", "SKILL.md"), []byte("---\nname: good\n---\nv2\n"), 0o644)
	gitIn(t, up, "commit", "-qam", "v2")
	lib.Lock.Packs[up].Skills["../../../victim"] = lib.Lock.Packs[up].Skills["good"]
	if _, err := Sync(lib, p); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, world, filepath.Join(world, "cache"), lib.Root, up)
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("files outside the library changed:\nbefore %v\nafter  %v", before, after)
	}
	if b, err := os.ReadFile(filepath.Join(victim, "precious.txt")); err != nil || string(b) != "precious" {
		t.Fatal("victim touched")
	}
	// Nothing landed outside vendor/ inside the library either.
	filepath.WalkDir(lib.Root, func(p string, d fs.DirEntry, err error) error {
		if p != lib.Root && !strings.HasPrefix(p, lib.VendorDir()) && !strings.HasPrefix(p, filepath.Join(lib.Root, TrashDir)) {
			t.Errorf("unexpected path %s", p)
		}
		return nil
	})
}

func TestResolveRejectsUnsafeManifestNames(t *testing.T) {
	lib, reg, _ := newTestLib(t)
	writeSkill(t, filepath.Join(lib.Root, "skills", "ok"), "ok")
	writeSkillMD(t, filepath.Join(lib.Root, "skills", "sneaky"), "../sneaky")
	lib.Manifest.Apps = []*manifest.App{{Name: "../app", Path: "/tmp"}}
	lib.Manifest.Locals = []*manifest.Local{{Name: "a/b"}}
	lib.Manifest.Packs = []*manifest.Pack{{Repo: "o/r", Use: []string{"*"},
		Skill: map[string]*manifest.SkillOverride{"x": {As: "../../x"}}}}
	lib.Lock.Packs["o/r"] = &lockPack{Skills: map[string]*lockSkill{"x": {Hash: "h"}, "../y": {Hash: "h"}}}
	writeSkill(t, filepath.Join(lib.VendorDir(), "o", "r", "x"), "x")

	entries, warnings, err := Resolve(lib, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].LinkName != "ok" {
		t.Errorf("entries %+v, want only ok", entries)
	}
	if len(warnings) != 5 {
		t.Errorf("want 5 warnings, got %d:\n%s", len(warnings), strings.Join(warnings, "\n"))
	}
}

func TestSyncWithoutLockKeepsLocalEdits(t *testing.T) {
	world := t.TempDir()
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(world, "cache"))
	lib, _, _ := newTestLib(t)
	up := filepath.Join(world, "up")
	writeSkillMD(t, filepath.Join(up, "skills", "a"), "a")
	writeSkillMD(t, filepath.Join(up, "skills", "b"), "b")
	gitRepo(t, up)
	p := &manifest.Pack{Repo: up, Use: []string{"*"}}
	if _, err := Sync(lib, p); err != nil {
		t.Fatal(err)
	}
	pd, _ := PackDir(lib, up)
	os.WriteFile(filepath.Join(pd, "a", "mine.txt"), []byte("edit"), 0o644)

	delete(lib.Lock.Packs, up) // lock lost
	rep, err := Sync(lib, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Backups) != 1 || !strings.Contains(rep.Backups[0], "a") {
		t.Fatalf("backups %v, want the edited a only", rep.Backups)
	}
	if m, _ := filepath.Glob(filepath.Join(lib.Root, TrashDir, "*", "a", "mine.txt")); len(m) != 1 {
		t.Fatal("local edit not preserved in the trash")
	}
	if _, err := os.Stat(filepath.Join(pd, "a", "mine.txt")); !os.IsNotExist(err) {
		t.Fatal("vendor copy not restored to upstream content")
	}
}

// A followed-everything pack whose upstream gains a skill named like one
// of the user's own gets an `as`, and nothing conflicts.
func TestSyncAliasesNewcomerClash(t *testing.T) {
	world := t.TempDir()
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(world, "cache"))
	lib, reg, _ := newTestLib(t)
	writeSkill(t, filepath.Join(lib.Root, "skills", "foo"), "foo")
	up := filepath.Join(world, "src", "team-skills")
	writeSkillMD(t, filepath.Join(up, "skills", "bar"), "bar")
	gitRepo(t, up)
	p := &manifest.Pack{Repo: up, Use: []string{"*"}}
	lib.Manifest.Packs = append(lib.Manifest.Packs, p)
	if _, err := Sync(lib, p); err != nil {
		t.Fatal(err)
	}

	writeSkillMD(t, filepath.Join(up, "skills", "foo"), "foo")
	gitIn(t, up, "add", "-A")
	gitIn(t, up, "commit", "-qm", "foo")
	rep, err := Sync(lib, p)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Skill["foo"]; got == nil || got.As != "team-skills-foo" {
		t.Fatalf("as = %+v, want team-skills-foo (renamed %v)", got, rep.Renamed)
	}
	entries, warnings, _ := Resolve(lib, reg)
	if len(warnings) > 0 {
		t.Errorf("warnings: %v", warnings)
	}
	if e := Find(entries, "team-skills-foo"); e == nil || e.Conflict != "" || e.Kind != KindVendor {
		t.Errorf("aliased entry: %+v", e)
	}
	// The user's own skill keeps its name.
	if e := Find(entries, "foo"); e == nil || e.Kind != KindSkill {
		t.Errorf("own entry: %+v", e)
	}
}

// When a skill's category changes, its vendored directory is renamed:
// nothing goes to the trash and the skill is not reported as changed.
func TestSyncCategoryChangeRenames(t *testing.T) {
	world := t.TempDir()
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(world, "cache"))
	lib, _, _ := newTestLib(t)
	up := filepath.Join(world, "up")
	writeSkillMD(t, filepath.Join(up, "skills", "old", "a"), "a")
	gitRepo(t, up)
	p := &manifest.Pack{Repo: up, Use: []string{"*"}}
	if _, err := Sync(lib, p); err != nil {
		t.Fatal(err)
	}
	pd, _ := PackDir(lib, up)
	marker := filepath.Join(pd, "old", "a", "SKILL.md")
	st0, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}

	gitIn(t, up, "mv", "skills/old", "skills/new")
	gitIn(t, up, "commit", "-qm", "mv")
	rep, err := Sync(lib, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Changed) != 0 || len(rep.Moved) != 1 || len(rep.Unchanged) != 1 {
		t.Fatalf("changed %v moved %v unchanged %v", rep.Changed, rep.Moved, rep.Unchanged)
	}
	st1, err := os.Stat(filepath.Join(pd, "new", "a", "SKILL.md"))
	if err != nil || !os.SameFile(st0, st1) {
		t.Fatal("vendored directory was not renamed in place")
	}
	if _, err := os.Stat(filepath.Join(pd, "old")); !os.IsNotExist(err) {
		t.Error("empty old category directory left behind")
	}
	if _, err := os.Stat(filepath.Join(lib.Root, TrashDir)); !os.IsNotExist(err) {
		t.Error("category change put something in the trash")
	}
	if got := lib.Lock.Packs[up].Skills["a"].Category; got != "new" {
		t.Errorf("lock category %q", got)
	}
}

// A LICENSE that is a symlink must not pull a file from elsewhere on the
// machine into vendor/.
func TestSyncIgnoresSymlinkedLicense(t *testing.T) {
	world := t.TempDir()
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(world, "cache"))
	lib, _, _ := newTestLib(t)
	secret := filepath.Join(world, "secret.txt")
	os.WriteFile(secret, []byte("TOP-SECRET"), 0o600)
	up := filepath.Join(world, "up")
	writeSkillMD(t, filepath.Join(up, "skills", "a"), "a")
	os.Symlink(secret, filepath.Join(up, "LICENSE"))
	os.Symlink(secret, filepath.Join(up, "skills", "a", "LICENSE.md"))
	gitRepo(t, up)
	p := &manifest.Pack{Repo: up, Use: []string{"*"}}
	rep, err := Sync(lib, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0], "LICENSE") {
		t.Errorf("warnings %v", rep.Warnings)
	}
	filepath.WalkDir(lib.Root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), "TOP-SECRET") {
				t.Errorf("secret copied into %s", p)
			}
			if strings.HasPrefix(d.Name(), "LICENSE") {
				t.Errorf("license copied: %s", p)
			}
		}
		return nil
	})
}

// Two skills whose names differ only in case: one is vendored, the other
// rejected, nothing is trashed, and a second update is a no-op. A local
// skill differing only in case from a new upstream one gets it an alias.
func TestSyncCaseClash(t *testing.T) {
	world := t.TempDir()
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(world, "cache"))
	lib, reg, _ := newTestLib(t)
	writeSkill(t, filepath.Join(lib.Root, "skills", "Bar"), "Bar")
	up := filepath.Join(world, "up")
	writeSkillMD(t, filepath.Join(up, "skills", "x", "foo"), "foo")
	writeSkillMD(t, filepath.Join(up, "skills", "y", "Foo"), "Foo")
	writeSkillMD(t, filepath.Join(up, "skills", "bar"), "bar")
	gitRepo(t, up)
	p := &manifest.Pack{Repo: up, Use: []string{"*"}}
	lib.Manifest.Packs = append(lib.Manifest.Packs, p)
	for round := 0; round < 2; round++ {
		rep, err := Sync(lib, p)
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Rejected) != 1 {
			t.Fatalf("round %d: rejected %v", round, rep.Rejected)
		}
		if round == 1 && len(rep.Changed)+len(rep.Added)+len(rep.Dropped) != 0 {
			t.Fatalf("second update not stable: %+v", rep)
		}
	}
	if _, err := os.Stat(filepath.Join(lib.Root, TrashDir)); !os.IsNotExist(err) {
		t.Error("something went to the trash")
	}
	if o := p.Skill["bar"]; o == nil || o.As == "" {
		t.Errorf("bar vs local Bar: no alias assigned (%+v)", o)
	}
	if _, warnings, _ := Resolve(lib, reg); len(warnings) != 0 {
		t.Errorf("warnings: %v", warnings)
	}
}

// A category equal to another skill's name must not make the two vendor
// directories nest and push each other out on every update.
func TestSyncNestedPathsStable(t *testing.T) {
	world := t.TempDir()
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(world, "cache"))
	lib, _, _ := newTestLib(t)
	up := filepath.Join(world, "up")
	writeSkillMD(t, filepath.Join(up, "skills", "foo"), "foo")
	writeSkillMD(t, filepath.Join(up, "skills", "foo-cat", "bar"), "bar")
	gitRepo(t, up)
	// Upstream later moves bar under a "foo" category.
	p := &manifest.Pack{Repo: up, Use: []string{"*"}}
	if _, err := Sync(lib, p); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(up, "plugins", "foo", "skills"), 0o755)
	gitIn(t, up, "mv", "skills/foo-cat/bar", "plugins/foo/skills/bar")
	gitIn(t, up, "commit", "-qm", "mv")
	pd, _ := PackDir(lib, up)
	for round := 0; round < 3; round++ {
		rep, err := Sync(lib, p)
		if err != nil {
			t.Fatal(err)
		}
		if round > 0 && len(rep.Changed)+len(rep.Added)+len(rep.Moved)+len(rep.Backups) != 0 {
			t.Fatalf("round %d not stable: %+v", round, rep)
		}
		if _, err := os.Stat(filepath.Join(pd, "foo", "SKILL.md")); err != nil {
			t.Fatalf("round %d: foo lost", round)
		}
		if _, err := os.Stat(filepath.Join(pd, "foo", "bar")); err == nil {
			t.Fatalf("round %d: bar nested inside foo", round)
		}
	}
}
