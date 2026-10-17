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
// record, is recorded only by the first command after the upgrade
// (AdoptOlderAppLinks, run by app.Open before anything changes
// skills.toml), never by a later apply: skills.toml and skills.lock
// come with the library and could name any link of the user's. Links
// into the library made by an older skd are recorded by the next apply,
// so they stay known as skd's when the library moves.
func TestAppLinksAndOlderLinks(t *testing.T) {
	for _, mode := range []string{"recorded", "upgraded, first command", "upgraded, applied", "upgraded, edited by hand"} {
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
		if mode != "recorded" {
			os.Remove(filepath.Join(home, "placed.json"))
		}
		switch mode {
		case "upgraded, first command":
			entries, _, _ = Resolve(lib, reg)
			if n, _ := AdoptOlderAppLinks(lib, entries); n != 1 {
				t.Fatalf("adopted %d", n)
			}
		case "upgraded, applied":
			// An apply with nothing to change records the library's link,
			// not the app's.
			entries, _, _ = Resolve(lib, reg)
			plan := MakePlan(lib, entries, false)
			if plan.Changes() {
				t.Fatalf("changes: %+v", plan.ChangeList())
			}
			Remember(lib, plan)
			m := loadPlaced()
			if e, ok := m[filepath.Join(home, ".agents", "skills", "s")]; !ok || e.Link == "" {
				t.Fatalf("library link not recorded: %v", m)
			}
			if _, ok := m[dest]; ok || len(m) != 1 {
				t.Fatalf("app link recorded by an apply: %v", m)
			}
		}
		lib.Manifest.Apps = nil
		entries, _, _ = Resolve(lib, reg)
		if mode == "upgraded, edited by hand" {
			// [[app]] taken out of skills.toml before skd ran: nothing
			// names the app any more, so nothing is adopted.
			if n, _ := AdoptOlderAppLinks(lib, entries); n != 0 {
				t.Fatalf("adopted %d", n)
			}
		}
		plan := MakePlan(lib, entries, true)
		var op Op
		for _, o := range plan.Ops {
			if o.Name == "x" {
				op = o
			}
		}
		if _, err := Apply(lib, plan); err != nil {
			t.Fatal(err)
		}
		switch mode {
		case "recorded", "upgraded, first command":
			if op.Action != "remove" || fsutil.Lexists(dest) {
				t.Fatalf("%s: %+v", mode, op)
			}
		default:
			// Left in place, said in the plan; nothing calls it the app's.
			if op.Action != "skip" || !isLinkTo(dest, app) || strings.Contains(op.Reason, "app") {
				t.Fatalf("%s: %+v", mode, op)
			}
		}
		if items, _ := ListTrash(lib, reg); len(items) != 0 {
			t.Fatalf("trashed %+v", items)
		}
		if !fileExists(filepath.Join(app, "SKILL.md")) {
			t.Fatal("the app's skill went")
		}
	}
}

// An app's link is adopted only while it is exactly what skd would
// make, for an app skills.toml wants in that target, and only while
// skd has kept no placed file yet: a forged record (and an [[app]]
// naming the user's directory) cannot claim the user's own link to
// something else, a link to an app directory that is not a skill, a
// link to a link, or anything in an agent's skill directory or the
// library.
func TestAppLinkRecordedOnlyWhenExact(t *testing.T) {
	for _, tc := range []string{"relative link", "record names another source", "not a skill", "not wanted there",
		"placed file exists", "source is a link", "source through a link into an agent's directory", "source in the library", "exact"} {
		lib, reg, home := newTestLib(t)
		app := filepath.Join(home, "Apps", "X.app", "x")
		writeSkill(t, app, "x")
		agents := filepath.Join(home, ".agents", "skills")
		os.MkdirAll(agents, 0o755)
		dest := filepath.Join(agents, "x")
		src := app
		target := app
		prev := &lock.Link{Mode: "link", Source: app}
		var targets *[]string
		switch tc {
		case "relative link":
			target, _ = filepath.Rel(agents, app)
		case "record names another source":
			prev.Source = filepath.Join(home, "elsewhere")
		case "not a skill":
			os.Remove(filepath.Join(app, "SKILL.md"))
		case "not wanted there":
			targets = &[]string{"claude"}
		case "placed file exists":
			os.WriteFile(filepath.Join(home, "placed.json"), []byte(`{"version":1,"placed":{}}`), 0o644)
		case "source is a link":
			src = filepath.Join(home, "Apps", "link")
			os.Symlink(app, src)
		case "source through a link into an agent's directory":
			real := filepath.Join(home, ".claude", "skills", "real")
			writeSkill(t, real, "real")
			os.Symlink(filepath.Join(home, ".claude", "skills"), filepath.Join(home, "Apps", "via"))
			src = filepath.Join(home, "Apps", "via", "real")
		case "source in the library":
			src = filepath.Join(lib.Root, "skills", "x")
			writeSkill(t, src, "x")
		}
		if src != app {
			prev.Source, target = src, src
		}
		os.Symlink(target, dest)
		lib.Lock.Links["agents/x"] = prev
		lib.Manifest.Apps = append(lib.Manifest.Apps, &manifest.App{Name: "x", Path: src, Targets: targets})
		entries, _, _ := Resolve(lib, reg)
		n, _ := AdoptOlderAppLinks(lib, entries)
		_, ok := loadPlaced()[dest]
		if want := tc == "exact"; ok != want || (n == 1) != want {
			t.Fatalf("%s: recorded %v (%d)", tc, ok, n)
		}
		if tc != "placed file exists" && OlderSkd() {
			t.Fatalf("%s: placed file not written", tc)
		}
	}
}

// A copy in a copy target made by an older skd (no record) is recorded
// by the next apply while it is exactly what skd copies from the source
// now; then a change of source replaces it outright. One with anything
// added since (a .git, notes) is not recorded, and goes to the trash.
func TestOlderCopyRecordedWhenPristine(t *testing.T) {
	for _, extra := range []bool{false, true} {
		lib, _, home := newTestLib(t)
		reg := customRegistry(t, map[string]struct{ Path, Mode string }{"box": {Path: filepath.Join(home, "box"), Mode: "copy"}})
		SetRegistry(reg)
		lib.Manifest.Targets.Default = []string{"box"}
		src := filepath.Join(lib.Root, "skills", "s")
		writeSkill(t, src, "s")
		entries, _, _ := Resolve(lib, reg)
		if _, err := Apply(lib, MakePlan(lib, entries, false)); err != nil {
			t.Fatal(err)
		}
		os.Remove(filepath.Join(home, "placed.json"))
		cp := filepath.Join(home, "box", "s")
		if extra {
			os.MkdirAll(filepath.Join(cp, ".git"), 0o755)
			os.WriteFile(filepath.Join(cp, ".git", "HEAD"), []byte("ref"), 0o644)
		}
		entries, _, _ = Resolve(lib, reg)
		plan := MakePlan(lib, entries, false)
		if plan.Changes() {
			t.Fatalf("changes %+v", plan.ChangeList())
		}
		Remember(lib, plan)
		if _, ok := loadPlaced()[cp]; ok == extra {
			t.Fatalf("extra %v: recorded %v", extra, ok)
		}
		os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: s\n---\nnew\n"), 0o644)
		entries, _, _ = Resolve(lib, reg)
		res, err := Apply(lib, MakePlan(lib, entries, false))
		if err != nil {
			t.Fatal(err)
		}
		if got := len(res.Backups); got != map[bool]int{false: 0, true: 1}[extra] {
			t.Fatalf("extra %v: backups %v", extra, res.Backups)
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
