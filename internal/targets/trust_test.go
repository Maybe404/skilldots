package targets

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// sandbox points HOME (and with it the settings directory, where the
// approvals are) at a temporary directory and returns it, links resolved.
func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if r, err := filepath.EvalSymlinks(home); err == nil {
		home = r
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("AppData", filepath.Join(home, "AppData", "Roaming"))
	return home
}

// approve is what a person at a terminal does for the custom target
// name at path of a library with no other custom target, having been
// shown that it leads to want.
func approve(name, path, want string) error {
	reg, err := NewRegistry("", map[string]struct{ Path, Mode string }{name: {Path: path}})
	if err != nil {
		return err
	}
	return reg.Approve(Withheld{Name: name, Path: path, Mode: "link", Real: want})
}

func TestCheckPathRefusesWhatIsNotForSkills(t *testing.T) {
	home := sandbox(t)
	outside := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	os.MkdirAll(filepath.Join(home, "Library", "LaunchAgents"), 0o755)
	os.MkdirAll(filepath.Join(home, "work"), 0o755)
	os.Symlink(filepath.Join(home, ".ssh"), filepath.Join(home, "work", "keys"))
	os.Symlink(outside, filepath.Join(home, "work", "out"))
	os.Symlink(home, filepath.Join(home, "work", "home"))
	// From inside home a relative path would land there: still refused.
	os.MkdirAll(filepath.Join(home, "skills"), 0o755)
	t.Chdir(home)
	for _, p := range []string{
		"", "skills", "./skills", "~", "~/", "~/.", "~/x/..", home, home + "/",
		outside, filepath.Join(outside, "skills"), "/etc/skills",
		"~/.ssh", "~/.ssh/skills", "~/.gnupg", "~/.aws/x", "~/.config", "~/.config/gcloud",
		"~/.local", "~/.local/bin", "~/bin", "~/Library", "~/Library/LaunchAgents", "~/Library/LaunchAgents/x",
		"~/work/keys", "~/work/keys/x", "~/work/out", "~/work/out/skills", "~/work/home",
		"~/.claude", "~/.agents", "~/.config/opencode",
		filepath.Dir(filepath.Dir(ApprovalsFile())), filepath.Dir(ApprovalsFile()),
	} {
		if real, err := CheckPath(p, ""); err == nil {
			t.Errorf("CheckPath(%q) = %q, allowed", p, real)
		}
	}
	// A file system that ignores case: ~/.SSH is ~/.ssh.
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		// ~/.gnupg does not exist: ~/.GnuPG would become it.
		for _, p := range []string{"~/.SSH", "~/.Ssh/x", "~/library/launchagents", "~/.GnuPG", "~/.GNUPG/x"} {
			if real, err := CheckPath(p, ""); err == nil {
				t.Errorf("CheckPath(%q) = %q, allowed", p, real)
			}
		}
	}
	for p, want := range map[string]string{
		"~/my-tool/skills":           filepath.Join(home, "my-tool", "skills"),
		"~/.config/my-agent/skills/": filepath.Join(home, ".config", "my-agent", "skills"),
		"~/.claude/skills":           filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, "x"):     filepath.Join(home, "x"),
		"~/work/sub/../skills":       filepath.Join(home, "work", "skills"),
	} {
		if real, err := CheckPath(p, ""); err != nil || real != want {
			t.Errorf("CheckPath(%q) = %q, %v; want %q", p, real, err, want)
		}
	}
}

