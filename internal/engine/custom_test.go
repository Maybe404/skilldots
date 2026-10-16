package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/targets"
)

// customRegistry is the registry with custom targets a person approved
// on this (test) machine: HOME must already point at a temporary
// directory, where the approvals are written.
func customRegistry(t *testing.T, custom map[string]struct{ Path, Mode string }) *targets.Registry {
	t.Helper()
	for name, c := range custom {
		real, err := targets.CheckPath(c.Path)
		if err != nil {
			t.Fatal(err)
		}
		if err := targets.Approve(name, c.Path, real); err != nil {
			t.Fatal(err)
		}
	}
	reg, err := targets.NewRegistry(custom)
	if err != nil {
		t.Fatal(err)
	}
	if w := reg.Withheld(); len(w) > 0 {
		t.Fatalf("approved custom targets withheld: %+v", w)
	}
	return reg
}

// A custom target gets back from the trash only what apply could have put
// there: an entry named like a skill, of a target a person approved. A
// target in the home directory itself is never one. Built-in targets are
// as they were.
func TestRestoreIntoCustomTargets(t *testing.T) {
	lib, _, home := restoreLib(t)
	box := filepath.Join(home, "box")
	customRegistry(t, map[string]struct{ Path, Mode string }{"box": {Path: box}})
	reg, err := targets.NewRegistry(map[string]struct{ Path, Mode string }{
		"box": {Path: box}, "other": {Path: "~/other"}, "evil": {Path: "~"}, "agents": {Path: "~"},
	})
	if err != nil {
		t.Fatal(err)
	}
	SetRegistry(reg)
	agents := filepath.Join(home, ".agents", "skills")
	batch := filepath.Join(lib.Root, TrashDir, "20260101-000000")
	ssh := park(t, batch, ".ssh", filepath.Join(home, ".ssh"))
	os.WriteFile(filepath.Join(ssh, "authorized_keys"), []byte("ssh-ed25519 AAAA attacker\n"), 0o600)
	b2 := filepath.Join(lib.Root, TrashDir, "20260101-000001")
	park(t, b2, ".ssh", filepath.Join(box, ".ssh"))
	park(t, b2, ".hidden", filepath.Join(agents, ".hidden"))
	park(t, b2, "ok", filepath.Join(box, "ok"))
	park(t, b2, "x", filepath.Join(home, "other", "x"))
	park(t, b2, "Documents", filepath.Join(home, "Documents"))
	want := map[string]bool{
		"20260101-000000/.ssh": false, "20260101-000001/.ssh": false, "20260101-000001/x": false,
		"20260101-000001/Documents": false, "20260101-000001/.hidden": true, "20260101-000001/ok": true,
	}
	items, err := ListTrash(lib, reg)
	if err != nil || len(items) != len(want) {
		t.Fatalf("%+v %v", items, err)
	}
	for _, it := range items {
		if it.Restorable != want[it.ID] {
			t.Errorf("%s from %s: restorable %v (%s)", it.ID, it.Origin, it.Restorable, it.Problem)
		}
		dest, err := Restore(lib, reg, it.ID)
		if (err == nil) != want[it.ID] {
			t.Errorf("restore %s: %q %v", it.ID, dest, err)
		}
	}
	for _, p := range []string{filepath.Join(home, ".ssh"), filepath.Join(box, ".ssh"), filepath.Join(home, "other"), filepath.Join(home, "Documents")} {
		if fileExists(p) {
			t.Errorf("restore wrote %s", p)
		}
	}
	if !fileExists(filepath.Join(ssh, "authorized_keys")) {
		t.Error("the refused entry left the trash")
	}
}

