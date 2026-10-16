package targets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Places that load or run what is put in them are refused, at, inside
// and above them; so is the library, and a directory inside a built-in
// target's (a skill linked there may hold a link that leads anywhere).
func TestCheckPathRefusesAutoloadPlacesTheLibraryAndBuiltinInsides(t *testing.T) {
	home := sandbox(t)
	lib := filepath.Join(home, "code", "lib")
	os.MkdirAll(filepath.Join(lib, ".git", "hooks"), 0o755)
	for _, p := range []string{
		"~/.vscode/extensions", "~/.cursor/extensions", "~/node_modules", "~/.git-templates/hooks",
		"~/Applications", "~/.local/share/nvim/site/pack/x/start", "~/.local/share/systemd/user",
		"~/.config/fish/conf.d", "~/.oh-my-zsh/custom", "~/Library/Services", "~/.cargo/bin",
		"~/.hammerspoon", "~/.vim/pack/x/start", "~/.config/nvim/lua", "~/Library/LaunchDaemons",
		"~/.vscode", "~/.local/share", "~/.cursor",
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
	if err := reg.Approve(w, "inner"); err == nil {
		t.Error("approved a target nested in another")
	}
}

// The approval names the mode: a target approved to be linked into is
// not used once skills.toml says copy, until approved again. A record
// without a mode (an older skd) approves link only.
func TestApprovalNamesTheMode(t *testing.T) {
	home := sandbox(t)
	real := filepath.Join(home, "my-tool", "skills")
	link := map[string]struct{ Path, Mode string }{"mine": {Path: "~/my-tool/skills"}}
	cp := map[string]struct{ Path, Mode string }{"mine": {Path: "~/my-tool/skills", Mode: "copy"}}
	if err := approve("mine", "~/my-tool/skills", real); err != nil {
		t.Fatal(err)
	}
	if reg, _ := NewRegistry("", link); !used(reg, "mine") {
		t.Fatal("approved link target not used")
	}
	reg, _ := NewRegistry("", cp)
	if used(reg, "mine") {
		t.Fatal("a link approval let skd copy")
	}
	w, _ := reg.WithheldTarget("mine")
	if !w.Approvable || w.Mode != "copy" {
		t.Fatalf("%+v", w)
	}
	// Shown as link, but skills.toml now says copy: not approved.
	if err := reg.Approve(Withheld{Name: "mine", Path: "~/my-tool/skills", Mode: "link", Real: real}, ""); err == nil {
		t.Fatal("approved a mode other than the one shown")
	}
	if err := reg.Approve(w, ""); err != nil {
		t.Fatal(err)
	}
	if reg, _ := NewRegistry("", cp); !used(reg, "mine") {
		t.Fatal("approved copy target not used")
	}
	// An approvals file of an older skd: no mode.
	os.WriteFile(ApprovalsFile(), []byte("[[target]]\nname = 'mine'\npath = '"+real+"'\napproved = 2026-01-01T00:00:00Z\n"), 0o644)
	if reg, _ := NewRegistry("", link); !used(reg, "mine") {
		t.Error("an old approval no longer approves link")
	}
	if reg, _ := NewRegistry("", cp); used(reg, "mine") {
		t.Error("an old approval approves copy")
	}
	// Any other mode is never used.
	reg, _ = NewRegistry("", map[string]struct{ Path, Mode string }{"mine": {Path: "~/my-tool/skills", Mode: "hardlink"}})
	if w, ok := reg.WithheldTarget("mine"); !ok || w.Approvable {
		t.Errorf("mode hardlink: %+v", w)
	}
}

// A directory not named skills is approved only when the person typed
// the target's name; the path in skills.toml must still be the one shown.
func TestUnusualDirectoryNeedsTheNameTyped(t *testing.T) {
	home := sandbox(t)
	custom := map[string]struct{ Path, Mode string }{"ext": {Path: "~/my-tool/plugins"}, "sk": {Path: "~/x/Skill"}}
	reg, _ := NewRegistry("", custom)
	ext, _ := reg.WithheldTarget("ext")
	sk, _ := reg.WithheldTarget("sk")
	if !Unusual(ext.Real) || Unusual(sk.Real) || Unusual(filepath.Join(home, "SKILLS")) {
		t.Fatal("Unusual")
	}
	for _, typed := range []string{"", "y", "yes", "Ext", "ext "} {
		if err := reg.Approve(ext, typed); err == nil {
			t.Errorf("approved %s with %q typed", ext.Real, typed)
		}
	}
	if _, err := os.Stat(ApprovalsFile()); err == nil {
		t.Fatal("approvals written")
	}
	if err := reg.Approve(ext, "ext"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Approve(sk, ""); err != nil {
		t.Fatal(err)
	}
	moved := sk
	moved.Path = "~/x/other/Skill"
	if err := reg.Approve(moved, ""); err == nil {
		t.Error("approved a path skills.toml does not have")
	}
	reg, _ = NewRegistry("", custom)
	if !used(reg, "ext") || !used(reg, "sk") {
		t.Fatal("approved targets not used")
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