func TestCustomTargetsNeedApproval(t *testing.T) {
	home := sandbox(t)
	custom := map[string]struct{ Path, Mode string }{
		"mine":   {Path: "~/my-tool/skills"},
		"home":   {Path: "~"},
		"agents": {Path: "~/.ssh"},
	}
	reg, err := NewRegistry("", custom)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"mine", "home"} {
		if _, err := reg.Get(n); err == nil {
			t.Errorf("%s is used without approval", n)
		}
	}
	if a, err := reg.Get("agents"); err != nil || a.Path != filepath.Join(home, ".agents", "skills") || !a.Builtin {
		t.Errorf("a custom target replaced the built-in agents: %+v %v", a, err)
	}
	for _, tt := range reg.All() {
		if !tt.Builtin {
			t.Errorf("All lists the withheld %s", tt.Name)
		}
	}
	w := reg.Withheld()
	if len(w) != 3 || !w[2].Approvable || w[0].Approvable || w[1].Approvable || !strings.Contains(w[2].Problem, "skd targets trust mine") {
		t.Fatalf("withheld: %+v", w)
	}
	if _, err := os.Stat(ApprovalsFile()); !os.IsNotExist(err) {
		t.Fatal("reading the targets wrote the approvals file")
	}

	// Only what was shown is approved; refused ones never are.
	real := filepath.Join(home, "my-tool", "skills")
	if err := approve("mine", "~/my-tool/skills", filepath.Join(home, "elsewhere")); err == nil {
		t.Error("approved a directory other than the one shown")
	}
	for _, n := range []string{"home", "agents"} {
		if err := approve(n, custom[n].Path, home); err == nil {
			t.Errorf("approved %s", n)
		}
	}
	if err := approve("mine", "~/my-tool/skills", real); err != nil {
		t.Fatal(err)
	}
	reg, _ = NewRegistry("", custom)
	if m, err := reg.Get("mine"); err != nil || m.Path != real || m.Builtin {
		t.Fatalf("approved target: %+v %v", m, err)
	}
	if _, err := reg.Get("home"); err == nil {
		t.Error("approving one approved another")
	}
	// The approval is the name with the directory: the same name elsewhere,
	// or another name for the directory, needs approving again.
	for name, c := range map[string]struct{ Path, Mode string }{"mine": {Path: "~/my-tool/other"}, "yours": {Path: "~/my-tool/skills"}} {
		reg, _ = NewRegistry("", map[string]struct{ Path, Mode string }{name: c})
		if _, err := reg.Get(name); err == nil {
			t.Errorf("%s at %s is used without approval", name, c.Path)
		}
	}
	// A link that comes to lead somewhere else is not what was approved.
	os.MkdirAll(filepath.Join(home, "a"), 0o755)
	os.MkdirAll(filepath.Join(home, "b"), 0o755)
	os.Symlink(filepath.Join(home, "a"), filepath.Join(home, "via"))
	via := map[string]struct{ Path, Mode string }{"via": {Path: "~/via/skills"}}
	if err := approve("via", "~/via/skills", filepath.Join(home, "a", "skills")); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(home, "via"))
	os.Symlink(filepath.Join(home, "b"), filepath.Join(home, "via"))
	reg, _ = NewRegistry("", via)
	if _, err := reg.Get("via"); err == nil {
		t.Error("a target whose link was changed is still used")
	}

	// Revoking stops it.
	if n, err := Revoke("mine"); err != nil || n != 1 {
		t.Fatalf("revoke: %d %v", n, err)
	}
	reg, _ = NewRegistry("", custom)
	if _, err := reg.Get("mine"); err == nil {
		t.Error("revoked target still used")
	}
	// An approvals file that cannot be read approves nothing.
	if err := approve("mine", "~/my-tool/skills", real); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(ApprovalsFile(), []byte("not = [toml"), 0o644)
	reg, _ = NewRegistry("", custom)
	if w, ok := reg.WithheldTarget("mine"); !ok || w.Approvable {
		t.Errorf("unreadable approvals: %+v", w)
	}
	if err := approve("mine", "~/my-tool/skills", real); err == nil {
		t.Error("approving over an unreadable approvals file")
	}
}

// A custom target named like a built-in one is never used nor approvable,
// wherever it points: the built-in target stays what it is.
func TestCustomTargetCannotTakeABuiltinName(t *testing.T) {
	home := sandbox(t)
	custom := map[string]struct{ Path, Mode string }{"claude": {Path: "~/elsewhere/skills"}}
	real := filepath.Join(home, "elsewhere", "skills")
	if err := approve("claude", "~/elsewhere/skills", real); err == nil {
		t.Fatal("approved a custom target with a built-in name")
	}
	reg, _ := NewRegistry("", custom)
	c, err := reg.Get("claude")
	if err != nil || !c.Builtin || c.Path != filepath.Join(home, ".claude", "skills") {
		t.Fatalf("claude: %+v %v", c, err)
	}
	if w, ok := reg.WithheldTarget("claude"); !ok || w.Approvable || !strings.Contains(w.Problem, "built-in") {
		t.Fatalf("withheld: %+v %v", w, ok)
	}
}
