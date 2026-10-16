package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/lock"
)

// TestMakePlanTable covers the plan decision for every combination that
// matters: what the lock says (prev), what is on disk (dest) and --force,
// including a library that was moved since the links were made. After
// each plan is applied, a directory holding user data must still exist,
// either in place or in the trash.
func TestMakePlanTable(t *testing.T) {
	type paths struct{ src, oldSrc, movedSrc string }
	var (
		absent  = func(paths, string) {}
		toSrc   = func(p paths, d string) { os.Symlink(p.src, d) }
		toOld   = func(p paths, d string) { os.Symlink(p.oldSrc, d) }
		toMoved = func(p paths, d string) { os.Symlink(p.movedSrc, d) }
		toElse  = func(p paths, d string) { os.Symlink("/elsewhere", d) }
		userDir = func(p paths, d string) {
			writeSkill(t, d, "s")
			os.WriteFile(filepath.Join(d, "mine.txt"), []byte("x"), 0o644)
		}
		copyOf = func(p paths, d string) { fsutil.CopyTree(p.src, d) }
		edited = func(p paths, d string) {
			fsutil.CopyTree(p.src, d)
			os.WriteFile(filepath.Join(d, "SKILL.md"), []byte("edited"), 0o644)
		}
	)
	link := func(src string) func(paths) *lock.Link {
		return func(paths) *lock.Link { return &lock.Link{Mode: "link", Source: src} }
	}
	copyPrev := func(p paths) *lock.Link {
		h, _ := fsutil.HashTree(p.src)
		return &lock.Link{Mode: "copy", Source: "skills/s", Hash: h}
	}
	none := func(paths) *lock.Link { return nil }

	cases := []struct {
		name    string
		copy    bool // copy-mode target instead of a link target
		desired bool
		prev    func(paths) *lock.Link
		dest    func(paths, string)
		want    string // action without --force
		wantF   string // action with --force
		note    string // reason substring without --force
	}{
		{"new", false, true, none, absent, "create", "create", ""},
		{"adopt existing link", false, true, none, toSrc, "create", "create", "adopting"},
		{"unmanaged link", false, true, none, toElse, "skip", "replace", "not managed"},
		{"unmanaged dir", false, true, none, userDir, "skip", "replace", "not managed"},
		{"ours, current", false, true, link("skills/s"), toSrc, "keep", "keep", ""},
		{"ours, source changed", false, true, link("old/s"), toOld, "replace", "replace", "source changed"},
		{"ours, relinked elsewhere", false, true, link("skills/s"), toElse, "skip", "replace", "modified"},
		{"ours, replaced by dir", false, true, link("skills/s"), userDir, "skip", "replace", "modified"},
		{"ours, gone", false, true, link("skills/s"), absent, "create", "create", ""},
		{"library moved", false, true, link("skills/s"), toMoved, "replace", "replace", "previous library location"},
		{"copy, pristine", true, true, copyPrev, copyOf, "keep", "keep", ""},
		{"copy, edited", true, true, copyPrev, edited, "skip", "replace", "modified"},
		{"undesired, current", false, false, link("skills/s"), toSrc, "remove", "remove", ""},
		{"undesired, gone", false, false, link("skills/s"), absent, "remove", "remove", "already gone"},
		{"undesired, replaced by dir", false, false, link("skills/s"), userDir, "skip", "remove", "modified"},
		{"undesired, library moved", false, false, link("skills/s"), toMoved, "remove", "remove", "previous library location"},
	}
	for _, tc := range cases {
		for _, force := range []bool{false, true} {
			lib, _, home := newTestLib(t)
			reg := customRegistry(t, map[string]struct{ Path, Mode string }{"box": {Path: filepath.Join(home, "box"), Mode: "copy"}})
			SetRegistry(reg)
			target, dir := "agents", filepath.Join(home, ".agents", "skills")
			if tc.copy {
				target, dir = "box", filepath.Join(home, "box")
			}
			lib.Manifest.Targets.Default = []string{target}
			if !tc.desired {
				lib.Manifest.Targets.Default = []string{}
			}
			p := paths{src: filepath.Join(lib.Root, "skills", "s"), oldSrc: filepath.Join(lib.Root, "old", "s"),
				movedSrc: filepath.Join(home, "previous-library", "skills", "s")}
			writeSkill(t, p.src, "s")
			writeSkill(t, p.oldSrc, "s")
			writeSkill(t, p.movedSrc, "s")
			os.MkdirAll(dir, 0o755)
			dest := filepath.Join(dir, "s")
			tc.dest(p, dest)
			if prev := tc.prev(p); prev != nil {
				lib.Lock.Links[target+"/s"] = prev
			}

			entries, _, _ := Resolve(lib, reg)
			plan := MakePlan(lib, entries, force)
			var got *Op
			for i := range plan.Ops {
				if plan.Ops[i].Name == "s" {
					got = &plan.Ops[i]
				}
			}
			want := tc.want
			if force {
				want = tc.wantF
			}
			label := tc.name
			if force {
				label += " (--force)"
			}
			if got == nil {
				t.Errorf("%s: no op, want %s", label, want)
				continue
			}
			if got.Action != want {
				t.Errorf("%s: %s (%s), want %s", label, got.Action, got.Reason, want)
			}
			if !force && !strings.Contains(got.Reason, tc.note) {
				t.Errorf("%s: reason %q, want it to mention %q", label, got.Reason, tc.note)
			}
			if _, err := Apply(lib, plan); err != nil {
				t.Errorf("%s: apply: %v", label, err)
			}
			if strings.Contains(tc.name, "dir") && !fsutil.Exists(filepath.Join(dest, "mine.txt")) {
				if m, _ := filepath.Glob(filepath.Join(lib.Root, TrashDir, "*", "s", "mine.txt")); len(m) == 0 {
					t.Errorf("%s: user directory neither kept nor in the trash", label)
				}
			}
		}
	}
}
