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
