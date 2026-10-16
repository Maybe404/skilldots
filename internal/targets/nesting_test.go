package targets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The library is refused, at, inside and above it, and so is a directory
// inside a built-in target's (a skill linked there may hold a link that
// leads anywhere).
func TestCheckPathRefusesTheLibraryAndBuiltinInsides(t *testing.T) {
	home := sandbox(t)
	lib := filepath.Join(home, "code", "lib")
	os.MkdirAll(filepath.Join(lib, ".git", "hooks"), 0o755)
	for _, p := range []string{
		"~/.cursor",
		lib, filepath.Join(lib, ".git", "hooks"), filepath.Join(lib, "vendor"), filepath.Join(lib, ".trash"),
		filepath.Join(lib, "skills", "foo", "newdir"), "~/code",
		"~/.claude/skills/foo", "~/.claude/skills/foo/newdir", "~/.agents/skills/x/y", "~/.gemini/skills/a",
	} {
		if real, err := CheckPath(p, lib); err == nil {
			t.Errorf("CheckPath(%q) = %q, allowed", p, real)
		}
	}
	for _, p := range []string{"~/my-tool/skills", "~/.claude/skills", "~/code/other/skills", "~/.gemini/antigravity/skills"} {
		if _, err := CheckPath(p, lib); err != nil {
			t.Errorf("CheckPath(%q): %v", p, err)
		}
	}
	// Once ~/.claude/skills/foo is a link into the library, a path through
	// it is the library: refused for that reason too.
	os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755)
	os.MkdirAll(filepath.Join(lib, "skills", "foo"), 0o755)
	os.Symlink(filepath.Join(lib, "skills", "foo"), filepath.Join(home, ".claude", "skills", "foo"))
	if _, err := CheckPath("~/.claude/skills/foo/newdir", lib); err == nil || !strings.Contains(err.Error(), "library") {
		t.Errorf("through a link into the library: %v", err)
	}
}

// A custom target inside (or holding, or being) another one is withheld,
// both of them: each target's entries are written through it alone. One
// whose own path is refused does not count.
func TestNestedCustomTargetsAreWithheld(t *testing.T) {
	sandbox(t)
	reg, err := NewRegistry("", map[string]struct{ Path, Mode string }{
		"outer": {Path: "~/tools/skills"},
		"inner": {Path: "~/tools/skills/x"},
		"same":  {Path: "~/same/skills"},
		"same2": {Path: "~/same/skills/"},
		"bad":   {Path: "~"},
		"alone": {Path: "~/alone/skills"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"outer", "inner", "same", "same2"} {
		w, ok := reg.WithheldTarget(n)
		if !ok || w.Approvable || !strings.Contains(w.Problem, "custom target") {
			t.Errorf("%s: %+v", n, w)
		}
	}
	if w, ok := reg.WithheldTarget("alone"); !ok || !w.Approvable {
		t.Errorf("alone: %+v", w)
	}
	// Approving one of them is refused too.
	w, _ := reg.WithheldTarget("inner")
	if err := reg.Approve(w); err == nil {
		t.Error("approved a target nested in another")
	}
}

// A target is intact while its path leads where the registry found it.
func TestIntact(t *testing.T) {
	home := sandbox(t)
	os.MkdirAll(filepath.Join(home, "a"), 0o755)
	os.MkdirAll(filepath.Join(home, "b"), 0o755)
	os.Symlink(filepath.Join(home, "a"), filepath.Join(home, "via"))
	if err := approve("via", "~/via/skills", filepath.Join(home, "a", "skills")); err != nil {
		t.Fatal(err)
	}
	reg, _ := NewRegistry("", map[string]struct{ Path, Mode string }{"via": {Path: "~/via/skills"}})
	v, err := reg.Get("via")
	if err != nil {
		t.Fatal(err)
	}
	c, _ := reg.Get("claude")
	if v.Intact() != nil || c.Intact() != nil {
		t.Fatal("fresh targets are not intact")
	}
	os.MkdirAll(c.Path, 0o755)
	if c.Intact() != nil {
		t.Fatal("creating the directory changed where it leads")
	}
	os.Remove(filepath.Join(home, "via"))
	os.Symlink(filepath.Join(home, "b"), filepath.Join(home, "via"))
	if v.Intact() == nil {
		t.Error("a target whose link changed is intact")
	}
	os.Remove(c.Path)
	os.Symlink(filepath.Join(home, "b"), c.Path)
	if c.Intact() == nil {
		t.Error("a built-in target replaced by a link is intact")
	}
	if (Target{Name: "x", Path: home}).Intact() == nil {
		t.Error("an unchecked target is intact")
	}
}

func used(reg *Registry, name string) bool {
	_, err := reg.Get(name)
	return err == nil
}
