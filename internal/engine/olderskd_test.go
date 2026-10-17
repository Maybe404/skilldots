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
// record may disable an app (all agents, or one), or the [[app]] may be
// gone from skills.toml by hand: its links, exactly as the older skd
// made them, are removed outright as it did, not left behind or moved
// to the trash. The app's files are not touched.
func TestOlderAppLinksRemovedOnFirstDisable(t *testing.T) {
	for _, how := range []string{"app removed", "disabled", "disabled for one agent"} {
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
		gone := []string{"agents", "claude"}
		switch how {
		case "disabled":
			lib.Manifest.Apps = []*manifest.App{{Name: "x", Path: app, Targets: &[]string{}}}
		case "disabled for one agent":
			lib.Manifest.Apps = []*manifest.App{{Name: "x", Path: app, Targets: &[]string{"agents"}}}
			gone = []string{"claude"}
		}
		entries, _, _ := Resolve(lib, reg)
		plan := MakePlan(lib, entries, false)
		for _, tn := range gone {
			_, op := planFor(t, func() Plan { return plan }, tn, "x")
			if op.Action != "remove" || !strings.Contains(op.Reason, app) {
				t.Fatalf("%s: %s: %+v", how, tn, op)
			}
		}
		res, err := Apply(lib, plan)
		if err != nil || len(res.Backups) != 0 {
			t.Fatalf("%s: %v %v", how, res.Backups, err)
		}
		for _, tn := range gone {
			if fsutil.Lexists(filepath.Join(home, "."+tn, "skills", "x")) {
				t.Fatalf("%s: the link in %s is still there", how, tn)
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

// A forged lock record of a link nothing wants can have skd remove a
// link of the user's only when it is exactly an app's link: an
// absolute link, spelled exactly as the record's source, to a directory
// with a regular SKILL.md that is not a place of keys and not in an
// agent's skill directory. Everything else stays where it is, with
// --force too.
func TestForgedAppRecordLeavesOtherLinks(t *testing.T) {
	for _, c := range []string{"relative link", "into an agent's directory", "no SKILL.md", "SKILL.md is a link", "keys", "spelled differently", "copy record", "a directory"} {
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


// A link nothing in skills.lock can confirm is moved to the trash when a
// wanted skill takes its place with --force, even when it looks exactly
// like an app's link (appLink is only for links nothing wants): a
// forged record must not turn the user's link into one skd removes.
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
