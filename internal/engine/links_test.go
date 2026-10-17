package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// A directory inside the library that is a symbolic link (skills/, a
// pack's directory in vendor/) would make a skill's files wherever it
// points. Such a skill is neither linked nor copied, into a link target
// or a copy target; skd says which link it is.
func TestSourcesThroughLinksInsideTheLibraryAreRefused(t *testing.T) {
	for _, mode := range []string{"link", "copy"} {
		t.Run(mode, func(t *testing.T) {
			lib, _, home := newTestLib(t)
			reg := customRegistry(t, map[string]struct{ Path, Mode string }{"box": {Path: filepath.Join(home, "box", "skills"), Mode: mode}})
			SetRegistry(reg)
			lib.Manifest.Targets.Default = []string{"box"}
			outside := filepath.Join(home, "outside")
			writeSkill(t, filepath.Join(outside, "baz"), "baz")
			os.WriteFile(filepath.Join(outside, "baz", "secret.txt"), []byte("s"), 0o600)
			if err := os.Symlink(outside, filepath.Join(lib.Root, "skills")); err != nil {
				t.Fatal(err)
			}
			// A vendored skill whose pack directory is a link.
			lib.Manifest.Packs = append(lib.Manifest.Packs, &manifest.Pack{Repo: "o/r", Use: []string{"*"}})
			lib.Lock.Packs["o/r"] = &lock.Pack{Skills: map[string]*lock.Skill{"baz": {Hash: "h"}}}
			os.MkdirAll(filepath.Join(lib.Root, "vendor", "o"), 0o755)
			os.Symlink(outside, filepath.Join(lib.Root, "vendor", "o", "r"))
			lib.Manifest.Packs[0].Skill = map[string]*manifest.SkillOverride{"baz": {As: "vbaz"}}

			entries, warnings, err := Resolve(lib, reg)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(warnings, "\n"), "is a symbolic link") {
				t.Errorf("no warning: %q", warnings)
			}
			plan := MakePlan(lib, entries, true)
			for _, op := range plan.Ops {
				if op.Action != "skip" || !strings.Contains(op.Reason, "symbolic link inside the library") {
					t.Errorf("%+v", op)
				}
			}
			// Apply refuses too, should a plan still hold such an op.
			for i := range plan.Ops {
				plan.Ops[i].Action = "create"
			}
			res, err := Apply(lib, plan)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Refused) != len(plan.Ops) || len(plan.Ops) != 2 {
				t.Errorf("refused %v of %d", res.Refused, len(plan.Ops))
			}
			if m, _ := filepath.Glob(filepath.Join(home, "box", "skills", "*")); len(m) != 0 {
				t.Errorf("wrote %v", m)
			}
		})
	}
}

// The library itself may be reached through a symbolic link (a library
// kept on another volume, or a temporary directory under /var on macOS):
// its skills are linked and copied as usual.
func TestLibraryRootThroughALinkIsFine(t *testing.T) {
	lib, _, home := newTestLib(t)
	reg := customRegistry(t, map[string]struct{ Path, Mode string }{"box": {Path: filepath.Join(home, "box", "skills"), Mode: "copy"}})
	SetRegistry(reg)
	real := lib.Root
	writeSkill(t, filepath.Join(real, "skills", "s"), "s")
	linked := filepath.Join(home, "lib-link")
	os.Symlink(real, linked)
	l2 := &library.Library{Root: linked, Manifest: lib.Manifest, Lock: lib.Lock}
	l2.Manifest.Targets.Default = []string{"agents", "box"}
	entries, warnings, _ := Resolve(l2, reg)
	if len(warnings) != 0 {
		t.Fatalf("warnings %q", warnings)
	}
	res, err := Apply(l2, MakePlan(l2, entries, false))
	if err != nil || len(res.Refused) != 0 {
		t.Fatalf("%v %v", res.Refused, err)
	}
	if !fileExists(filepath.Join(home, ".agents", "skills", "s", "SKILL.md")) || !fileExists(filepath.Join(home, "box", "skills", "s", "SKILL.md")) {
		t.Fatal("not linked and copied")
	}
}

