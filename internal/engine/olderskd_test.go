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

// planFor plans lib and returns the op for "<target>/<name>".
func planFor(t *testing.T, lib0 func() Plan, target, name string) (Plan, Op) {
	t.Helper()
	plan := lib0()
	for _, o := range plan.Ops {
		if o.Target.Name == target && o.Name == name {
			return plan, o
		}
	}
	t.Fatalf("no op for %s/%s: %+v", target, name, plan.Ops)
	return plan, Op{}
}

// The first command after upgrading from an skd that kept no placed
// record (app.Open runs AdoptOlderAppLinks before the command changes
// skills.toml) may disable an app (all agents, or one): its links,
// exactly as the older skd made them, are removed outright as it did,
// not left behind or moved to the trash. So is an [[app]] taken out of
// skills.toml by hand after skd ran once since the upgrade; taken out
// before, nothing names the app any more and its links are left in
// place. The app's files are not touched.
func TestOlderAppLinksRemovedOnFirstDisable(t *testing.T) {
	for _, how := range []string{"app removed after a command", "app removed before any command", "disabled", "disabled for one agent"} {
		lib, reg, home := newTestLib(t)
		app := filepath.Join(home, "Applications", "X.app", "Skills", "x")
		writeSkill(t, app, "x")
		os.WriteFile(filepath.Join(app, "notes.txt"), []byte("app data"), 0o644)
		lib.Manifest.Targets.Default = []string{"agents", "claude"}
		for _, tn := range []string{"agents", "claude"} {
			dir := filepath.Join(home, "."+tn, "skills")
			os.MkdirAll(dir, 0o755)
			os.Symlink(app, filepath.Join(dir, "x"))
			lib.Lock.Links[tn+"/x"] = &lock.Link{Mode: "link", Source: app, Path: filepath.Join(dir, "x")}
		}
		lib.Manifest.Apps = []*manifest.App{{Name: "x", Path: app}}
		if how != "app removed before any command" {
			entries, _, _ := Resolve(lib, reg)
			if n, _ := AdoptOlderAppLinks(lib, entries); n != 2 {
				t.Fatalf("%s: adopted %d", how, n)
			}
		}
		gone := []string{"agents", "claude"}
		switch how {
		case "app removed after a command", "app removed before any command":
			lib.Manifest.Apps = nil
		case "disabled":
			lib.Manifest.Apps = []*manifest.App{{Name: "x", Path: app, Targets: &[]string{}}}
		case "disabled for one agent":
			lib.Manifest.Apps = []*manifest.App{{Name: "x", Path: app, Targets: &[]string{"agents"}}}
			gone = []string{"claude"}
		}
		entries, _, _ := Resolve(lib, reg)
		if how == "app removed before any command" {
			AdoptOlderAppLinks(lib, entries)
		}
		plan := MakePlan(lib, entries, false)
		for _, tn := range gone {
			_, op := planFor(t, func() Plan { return plan }, tn, "x")
			want := "remove"
			if how == "app removed before any command" {
				want = "skip"
			}
			if op.Action != want || strings.Contains(op.Reason, "app") {
				t.Fatalf("%s: %s: %+v", how, tn, op)
			}
		}
		res, err := Apply(lib, plan)
		if err != nil || len(res.Backups) != 0 {
			t.Fatalf("%s: %v %v", how, res.Backups, err)
		}
		for _, tn := range gone {
			if fsutil.Lexists(filepath.Join(home, "."+tn, "skills", "x")) != (how == "app removed before any command") {
				t.Fatalf("%s: the link in %s", how, tn)
			}
		}
		if how == "disabled for one agent" && !isLinkTo(filepath.Join(home, ".agents", "skills", "x"), app) {
			t.Fatalf("%s: the wanted link went", how)
		}
		if b, err := os.ReadFile(filepath.Join(app, "notes.txt")); err != nil || string(b) != "app data" {
			t.Fatalf("%s: the app's files changed", how)
		}
	}
}

