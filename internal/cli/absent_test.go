package cli_test

import (
	"os"
	"strings"
	"testing"
)

// A skill for every installed agent ("*", as the skilldots skill is
// followed) is not linked into the own directory of an agent that reads
// ~/.agents/skills too (Codex would load it twice); one that does not
// (Kiro, Claude Code) gets its link. With the shared directory paused,
// Codex gets its own.
func TestEveryAgentMeansOneCopyEach(t *testing.T) {
	e := newEnv(t)
	e.installed(".claude", ".codex", ".kiro")
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.skill("lib/skills/mine", "mine")
	b, _ := os.ReadFile(e.p("lib/skills.toml"))
	os.WriteFile(e.p("lib/skills.toml"), append(b, []byte("\n[[local]]\nname = \"mine\"\ntargets = [\"agents\", \"claude\", \"*\"]\n")...), 0o644)
	e.ok(lib, "apply")
	for _, d := range []string{".agents", ".claude", ".kiro"} {
		isLink(t, e.p(d+"/skills/mine"), lib)
	}
	if _, err := os.Lstat(e.p(".codex/skills/mine")); !os.IsNotExist(err) {
		t.Fatalf("linked into Codex's own directory too: %v", err)
	}
	if out := e.ok(lib, "targets"); !strings.Contains(out, "reads agents") {
		t.Errorf("targets:\n%s", out)
	}
	e.ok(lib, "targets", "pause", "agents")
	isLink(t, e.p(".codex/skills/mine"), lib)
	if _, err := os.Lstat(e.p(".agents/skills/mine")); !os.IsNotExist(err) {
		t.Fatalf("still in the paused shared directory: %v", err)
	}
	e.ok(lib, "targets", "resume", "agents")
	if _, err := os.Lstat(e.p(".codex/skills/mine")); !os.IsNotExist(err) {
		t.Fatalf("Codex's own link stayed after resume: %v", err)
	}
	isLink(t, e.p(".agents/skills/mine"), lib)
}

// skills.toml travels with the library: a target written on machine A,
// which has Claude Code, does not make ~/.claude on machine B, which
// does not. B's apply links into the agents it has, says Claude Code is
// not installed there, and keeps the lock's record of A's link.
func TestNoDirectoryForAnAgentNotInstalled(t *testing.T) {
	a := newEnv(t)
	os.MkdirAll(a.p(".claude"), 0o755)
	lib := a.p("lib")
	a.ok("", "init", lib, "--no-skill")
	a.ok(lib, "targets", "default", "agents", "claude", "codex")
	a.ok(lib, "new", "mine")
	isLink(t, a.p(".claude/skills/mine"), lib)
	if _, err := os.Stat(a.p(".codex")); !os.IsNotExist(err) {
		t.Fatalf("~/.codex made on A, which has no Codex: %v", err)
	}
	a.git(lib, "add", "-A")
	a.git(lib, "commit", "-qm", "x")
	a.git(a.home, "clone", "-q", "--bare", lib, a.p("backup.git"))

	b := newEnv(t)
	for _, lang := range []string{"en", "zh"} {
		b.setLang(lang)
		os.RemoveAll(b.p("lib2"))
		out := b.ok("", "clone", a.p("backup.git"), b.p("lib2"))
		if _, err := os.Stat(b.p(".claude")); !os.IsNotExist(err) {
			t.Fatalf("%s: ~/.claude made on B: %v\n%s", lang, err, out)
		}
		isLink(t, b.p(".agents/skills/mine"), b.p("lib2"))
		out = b.ok(b.p("lib2"), "apply")
		want := map[string]string{"en": "claude is not installed on this machine: 1 skill not linked there",
			"zh": "本机没有安装 claude：有 1 个 skill 没有链接到那里"}[lang]
		if !strings.Contains(out, want) || strings.Contains(out, "error") {
			t.Fatalf("%s: apply:\n%s", lang, out)
		}
		if _, err := os.Stat(b.p(".claude")); !os.IsNotExist(err) {
			t.Fatalf("%s: ~/.claude made by apply: %v", lang, err)
		}
		lock, _ := os.ReadFile(b.p("lib2/skills.lock"))
		if !strings.Contains(string(lock), "claude/mine") {
			t.Fatalf("%s: the record of A's link was dropped:\n%s", lang, lock)
		}
	}
}
