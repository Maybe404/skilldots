package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
)

// A new library learns the skilldots skill for every agent on the machine:
// Claude Code reads ~/.claude/skills, not the default ~/.agents/skills.
func TestInitLibraryInstallsSelfSkillForDetectedAgents(t *testing.T) {
	up := t.TempDir()
	repo := filepath.Join(up, "Maybe404", "skilldots.git")
	write(t, filepath.Join(repo, "LICENSE"), "MIT License\n")
	write(t, filepath.Join(repo, "skills", "skilldots", "SKILL.md"), skillMD("skilldots"))
	git(t, repo, "init", "-q")
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init")

	home, _ := filepath.EvalSymlinks(t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(home, "cache"))
	t.Setenv("SKILLDOTS_LIBRARY", "")
	gc := filepath.Join(home, "gitconfig")
	write(t, gc, "[user]\n\tname = t\n\temail = t@example.com\n[init]\n\tdefaultBranch = main\n[url \"file://"+up+"/\"]\n\tinsteadOf = https://github.com/\n")
	t.Setenv("GIT_CONFIG_GLOBAL", gc)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	// Claude Code is installed (~/.claude), Codex is not.
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	root := filepath.Join(home, "lib")
	// Claude Code is installed, so it is a default target from the start.
	if got := app.DefaultTargets(); !slices.Equal(got, []string{"agents", "claude"}) {
		t.Fatalf("default targets = %v, want [agents claude]", got)
	}
	if err := app.InitLibrary(root, nil); err != nil {
		t.Fatal(err)
	}
	if err := app.InitLibrary(root, nil); err == nil {
		t.Fatal("a second init over the same library succeeded")
	}
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.DetectedTargets(); !slices.Equal(got, []string{"agents", "claude"}) {
		t.Fatalf("detected targets = %v, want [agents claude]", got)
	}
	if s.HasSelfSkill() {
		t.Fatal("a new library already has the skilldots skill")
	}
	if _, _, err := s.InstallSelfSkill(); err != nil {
		t.Fatal(err)
	}
	s, _ = app.Open(root)
	if !s.HasSelfSkill() {
		t.Fatal("the skilldots skill is not in the manifest")
	}
	for _, dir := range []string{".agents/skills", ".claude/skills"} {
		if _, err := os.Stat(filepath.Join(home, dir, "skilldots", "SKILL.md")); err != nil {
			t.Fatalf("skilldots not linked into ~/%s: %v", dir, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
		t.Fatal("a directory was made for an agent that is not installed")
	}
	// The defaults first: an older skd, which ignores "*", still links those.
	if p := s.Lib.Manifest.FindPack(app.SelfRepo); p == nil || p.Targets == nil || !slices.Equal(*p.Targets, []string{"agents", "claude", "*"}) {
		t.Fatalf("the skilldots pack should target [agents claude *], got %+v", p)
	}

	// An agent installed later gets the skill at the next apply.
	// Kiro does not read ~/.agents/skills, so "*" links into its own directory.
	os.MkdirAll(filepath.Join(home, ".kiro"), 0o755)
	s, _ = app.Open(root)
	if _, err := s.Apply(app.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".kiro", "skills", "skilldots", "SKILL.md")); err != nil {
		t.Fatalf("an agent installed later did not get skilldots: %v", err)
	}

	// The pack without the skill (use = []): installing adds it to that
	// pack, never a second one, and keeps the agents the user chose for it.
	s, _ = app.Open(root)
	s.Lib.Manifest.FindPack(app.SelfRepo).Use = []string{}
	s.Lib.Manifest.FindPack(app.SelfRepo).Targets = &[]string{"agents"}
	if err := s.Lib.Manifest.Save(filepath.Join(root, "skills.toml")); err != nil {
		t.Fatal(err)
	}
	s, _ = app.Open(root)
	if s.HasSelfSkill() {
		t.Fatal("HasSelfSkill with use = []")
	}
	if _, _, err := s.InstallSelfSkill(); err != nil {
		t.Fatal(err)
	}
	s, _ = app.Open(root)
	n := 0
	for _, p := range s.Lib.Manifest.Packs {
		if p.Repo == app.SelfRepo {
			n++
		}
	}
	if n != 1 || !s.HasSelfSkill() {
		t.Fatalf("%d skilldots packs, HasSelfSkill %v; want 1, true", n, s.HasSelfSkill())
	}
	if p := s.Lib.Manifest.FindPack(app.SelfRepo); !slices.Equal(*p.Targets, []string{"agents"}) {
		t.Fatalf("the user's targets were replaced: %v", *p.Targets)
	}
	if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "skilldots-skilldots")); !os.IsNotExist(err) {
		t.Fatal("a second copy of skilldots was linked")
	}
}

// The user's own fork of skilldots counts: no second copy is installed.
func TestHasSelfSkillCountsAFork(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SKILLDOTS_LIBRARY", "")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	write(t, filepath.Join(home, "gitconfig"), "[user]\n\tname = t\n\temail = t@example.com\n")
	root := filepath.Join(home, "lib")
	if err := app.InitLibrary(root, nil); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "skills", "skilldots", "SKILL.md"), skillMD("skilldots"))
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if !s.HasSelfSkill() {
		t.Fatal("a fork of skilldots in skills/ does not count")
	}
}

// Without Claude Code a new library links into ~/.agents/skills only: no
// ~/.claude is made for an agent that is not there.
func TestDefaultTargetsWithoutClaude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := app.DefaultTargets(); !slices.Equal(got, []string{"agents"}) {
		t.Fatalf("default targets = %v, want [agents]", got)
	}
}
