package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/manifest"
)

// An [[app]] in a cloned library's skills.toml can name any directory.
// skd links or copies only a skill: a directory holding SKILL.md as a
// regular file, not a place of keys and credentials nor one holding
// such a place. Anything else is skipped with a warning; nothing is
// linked or copied, into a copy target least of all.
func TestAppMustBeASkill(t *testing.T) {
	cases := []struct {
		name  string
		setup func(home string) string // returns the app path
	}{
		{"no SKILL.md (~/.ssh)", func(home string) string {
			d := filepath.Join(home, ".ssh")
			os.MkdirAll(d, 0o700)
			os.WriteFile(filepath.Join(d, "id_ed25519"), []byte("KEY"), 0o600)
			return d
		}},
		{"a key directory with a SKILL.md", func(home string) string {
			d := filepath.Join(home, ".ssh")
			writeSkill(t, d, "keys")
			os.WriteFile(filepath.Join(d, "id_ed25519"), []byte("KEY"), 0o600)
			return d
		}},
		{"inside a key directory", func(home string) string {
			d := filepath.Join(home, ".aws", "x")
			writeSkill(t, d, "x")
			return d
		}},
		{"holding a key directory", func(home string) string {
			writeSkill(t, home, "home")
			os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
			return home
		}},
		{"SKILL.md is a link", func(home string) string {
			d := filepath.Join(home, "app")
			os.MkdirAll(d, 0o755)
			os.WriteFile(filepath.Join(home, "notes.md"), []byte("x"), 0o644)
			os.Symlink(filepath.Join(home, "notes.md"), filepath.Join(d, "SKILL.md"))
			return d
		}},
		{"a key directory through a link", func(home string) string {
			d := filepath.Join(home, ".gnupg")
			writeSkill(t, d, "g")
			os.Symlink(d, filepath.Join(home, "innocent"))
			return filepath.Join(home, "innocent")
		}},
		// A skill there, from where skd runs: checked there, but a link
		// to it would be read from the agent's directory.
		{"a relative path", func(home string) string {
			writeSkill(t, filepath.Join(home, "rel", "x"), "x")
			t.Chdir(home)
			return "rel/x"
		}},
		// Nothing there from where skd runs: still said to be refused
		// for being relative, not taken for a missing app.
		{"a relative path to nothing here", func(home string) string {
			t.Chdir(home)
			return "nowhere/x"
		}},
	}
	for _, tc := range cases {
		lib, _, home := newTestLib(t)
		box := filepath.Join(home, "box")
		reg := customRegistry(t, map[string]struct{ Path, Mode string }{"box": {Path: box, Mode: "copy"}})
		SetRegistry(reg)
		lib.Manifest.Targets.Default = []string{"agents", "box"}
		lib.Manifest.Apps = append(lib.Manifest.Apps, &manifest.App{Name: "x", Path: tc.setup(home)})
		entries, warnings, _ := Resolve(lib, reg)
		if !strings.Contains(strings.Join(warnings, "\n"), "[[app]] x") {
			t.Errorf("%s: no warning: %v", tc.name, warnings)
		}
		plan := MakePlan(lib, entries, true)
		for _, o := range plan.Ops {
			if o.Name == "x" && o.Action != "skip" {
				t.Errorf("%s: %+v", tc.name, o)
			}
		}
		if _, err := Apply(lib, plan); err != nil {
			t.Fatal(err)
		}
		for _, d := range []string{filepath.Join(home, ".agents", "skills", "x"), filepath.Join(box, "x")} {
			if fileExists(d) {
				t.Errorf("%s: %s made", tc.name, d)
			}
		}
	}
}

// An app such as Surge's (a SKILL.md in an application bundle outside
// the home directory) is linked as before.
func TestAppInABundleIsLinked(t *testing.T) {
	lib, reg, home := newTestLib(t)
	app := filepath.Join(t.TempDir(), "Applications", "Surge.app", "Contents", "Resources", "Skills", "surge")
	writeSkill(t, app, "surge")
	os.MkdirAll(filepath.Join(app, "references"), 0o755)
	lib.Manifest.Apps = append(lib.Manifest.Apps, &manifest.App{Name: "surge", Path: app})
	entries, warnings, _ := Resolve(lib, reg)
	if len(warnings) != 0 {
		t.Fatalf("warnings %v", warnings)
	}
	if _, err := Apply(lib, MakePlan(lib, entries, false)); err != nil {
		t.Fatal(err)
	}
	if !isLinkTo(filepath.Join(home, ".agents", "skills", "surge"), app) {
		t.Fatal("not linked")
	}
}

// The app's directory is checked again when the change is made: one
// that stopped being a skill since the plan is refused.
func TestAppCheckedAgainWhenApplied(t *testing.T) {
	lib, reg, home := newTestLib(t)
	app := filepath.Join(home, "Apps", "X.app", "x")
	writeSkill(t, app, "x")
	lib.Manifest.Apps = append(lib.Manifest.Apps, &manifest.App{Name: "x", Path: app})
	entries, _, _ := Resolve(lib, reg)
	plan := MakePlan(lib, entries, false)
	os.Remove(filepath.Join(app, "SKILL.md"))
	res, err := Apply(lib, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Refused) != 1 || fileExists(filepath.Join(home, ".agents", "skills", "x")) {
		t.Fatalf("refused %v", res.Refused)
	}
}

// CheckAppDir refuses a relative path whatever it names, and says so
// without repeating it (it comes from skills.toml).
func TestCheckAppDirWantsAnAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, filepath.Join(dir, "x"), "x")
	t.Chdir(dir)
	err := CheckAppDir("x")
	if err == nil || !strings.Contains(err.Error(), "not absolute") || strings.Contains(err.Error(), `"x"`) {
		t.Fatalf("%v", err)
	}
	if err := CheckAppDir(filepath.Join(dir, "x")); err != nil {
		t.Fatal(err)
	}
}
