package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// A copy skd wrote is removed outright only while skd's own record of
// what it placed confirms it; a copy the record does not know (written
// by an older skd, or a directory a forged lock record's hash matches)
// is left where it is when no longer wanted, never deleted or moved.
func TestCopyRemovedOutrightOnlyWhenRecorded(t *testing.T) {
	for _, recorded := range []bool{true, false} {
		lib, _, home := newTestLib(t)
		reg := customRegistry(t, map[string]struct{ Path, Mode string }{"box": {Path: filepath.Join(home, "box"), Mode: "copy"}})
		SetRegistry(reg)
		lib.Manifest.Targets.Default = []string{"box"}
		writeSkill(t, filepath.Join(lib.Root, "skills", "s"), "s")
		entries, _, _ := Resolve(lib, reg)
		if _, err := Apply(lib, MakePlan(lib, entries, false)); err != nil {
			t.Fatal(err)
		}
		if !recorded {
			os.Remove(filepath.Join(home, "placed.json"))
		}
		lib.Manifest.Targets.Default = []string{}
		entries, _, _ = Resolve(lib, reg)
		res, err := Apply(lib, MakePlan(lib, entries, true))
		if err != nil {
			t.Fatal(err)
		}
		if got := fileExists(filepath.Join(home, "box", "s", "SKILL.md")); got == recorded {
			t.Fatalf("recorded %v: copy there %v", recorded, got)
		}
		if items, _ := ListTrash(lib, reg); len(items) != 0 || len(res.Backups) != 0 {
			t.Fatalf("recorded %v: copy trashed: %+v", recorded, items)
		}
		if _, kept := lib.Lock.Links["box/s"]; kept == recorded {
			t.Fatalf("recorded %v: lock record kept %v", recorded, kept)
		}
		if m := loadPlaced(); len(m) != 0 {
			t.Fatalf("record kept after removal: %v", m)
		}
	}
}

// The record confirms only exactly what was placed: a link that now
// points elsewhere, or a copy changed since, is not confirmed.
func TestPlacedConfirmsOnlyWhatWasPlaced(t *testing.T) {
	_, _, home := newTestLib(t)
	link := filepath.Join(home, "l")
	os.Symlink("a", link)
	dir := filepath.Join(home, "d")
	writeSkill(t, dir, "d")
	h, _ := fsutil.HashTree(dir)
	m := map[string]placedEntry{link: {Link: "a"}, dir: {Hash: h}}
	if !confirms(m, link) || !confirms(m, dir) {
		t.Fatal("not confirmed as placed")
	}
	os.Remove(link)
	os.Symlink("b", link)
	os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("x"), 0o644)
	if confirms(m, link) || confirms(m, dir) {
		t.Fatal("confirmed after a change")
	}
	if confirms(m, filepath.Join(home, "other")) {
		t.Fatal("confirmed what is not recorded")
	}
	// A link where a copy was recorded, and the reverse.
	os.Remove(link)
	os.Rename(dir, link)
	os.Symlink("a", dir)
	if confirms(m, link) || confirms(m, dir) {
		t.Fatal("confirmed the wrong kind")
	}
}

