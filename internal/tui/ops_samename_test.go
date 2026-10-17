package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/manifest"
)

// sameNameLibrary has vendored acme/skills tdd linked into claude only and
// your own skills/tdd linked into agents only: two rows named tdd.
func sameNameLibrary(t *testing.T) (o *libOps, root, home string) {
	t.Helper()
	o, root, home, _ = acmeLibrary(t)
	m := loadManifest(t, root)
	claude := []string{"claude"}
	m.FindPack("acme/skills").Override("tdd", true).Targets = &claude
	agents := []string{"agents"}
	m.Locals = append(m.Locals, &manifest.Local{Name: "tdd", Targets: &agents})
	if err := m.Save(filepath.Join(root, manifest.FileName)); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "skills", "tdd", "SKILL.md"), skillMD("tdd", "Mine."))
	if _, err := o.Apply(); err != nil {
		t.Fatal(err)
	}
	checkSameNameLinks(t, root, home, true, true)
	return o, root, home
}

// checkSameNameLinks asserts where each tdd is linked: the vendored one
// in claude, yours in agents.
func checkSameNameLinks(t *testing.T, root, home string, vendor, mine bool) {
	t.Helper()
	got := func(p string) string { s, _ := os.Readlink(p); return s }
	v := got(filepath.Join(home, ".claude", "skills", "tdd"))
	if vendor != strings.Contains(v, filepath.Join("vendor", "acme", "skills")) {
		t.Errorf("claude/tdd = %q, want vendored link %v", v, vendor)
	}
	l := got(filepath.Join(home, ".agents", "skills", "tdd"))
	if mine != (l == filepath.Join(root, "skills", "tdd")) {
		t.Errorf("agents/tdd = %q, want own link %v", l, mine)
	}
}

func localTargets(m *manifest.Manifest, name string) []string {
	for _, l := range m.Locals {
		if l.Name == name && l.Targets != nil {
			return *l.Targets
		}
	}
	return nil
}

func vendorTargets(m *manifest.Manifest, name string) []string {
	if o := m.FindPack("acme/skills").Override(name, false); o != nil && o.Targets != nil {
		return *o.Targets
	}
	return nil
}

func sameNameRows(t *testing.T, o *libOps) (vendor, mine Skill) {
	t.Helper()
	s, err := o.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Mine) != 1 || s.Mine[0].Name != "tdd" {
		t.Fatalf("mine: %+v", s.Mine)
	}
	return *findSkill(findPack(s, "acme/skills"), "tdd"), s.Mine[0]
}

func TestOpsSameNameTouchesOnlyTheChosenRow(t *testing.T) {
	t.Run("set targets on yours", func(t *testing.T) {
		o, root, home := sameNameLibrary(t)
		_, mine := sameNameRows(t, o)
		if _, err := o.SetTargets(&mine, "", []string{}); err != nil {
			t.Fatal(err)
		}
		m := loadManifest(t, root)
		if got := localTargets(m, "tdd"); got == nil || len(got) != 0 {
			t.Errorf("[[local]] tdd targets = %v, want []", got)
		}
		if got := vendorTargets(m, "tdd"); !slices.Equal(got, []string{"claude"}) {
			t.Errorf("vendored tdd targets = %v, want [claude]", got)
		}
		checkSameNameLinks(t, root, home, true, false)
	})
	t.Run("trash yours", func(t *testing.T) {
		o, root, home := sameNameLibrary(t)
		_, mine := sameNameRows(t, o)
		if _, err := o.Trash(mine); err != nil {
			t.Fatal(err)
		}
		m := loadManifest(t, root)
		if !slices.Contains(m.FindPack("acme/skills").Use, "tdd") && !m.FindPack("acme/skills").UsesAll() {
			t.Error("vendored tdd deselected")
		}
		if got := vendorTargets(m, "tdd"); !slices.Equal(got, []string{"claude"}) {
			t.Errorf("vendored tdd targets = %v", got)
		}
		checkSameNameLinks(t, root, home, true, false)
		if !trashed(t, root, "tdd") {
			t.Error("own tdd did not reach the trash")
		}
	})
	t.Run("fork yours is refused", func(t *testing.T) {
		o, root, home := sameNameLibrary(t)
		_, mine := sameNameRows(t, o)
		if _, err := o.Fork(mine, false, ""); err == nil || !strings.Contains(err.Error(), "already yours") {
			t.Errorf("fork of your own tdd: %v", err)
		}
		checkSameNameLinks(t, root, home, true, true)
	})
	t.Run("set targets on the vendored one", func(t *testing.T) {
		o, root, home := sameNameLibrary(t)
		vendor, _ := sameNameRows(t, o)
		if _, err := o.SetTargets(&vendor, "", []string{}); err != nil {
			t.Fatal(err)
		}
		m := loadManifest(t, root)
		if got := vendorTargets(m, "tdd"); got == nil || len(got) != 0 {
			t.Errorf("vendored tdd targets = %v, want []", got)
		}
		if got := localTargets(m, "tdd"); !slices.Equal(got, []string{"agents"}) {
			t.Errorf("[[local]] tdd targets = %v", got)
		}
		checkSameNameLinks(t, root, home, false, true)
	})
	t.Run("trash the vendored one", func(t *testing.T) {
		o, root, home := sameNameLibrary(t)
		vendor, _ := sameNameRows(t, o)
		if _, err := o.Trash(vendor); err != nil {
			t.Fatal(err)
		}
		m := loadManifest(t, root)
		if slices.Contains(m.FindPack("acme/skills").Use, "tdd") {
			t.Error("vendored tdd still selected")
		}
		if got := localTargets(m, "tdd"); !slices.Equal(got, []string{"agents"}) {
			t.Errorf("[[local]] tdd targets = %v", got)
		}
		checkSameNameLinks(t, root, home, false, true)
	})
	t.Run("fork the vendored one", func(t *testing.T) {
		o, root, home := sameNameLibrary(t)
		vendor, _ := sameNameRows(t, o)
		// skills/tdd is taken by yours: the fork must say so, not touch it.
		if _, err := o.Fork(vendor, false, ""); err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Errorf("fork onto your tdd: %v", err)
		}
		checkSameNameLinks(t, root, home, true, true)
	})
}