// A forged lock record of an app's link no [[app]] names (the library
// is someone else's, cloned and applied by an agent): the user's own
// link it names, `ln -s ~/projects/my-skill ~/.claude/skills/my-skill`,
// a directory with a SKILL.md, is left exactly as it is, never removed
// or moved to the trash, and the plan does not call it skd's or an
// app's.
func TestForgedAppRecordLeavesTheUsersLink(t *testing.T) {
	for _, force := range []bool{false, true} {
		lib, reg, home := newTestLib(t)
		lib.Manifest.Targets.Default = []string{}
		mine := filepath.Join(home, "projects", "my-skill")
		writeSkill(t, mine, "my-skill")
		claude := filepath.Join(home, ".claude", "skills")
		os.MkdirAll(claude, 0o755)
		dest := filepath.Join(claude, "my-skill")
		os.Symlink(mine, dest)
		lib.Lock.Links["claude/my-skill"] = &lock.Link{Mode: "link", Source: mine}
		entries, _, _ := Resolve(lib, reg)
		if n, _ := AdoptOlderAppLinks(lib, entries); n != 0 {
			t.Fatalf("adopted %d", n)
		}
		plan, op := planFor(t, func() Plan { return MakePlan(lib, entries, force) }, "claude", "my-skill")
		if op.Action != "skip" || strings.Contains(op.Reason, "app") || !strings.Contains(op.Reason, "may be the user's own") {
			t.Fatalf("force %v: %+v", force, op)
		}
		if res, err := Apply(lib, plan); err != nil || len(res.Backups) != 0 {
			t.Fatalf("%v %v", res, err)
		}
		if got, err := os.Readlink(dest); err != nil || got != mine {
			t.Fatalf("force %v: the user's link changed: %q %v", force, got, err)
		}
	}
}

// The user's link from one agent's directory to another's entry, which
// is skd's own link into the library (~/.claude/skills/mine ->
// ~/.agents/skills/mine): a forged record naming the latter as an
// app's source, with or without an [[app]] naming it too, never has
// skd take the user's link for an app's. The source is itself a link
// and is in an agent's directory.
func TestForgedAppRecordThroughAnAgentLink(t *testing.T) {
	for _, declared := range []bool{false, true} {
		lib, reg, home := newTestLib(t)
		writeSkill(t, filepath.Join(lib.Root, "skills", "mine"), "mine")
		entries, _, _ := Resolve(lib, reg)
		if _, err := Apply(lib, MakePlan(lib, entries, false)); err != nil {
			t.Fatal(err)
		}
		os.Remove(filepath.Join(home, "placed.json"))
		via := filepath.Join(home, ".agents", "skills", "mine")
		claude := filepath.Join(home, ".claude", "skills")
		os.MkdirAll(claude, 0o755)
		dest := filepath.Join(claude, "mine")
		os.Symlink(via, dest)
		lib.Lock.Links["claude/mine"] = &lock.Link{Mode: "link", Source: via}
		if declared {
			lib.Manifest.Apps = []*manifest.App{{Name: "mine", Path: via, Targets: &[]string{"claude"}}}
		}
		entries, _, _ = Resolve(lib, reg)
		if n, _ := AdoptOlderAppLinks(lib, entries); n != 0 {
			t.Fatalf("declared %v: adopted %d", declared, n)
		}
		lib.Manifest.Apps = nil
		entries, _, _ = Resolve(lib, reg)
		plan, op := planFor(t, func() Plan { return MakePlan(lib, entries, true) }, "claude", "mine")
		if op.Action != "skip" {
			t.Fatalf("declared %v: %+v", declared, op)
		}
		if res, err := Apply(lib, plan); err != nil || len(res.Backups) != 0 {
			t.Fatalf("%v %v", res, err)
		}
		if got, err := os.Readlink(dest); err != nil || got != via {
			t.Fatalf("declared %v: the user's link changed: %q %v", declared, got, err)
		}
	}
}