// An app's link (its source outside the library) skd made is removed
// outright when the app is disabled. One made by an older skd, with no
// record, is left in place and said so: a forged lock could claim any
// link that way. Links into the library made by an older skd are
// recorded on the next apply, so they stay known as skd's when the
// library moves.
func TestAppLinksAndOlderLinks(t *testing.T) {
	for _, recorded := range []bool{true, false} {
		lib, reg, home := newTestLib(t)
		app := filepath.Join(home, "Applications", "X.app", "skills", "x")
		writeSkill(t, app, "x")
		writeSkill(t, filepath.Join(lib.Root, "skills", "s"), "s")
		lib.Manifest.Apps = append(lib.Manifest.Apps, &manifest.App{Name: "x", Path: app})
		entries, _, _ := Resolve(lib, reg)
		if _, err := Apply(lib, MakePlan(lib, entries, false)); err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(home, ".agents", "skills", "x")
		// The app's link is kept as it is, record or not.
		entries, _, _ = Resolve(lib, reg)
		for _, o := range MakePlan(lib, entries, false).Ops {
			if o.Action != "keep" {
				t.Fatalf("second apply: %+v", o)
			}
		}
		if !recorded {
			os.Remove(filepath.Join(home, "placed.json"))
			// The next apply has nothing to change; the link into the
			// library is recorded, the app's link is not.
			entries, _, _ = Resolve(lib, reg)
			plan := MakePlan(lib, entries, false)
			if plan.Changes() {
				t.Fatalf("changes: %+v", plan.ChangeList())
			}
			Remember(lib, plan)
			m := loadPlaced()
			if _, ok := m[filepath.Join(home, ".agents", "skills", "s")]; !ok || len(m) != 1 {
				t.Fatalf("recorded %v", m)
			}
		}
		lib.Manifest.Apps = nil
		entries, _, _ = Resolve(lib, reg)
		plan := MakePlan(lib, entries, true)
		var op Op
		for _, o := range plan.Ops {
			if o.Name == "x" {
				op = o
			}
		}
		want := "remove"
		if !recorded {
			want = "skip"
		}
		if op.Action != want {
			t.Fatalf("recorded %v: %+v", recorded, op)
		}
		if _, err := Apply(lib, plan); err != nil {
			t.Fatal(err)
		}
		if got := fileExists(dest); got == recorded {
			t.Fatalf("recorded %v: link there %v", recorded, got)
		}
		if items, _ := ListTrash(lib, reg); len(items) != 0 {
			t.Fatalf("trashed %+v", items)
		}
		if !fileExists(filepath.Join(app, "SKILL.md")) {
			t.Fatal("the app's skill went")
		}
	}
}

// A copy record never matches a link: a forged record with the hash of
// nothing (what a link hashes to) leaves the user's link alone.
func TestCopyRecordDoesNotClaimALink(t *testing.T) {
	lib, reg, home := newTestLib(t)
	lib.Manifest.Targets.Default = []string{}
	agents := filepath.Join(home, ".agents", "skills")
	os.MkdirAll(agents, 0o755)
	empty := filepath.Join(home, "empty")
	os.MkdirAll(empty, 0o755)
	h, _ := fsutil.HashTree(empty)
	dest := filepath.Join(agents, "mine")
	os.Symlink(filepath.Join(home, "mine"), dest)
	lib.Lock.Links["agents/mine"] = &lock.Link{Mode: "copy", Source: "skills/mine", Hash: h}
	entries, _, _ := Resolve(lib, reg)
	plan := MakePlan(lib, entries, true)
	if len(plan.Ops) != 1 || plan.Ops[0].Action != "skip" {
		t.Fatalf("%+v", plan.Ops)
	}
}

// A link already where a skill wants it (made by hand) is taken over as
// it is: nothing is removed or moved to the trash.
func TestAdoptingALinkMovesNothing(t *testing.T) {
	lib, reg, home := newTestLib(t)
	src := filepath.Join(lib.Root, "skills", "s")
	writeSkill(t, src, "s")
	agents := filepath.Join(home, ".agents", "skills")
	os.MkdirAll(agents, 0o755)
	os.Symlink(src, filepath.Join(agents, "s"))
	entries, _, _ := Resolve(lib, reg)
	res, err := Apply(lib, MakePlan(lib, entries, false))
	if err != nil || len(res.Backups) != 0 {
		t.Fatalf("%v %v", res.Backups, err)
	}
	if items, _ := ListTrash(lib, reg); len(items) != 0 {
		t.Fatalf("trashed %+v", items)
	}
	if l := lib.Lock.Links["agents/s"]; l == nil || l.Source != "skills/s" {
		t.Fatalf("not recorded: %+v", l)
	}
}