// The lock's record of where a link is decides nothing on its own: apply
// --force moves only a direct child, named like a skill, of a target in
// use. A record pointing at ~/Documents never moves it, whatever target
// it is filed under.
func TestForgedLockPathsNeverMoveOutsideTargets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		custom map[string]struct{ Path, Mode string }
		trust  bool
		key    string
	}{
		{"custom target in home", map[string]struct{ Path, Mode string }{"evil": {Path: "~"}}, false, "evil/Documents"},
		{"custom target in home, copy", map[string]struct{ Path, Mode string }{"evil": {Path: "~", Mode: "copy"}}, false, "evil/Documents"},
		{"unapproved custom target", map[string]struct{ Path, Mode string }{"evil": {Path: "~/evilbox"}}, false, "evil/Documents"},
		{"approved custom target", map[string]struct{ Path, Mode string }{"evil": {Path: "~/evilbox"}}, true, "evil/Documents"},
		{"undefined target", nil, false, "ghost/Documents"},
		{"built-in target", nil, false, "agents/Documents"},
		{"name with a path", nil, false, "agents/../../Documents"},
		{"custom named like a built-in", map[string]struct{ Path, Mode string }{"agents": {Path: "~"}}, false, "agents/Documents"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lib, _, home := restoreLib(t)
			var reg *targets.Registry
			if tc.trust {
				reg = customRegistry(t, tc.custom)
			} else {
				reg, _ = targets.NewRegistry(tc.custom)
			}
			SetRegistry(reg)
			for n, c := range tc.custom {
				lib.Manifest.Targets.Custom[n] = manifest.CustomTarget{Path: c.Path, Mode: c.Mode}
			}
			docs := filepath.Join(home, "Documents")
			writeSkill(t, docs, "Documents")
			os.WriteFile(filepath.Join(docs, "thesis.txt"), []byte("precious"), 0o644)
			writeSkill(t, filepath.Join(lib.Root, "skills", "x"), "x")
			for _, mode := range []string{"link", "copy"} {
				lib.Lock.Links[tc.key] = &lock.Link{Mode: mode, Source: "skills/x", Path: docs, Hash: "h"}
				entries, _, err := Resolve(lib, reg)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := Apply(lib, MakePlan(lib, entries, true)); err != nil {
					t.Logf("apply: %v", err)
				}
				if b, err := os.ReadFile(filepath.Join(docs, "thesis.txt")); err != nil || string(b) != "precious" {
					t.Fatalf("%s (%s): apply --force took ~/Documents: %v", tc.key, mode, err)
				}
				if items, _ := ListTrash(lib, reg); len(items) != 0 {
					t.Fatalf("%s (%s): trashed %+v", tc.key, mode, items)
				}
			}
		})
	}
}

// A lock record filed under a target name not in use (never defined,
// defined but not approved, or refused) is only forgotten, with --force
// too, even when its path names an entry of a built-in target's
// directory: the user's own skill directory there stays.
func TestForgedLockRecordOfAnUnusedTargetIsOnlyForgotten(t *testing.T) {
	for _, tc := range []struct {
		name   string
		custom map[string]struct{ Path, Mode string }
	}{
		{"undefined", nil},
		{"not approved", map[string]struct{ Path, Mode string }{"evil": {Path: "~/evilbox/skills"}}},
		{"refused", map[string]struct{ Path, Mode string }{"evil": {Path: "~"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lib, _, home := restoreLib(t)
			reg, _ := targets.NewRegistry(tc.custom)
			SetRegistry(reg)
			for n, c := range tc.custom {
				lib.Manifest.Targets.Custom[n] = manifest.CustomTarget{Path: c.Path, Mode: c.Mode}
			}
			lib.Manifest.Targets.Default = []string{}
			mine := filepath.Join(home, ".claude", "skills", "mine")
			writeSkill(t, mine, "mine")
			os.WriteFile(filepath.Join(mine, "notes.md"), []byte("mine"), 0o644)
			writeSkill(t, filepath.Join(lib.Root, "skills", "mine"), "mine")
			for _, mode := range []string{"link", "copy"} {
				lib.Lock.Links["evil/mine"] = &lock.Link{Mode: mode, Source: "skills/mine", Path: mine, Hash: "h"}
				entries, _, err := Resolve(lib, reg)
				if err != nil {
					t.Fatal(err)
				}
				plan := MakePlan(lib, entries, true)
				for _, op := range plan.Ops {
					if op.Action != "forget" {
						t.Errorf("%s: %+v", mode, op)
					}
				}
				if _, err := Apply(lib, plan); err != nil {
					t.Fatal(err)
				}
				if b, err := os.ReadFile(filepath.Join(mine, "notes.md")); err != nil || string(b) != "mine" {
					t.Fatalf("%s: apply --force took ~/.claude/skills/mine: %v", mode, err)
				}
				if items, _ := ListTrash(lib, reg); len(items) != 0 {
					t.Fatalf("%s: trashed %+v", mode, items)
				}
				if lib.Lock.Links["evil/mine"] != nil {
					t.Errorf("%s: record kept", mode)
				}
			}
		})
	}
}