// A forged lock record of a link nothing wants never has skd remove a
// link of the user's or anything else, whatever it is: everything
// stays where it is, with --force too.
func TestForgedAppRecordLeavesOtherLinks(t *testing.T) {
	for _, c := range []string{"exactly an app's link", "relative link", "into an agent's directory", "no SKILL.md", "SKILL.md is a link", "keys", "spelled differently", "copy record", "a directory"} {
		for _, force := range []bool{false, true} {
			lib, reg, home := newTestLib(t)
			lib.Manifest.Targets.Default = []string{}
			agents := filepath.Join(home, ".agents", "skills")
			src := filepath.Join(home, "code", "tool", "skill")
			writeSkill(t, src, "s")
			dest := filepath.Join(agents, "mine")
			os.MkdirAll(agents, 0o755)
			rec := &lock.Link{Mode: "link", Source: src}
			to := src
			switch c {
			case "relative link":
				to = filepath.Join("..", "..", "code", "tool", "skill")
			case "into an agent's directory":
				src = filepath.Join(home, ".claude", "skills", "dot")
				writeSkill(t, src, "dot")
				rec.Source, to = src, src
			case "no SKILL.md":
				os.Remove(filepath.Join(src, "SKILL.md"))
			case "SKILL.md is a link":
				os.Remove(filepath.Join(src, "SKILL.md"))
				os.WriteFile(filepath.Join(home, "elsewhere.md"), []byte("---\nname: s\n---\n"), 0o644)
				os.Symlink(filepath.Join(home, "elsewhere.md"), filepath.Join(src, "SKILL.md"))
			case "keys":
				src = filepath.Join(home, ".ssh")
				writeSkill(t, src, "ssh")
				rec.Source, to = src, src
			case "spelled differently":
				to = src + string(filepath.Separator)
			case "copy record":
				rec.Mode = "copy"
			}
			if c == "a directory" {
				writeSkill(t, dest, "mine")
			} else if err := os.Symlink(to, dest); err != nil {
				t.Fatal(err)
			}
			lib.Lock.Links["agents/mine"] = rec
			entries, _, _ := Resolve(lib, reg)
			plan, op := planFor(t, func() Plan { return MakePlan(lib, entries, force) }, "agents", "mine")
			if op.Action != "skip" {
				t.Fatalf("%s (force %v): %+v", c, force, op)
			}
			if res, err := Apply(lib, plan); err != nil || len(res.Backups) != 0 {
				t.Fatalf("%s: %v %v", c, res, err)
			}
			if c == "a directory" {
				if !fileExists(filepath.Join(dest, "SKILL.md")) {
					t.Fatalf("%s: the user's directory went", c)
				}
			} else if got, err := os.Readlink(dest); err != nil || got != to {
				t.Fatalf("%s (force %v): the user's link changed: %q %v", c, force, got, err)
			}
		}
	}
}

// A link that leads nowhere is taken for one skd made before the
// library moved only when the old root itself is gone from a directory
// that is still there (mv ~/lib ~/lib2): not when the directory it was
// in is gone too (a volume not mounted) or when the old root is still
// there without the skill.
func TestStaleDanglingOnlyWhenTheLibraryMoved(t *testing.T) {
	for _, c := range []struct {
		name   string
		to     func(home string) string
		direct bool
	}{
		{"library moved", func(h string) string { return filepath.Join(h, "old-lib", "skills", "s") }, true},
		{"volume not mounted", func(h string) string { return filepath.Join(h, "Volumes", "Unmounted", "foo", "skills", "s") }, false},
		{"old root there, skill gone", func(h string) string {
			os.MkdirAll(filepath.Join(h, "project", "skills"), 0o755)
			return filepath.Join(h, "project", "skills", "s")
		}, false},
	} {
		for _, wanted := range []bool{false, true} {
			lib, reg, home := newTestLib(t)
			writeSkill(t, filepath.Join(lib.Root, "skills", "s"), "s")
			if !wanted {
				lib.Manifest.Targets.Default = []string{}
			}
			agents := filepath.Join(home, ".agents", "skills")
			os.MkdirAll(agents, 0o755)
			dest := filepath.Join(agents, "s")
			to := c.to(home)
			os.Symlink(to, dest)
			lib.Lock.Links["agents/s"] = &lock.Link{Mode: "link", Source: "skills/s", Path: dest}
			entries, _, _ := Resolve(lib, reg)
			plan, op := planFor(t, func() Plan { return MakePlan(lib, entries, false) }, "agents", "s")
			res, err := Apply(lib, plan)
			if err != nil {
				t.Fatal(err)
			}
			label := c.name
			if wanted {
				label += ", wanted"
			}
			switch {
			case c.direct:
				if len(res.Backups) != 0 || (!wanted && fsutil.Lexists(dest)) {
					t.Fatalf("%s: %+v %v", label, op, res.Backups)
				}
			case wanted:
				// Replaced, the old link moved to the trash.
				if op.Action != "replace" || len(res.Backups) != 1 {
					t.Fatalf("%s: %+v %v", label, op, res.Backups)
				}
			default:
				if op.Action != "skip" || len(res.Backups) != 0 {
					t.Fatalf("%s: %+v %v", label, op, res.Backups)
				}
				if got, err := os.Readlink(dest); err != nil || got != to {
					t.Fatalf("%s: the link changed: %q %v", label, got, err)
				}
			}
		}
	}
}