// Forged records of entries the manifest does not want: a copy record
// whose hash matches the user's own directory, and a link record whose
// source makes the user's own link look like skd's from a previous
// library location. Neither apply nor apply --force moves or removes
// anything; the plan says skd cannot tell it made them.
func TestForgedOrphanRecordsTouchNothing(t *testing.T) {
	for _, force := range []bool{false, true} {
		lib, reg, home := newTestLib(t)
		lib.Manifest.Targets.Default = []string{}
		agents := filepath.Join(home, ".agents", "skills")
		mine := filepath.Join(agents, "mine")
		writeSkill(t, mine, "mine")
		h, _ := fsutil.HashTree(mine)
		lib.Lock.Links["agents/mine"] = &lock.Link{Mode: "copy", Source: "skills/mine", Hash: h}
		dot := filepath.Join(home, "dotfiles", "skills", "dot")
		writeSkill(t, dot, "dot")
		os.Symlink(dot, filepath.Join(agents, "dot"))
		lib.Lock.Links["agents/dot"] = &lock.Link{Mode: "link", Source: "skills/dot"}
		entries, _, _ := Resolve(lib, reg)
		plan := MakePlan(lib, entries, force)
		for _, o := range plan.Ops {
			if o.Action != "skip" || !strings.Contains(o.Reason, "cannot tell it made it") {
				t.Fatalf("force=%v: %+v", force, o)
			}
		}
		res, err := Apply(lib, plan)
		if err != nil || len(res.Backups) != 0 {
			t.Fatalf("force=%v: %v %v", force, res.Backups, err)
		}
		if !fileExists(filepath.Join(mine, "SKILL.md")) {
			t.Fatalf("force=%v: the user's directory moved", force)
		}
		if l, err := os.Readlink(filepath.Join(agents, "dot")); err != nil || l != dot {
			t.Fatalf("force=%v: the user's link changed: %q %v", force, l, err)
		}
	}
}

// After the library moved, a link skd made at its previous location that
// now leads nowhere holds only that path: apply removes it outright (it
// was made before skd kept a placed record), whether the skill is still
// wanted (a new link replaces it) or not. One that still leads somewhere
// is never removed without the record.
func TestStaleDanglingLinkRemovedOutright(t *testing.T) {
	for _, wanted := range []bool{true, false} {
		lib, reg, home := newTestLib(t)
		src := filepath.Join(lib.Root, "skills", "s")
		writeSkill(t, src, "s")
		if !wanted {
			lib.Manifest.Targets.Default = []string{}
		}
		agents := filepath.Join(home, ".agents", "skills")
		os.MkdirAll(agents, 0o755)
		dest := filepath.Join(agents, "s")
		os.Symlink(filepath.Join(home, "old-library", "skills", "s"), dest)
		lib.Lock.Links["agents/s"] = &lock.Link{Mode: "link", Source: "skills/s"}
		entries, _, _ := Resolve(lib, reg)
		plan := MakePlan(lib, entries, false)
		if len(plan.Ops) != 1 || !strings.Contains(plan.Ops[0].Reason, filepath.Join(home, "old-library")) {
			t.Fatalf("wanted %v: %+v", wanted, plan.Ops)
		}
		res, err := Apply(lib, plan)
		if err != nil || len(res.Backups) != 0 {
			t.Fatalf("wanted %v: %v %v", wanted, res.Backups, err)
		}
		if items, _ := ListTrash(lib, reg); len(items) != 0 {
			t.Fatalf("wanted %v: trashed %+v", wanted, items)
		}
		if got := isLinkTo(dest, src); got != wanted {
			t.Fatalf("wanted %v: linked %v", wanted, got)
		}
		if !wanted && fsutil.Lexists(dest) {
			t.Fatal("stale link left")
		}
	}
}
