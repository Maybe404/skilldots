package cli_test

import (
	"os"
	"strings"
	"testing"
)

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