// cp -R lib lib2, then skd use lib2: the links an older skd made from
// lib (with no placed record) are relinked and removed outright, as
// that skd did, while lib's own skills.lock records them and lib still
// has the skill. Without that, they go to the trash (wanted) or stay
// (not wanted): the current lock may be forged, the old library's is
// not part of it.
func TestLinksOfACopiedLibrary(t *testing.T) {
	for _, c := range []string{"recorded", "not recorded", "another path", "another source", "lock is a link", "no skills.toml", "inside this library",
		"skill gone from it", "skill is a file there", "skill reached through a link there"} {
		for _, wanted := range []bool{true, false} {
			lib, reg, home := newTestLib(t)
			writeSkill(t, filepath.Join(lib.Root, "skills", "s"), "s")
			if !wanted {
				lib.Manifest.Targets.Default = []string{}
			}
			old := filepath.Join(home, "old-lib")
			if c == "inside this library" {
				// Its lock comes with this library: no evidence.
				old = filepath.Join(lib.Root, "nested")
			}
			writeSkill(t, filepath.Join(old, "skills", "s"), "s")
			writeSkill(t, filepath.Join(old, "skills", "t"), "t")
			agents := filepath.Join(home, ".agents", "skills")
			os.MkdirAll(agents, 0o755)
			dest := filepath.Join(agents, "s")
			os.Symlink(filepath.Join(old, "skills", "s"), dest)
			rec := &lock.Link{Mode: "link", Source: "skills/s", Path: dest}
			lib.Lock.Links["agents/s"] = rec
			oldRec := *rec
			switch c {
			case "another path":
				oldRec.Path = filepath.Join(home, ".claude", "skills", "s")
			case "another source":
				oldRec.Source = "skills/t"
			}
			ol := &lock.Lock{Version: 1, Packs: map[string]*lock.Pack{}, Links: map[string]*lock.Link{"agents/s": &oldRec}}
			if c == "not recorded" {
				ol.Links = map[string]*lock.Link{}
			}
			lockAt := filepath.Join(old, lock.FileName)
			if c == "lock is a link" {
				lockAt = filepath.Join(home, "elsewhere.lock")
				os.Symlink(lockAt, filepath.Join(old, lock.FileName))
			}
			if err := ol.Save(lockAt); err != nil {
				t.Fatal(err)
			}
			if c != "no skills.toml" {
				os.WriteFile(filepath.Join(old, manifest.FileName), []byte("[targets]\n"), 0o644)
			}
			switch c {
			case "skill gone from it":
				os.RemoveAll(filepath.Join(old, "skills", "s"))
			case "skill is a file there":
				os.RemoveAll(filepath.Join(old, "skills", "s"))
				os.WriteFile(filepath.Join(old, "skills", "s"), []byte("x"), 0o644)
			case "skill reached through a link there":
				// old/skills is a link to a directory with the skill: the
				// link to old/skills/s is not one skd made from old.
				writeSkill(t, filepath.Join(home, "elsewhere", "s"), "s")
				os.RemoveAll(filepath.Join(old, "skills"))
				os.Symlink(filepath.Join(home, "elsewhere"), filepath.Join(old, "skills"))
			}
			entries, _, _ := Resolve(lib, reg)
			plan, op := planFor(t, func() Plan { return MakePlan(lib, entries, false) }, "agents", "s")
			res, err := Apply(lib, plan)
			if err != nil {
				t.Fatal(err)
			}
			label := c
			if wanted {
				label += ", wanted"
			}
			switch {
			case c == "recorded":
				if len(res.Backups) != 0 {
					t.Fatalf("%s: trashed %v", label, res.Backups)
				}
				if wanted != isLinkTo(dest, filepath.Join(lib.Root, "skills", "s")) || (!wanted && fsutil.Lexists(dest)) {
					t.Fatalf("%s: %+v", label, op)
				}
			case wanted:
				if op.Action != "replace" || len(res.Backups) != 1 {
					t.Fatalf("%s: %+v %v", label, op, res.Backups)
				}
			default:
				if op.Action != "skip" || !isLinkTo(dest, filepath.Join(old, "skills", "s")) {
					t.Fatalf("%s: %+v", label, op)
				}
			}
			if !strings.HasPrefix(c, "skill ") && !fileExists(filepath.Join(old, "skills", "s", "SKILL.md")) {
				t.Fatalf("%s: the old library's skill went", label)
			}
		}
	}
}