// Sync never writes, moves or removes through a link inside the library:
// a pack directory or vendor/ that is a link to elsewhere stops it, and
// what is there is untouched.
func TestSyncRefusesLinksInsideTheLibrary(t *testing.T) {
	for _, where := range []string{"vendor", "pack", "category"} {
		t.Run(where, func(t *testing.T) {
			world := t.TempDir()
			t.Setenv("SKILLDOTS_CACHE", filepath.Join(world, "cache"))
			lib, _, _ := newTestLib(t)
			up := filepath.Join(world, "up")
			writeSkillMD(t, filepath.Join(up, "skills", "eng", "a"), "a")
			gitRepo(t, up)
			p := &manifest.Pack{Repo: up, Use: []string{"*"}}
			if _, err := Sync(lib, p); err != nil {
				t.Fatal(err)
			}
			pd, _ := PackDir(lib, up)
			// Someone else's directory, holding exactly what the lock
			// records for a (so it would count as pristine), and a link
			// to it inside the library.
			victim := filepath.Join(world, "victim")
			var link string
			switch where {
			case "vendor":
				link = lib.VendorDir()
				os.Rename(link, victim)
			case "pack":
				link = pd
				os.Rename(link, victim)
			case "category":
				link = filepath.Join(pd, "eng")
				os.Rename(link, victim)
			}
			os.Symlink(victim, link)
			before := snapshot(t, victim)
			// Upstream changes a, so Sync would replace it.
			os.WriteFile(filepath.Join(up, "skills", "eng", "a", "SKILL.md"), []byte("---\nname: a\ndescription: new\n---\n"), 0o644)
			gitIn(t, up, "commit", "-qam", "change")
			if _, err := Sync(lib, p); err == nil || !strings.Contains(err.Error(), "symbolic link inside the library") {
				t.Fatalf("Sync: %v", err)
			}
			// Removing the pack refuses a pack directory reached through a
			// link. (A link inside the pack moves to the trash with it,
			// as a link: nothing it points at is touched.)
			if where != "category" {
				if err := RemovePack(lib, up); err == nil || !strings.Contains(err.Error(), "symbolic link") {
					t.Fatalf("RemovePack: %v", err)
				}
			}
			if after := snapshot(t, victim); strings.Join(after, "\n") != strings.Join(before, "\n") {
				t.Fatalf("victim changed:\n%v\n%v", before, after)
			}
			if items, _ := ListTrash(lib, nil); len(items) != 0 {
				t.Fatalf("trashed %+v", items)
			}
			if fsutil.LinkOnPath(lib.Root, link) != link {
				t.Fatal("the link was replaced")
			}
		})
	}
}

// A skill the pack no longer selects is moved to the trash on Sync;
// under a category that is a link to elsewhere, Sync stops instead of
// moving someone else's directory.
func TestSyncDropsNothingThroughALink(t *testing.T) {
	world := t.TempDir()
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(world, "cache"))
	lib, _, _ := newTestLib(t)
	up := filepath.Join(world, "up")
	writeSkillMD(t, filepath.Join(up, "skills", "eng", "a"), "a")
	writeSkillMD(t, filepath.Join(up, "skills", "other", "b"), "b")
	gitRepo(t, up)
	p := &manifest.Pack{Repo: up, Use: []string{"*"}}
	if _, err := Sync(lib, p); err != nil {
		t.Fatal(err)
	}
	pd, _ := PackDir(lib, up)
	victim := filepath.Join(world, "victim")
	os.Rename(filepath.Join(pd, "other"), victim)
	os.Symlink(victim, filepath.Join(pd, "other"))
	before := snapshot(t, victim)
	p.Use = []string{"a"}
	if _, err := Sync(lib, p); err == nil || !strings.Contains(err.Error(), "symbolic link inside the library") {
		t.Fatalf("Sync: %v", err)
	}
	// Nor is a new skill written into a category that is a link.
	os.Remove(filepath.Join(pd, "other"))
	os.Rename(victim, filepath.Join(pd, "other"))
	os.Rename(filepath.Join(pd, "eng"), victim)
	os.Symlink(victim, filepath.Join(pd, "eng"))
	writeSkillMD(t, filepath.Join(up, "skills", "eng", "c"), "c")
	gitIn(t, up, "add", "-A")
	gitIn(t, up, "commit", "-qm", "c")
	delete(lib.Lock.Packs[up].Skills, "a")
	before = snapshot(t, victim)
	p.Use = []string{"*"}
	if _, err := Sync(lib, p); err == nil || !strings.Contains(err.Error(), "symbolic link inside the library") {
		t.Fatalf("Sync of a new skill: %v", err)
	}
	if after := snapshot(t, victim); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Fatalf("victim changed:\n%v\n%v", before, after)
	}
	if items, _ := ListTrash(lib, nil); len(items) != 0 {
		t.Fatalf("trashed %+v", items)
	}
}

func TestLinkOnPath(t *testing.T) {
	root := realTemp(t)
	os.MkdirAll(filepath.Join(root, "a", "b"), 0o755)
	os.Symlink(filepath.Join(root, "a"), filepath.Join(root, "l"))
	for p, want := range map[string]string{
		filepath.Join(root, "a", "b"):           "",
		filepath.Join(root, "a", "b", "absent"): "",
		filepath.Join(root, "l"):                filepath.Join(root, "l"),
		filepath.Join(root, "l", "b", "c"):      filepath.Join(root, "l"),
		filepath.Join(root, "x", "l"):           "",
		root:                                    root,
		filepath.Dir(root):                      filepath.Dir(root),
	} {
		if got := fsutil.LinkOnPath(root, p); got != want {
			t.Errorf("LinkOnPath(%s) = %q, want %q", p, got, want)
		}
	}
	// The root itself may be a link.
	lr := filepath.Join(t.TempDir(), "lr")
	os.Symlink(root, lr)
	if got := fsutil.LinkOnPath(lr, filepath.Join(lr, "a", "b")); got != "" {
		t.Errorf("through the root: %q", got)
	}
}