// A link nothing in skills.lock can confirm is moved to the trash when a
// wanted skill takes its place with --force, even when it looks exactly
// like an app's link: a forged record must not turn the user's link
// into one skd removes.
func TestForcedReplaceOfAnAppLikeLinkGoesToTheTrash(t *testing.T) {
	lib, reg, home := newTestLib(t)
	writeSkill(t, filepath.Join(lib.Root, "skills", "mine"), "mine")
	src := filepath.Join(home, "code", "tool", "skill")
	writeSkill(t, src, "s")
	agents := filepath.Join(home, ".agents", "skills")
	os.MkdirAll(agents, 0o755)
	dest := filepath.Join(agents, "mine")
	os.Symlink(src, dest)
	lib.Lock.Links["agents/mine"] = &lock.Link{Mode: "link", Source: src}
	entries, _, _ := Resolve(lib, reg)
	plan, op := planFor(t, func() Plan { return MakePlan(lib, entries, true) }, "agents", "mine")
	if op.Action != "replace" {
		t.Fatalf("%+v", op)
	}
	res, err := Apply(lib, plan)
	if err != nil || len(res.Backups) != 1 {
		t.Fatalf("%v %v", res, err)
	}
}

// A placed file that cannot be written (its directory read-only): the
// app's links an older skd made are recorded for the rest of the
// process, so a disable in it removes them outright; the window stays
// shut in that process (looked for once; shut by SealPlaced) though no
// file says so, and the same file set again (every app.Open) keeps
// both. Another placed file starts over.
func TestOlderAppLinksWithAPlacedFileThatCannotBeWritten(t *testing.T) {
	for _, how := range []string{"adopted", "sealed"} {
		lib, reg, home := newTestLib(t)
		ro := filepath.Join(home, "ro")
		os.MkdirAll(ro, 0o755)
		p := filepath.Join(ro, "placed.json")
		SetPlacedFile(p)
		app := filepath.Join(home, "Applications", "X.app", "Skills", "x")
		writeSkill(t, app, "x")
		dir := filepath.Join(home, ".agents", "skills")
		os.MkdirAll(dir, 0o755)
		dest := filepath.Join(dir, "x")
		os.Symlink(app, dest)
		lib.Lock.Links["agents/x"] = &lock.Link{Mode: "link", Source: app, Path: dest}
		lib.Manifest.Apps = []*manifest.App{{Name: "x", Path: app}}
		if err := os.Chmod(ro, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(ro, 0o755) })
		entries, _, _ := Resolve(lib, reg)
		if how == "sealed" {
			SealPlaced(p)
			if OlderSkd() {
				t.Fatalf("%s: the window is still open", how)
			}
			if n, _ := AdoptOlderAppLinks(lib, entries); n != 0 {
				t.Fatalf("%s: adopted %d", how, n)
			}
			continue
		}
		n, err := AdoptOlderAppLinks(lib, entries)
		if n != 1 || err == nil {
			t.Fatalf("%s: adopted %d, %v", how, n, err)
		}
		SetPlacedFile(p)
		if OlderSkd() {
			t.Fatalf("%s: the window is open again", how)
		}
		lib.Manifest.Apps = []*manifest.App{{Name: "x", Path: app, Targets: &[]string{}}}
		entries, _, _ = Resolve(lib, reg)
		_, op := planFor(t, func() Plan { return MakePlan(lib, entries, false) }, "agents", "x")
		if op.Action != "remove" {
			t.Fatalf("%s: %+v", how, op)
		}
		SetPlacedFile(filepath.Join(home, "other.json"))
		SetPlacedFile(p)
		if !OlderSkd() {
			t.Fatalf("%s: another placed file did not start over", how)
		}
		if _, op := planFor(t, func() Plan { return MakePlan(lib, entries, false) }, "agents", "x"); op.Action != "skip" {
			t.Fatalf("%s: started over yet %+v", how, op)
		}
	}
}
