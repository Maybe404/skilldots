package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func longMD(top, bottom string) string {
	return "---\nname: tdd\ndescription: d\n---\n" + top + "\na\nb\nc\nd\ne\nf\ng\n" + bottom + "\n"
}

// skd update shelves a conflicting skill and says what resolves it;
// skd resolve, --continue and --abort work as a git merge does; skd
// history --diff shows what upstream changed.
func TestResolveCommands(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.write("up/skills/eng/tdd/SKILL.md", longMD("top", "bottom"))
	up := e.repo("up")
	e.ok(lib, "add", up, "--all")
	vendored := filepath.Join(strings.TrimSpace(e.ok(lib, "path", "tdd")), "SKILL.md")
	os.WriteFile(vendored, []byte(longMD("top mine", "bottom")), 0o644)
	e.write("up/skills/eng/tdd/SKILL.md", longMD("top upstream", "bottom"))
	e.git(up, "commit", "-qam", "upstream")

	out := e.ok(lib, "update", "--yes")
	if !strings.Contains(out, "shelved") || !strings.Contains(out, "skd resolve tdd") {
		t.Fatalf("update:\n%s", out)
	}
	if out := e.ok(lib, "list", "--flat"); !strings.Contains(out, "shelved") {
		t.Errorf("list:\n%s", out)
	}
	if out := e.ok(lib, "list"); !strings.Contains(out, "1 shelved with conflicts: tdd") {
		t.Errorf("list overview:\n%s", out)
	}
	js := e.js(lib, "resolve")
	if sk := js["skills"].([]any); len(sk) != 1 || sk[0].(map[string]any)["name"] != "tdd" {
		t.Fatalf("resolve --json: %v", js)
	}
	if js := e.js(lib, "outdated"); js["packs"].([]any)[0].(map[string]any)["shelved"] == nil {
		t.Errorf("outdated: %v", js)
	}

	out = e.ok(lib, "resolve", "tdd")
	if !strings.Contains(out, "SKILL.md") || !strings.Contains(out, "skd resolve --continue tdd") {
		t.Fatalf("resolve:\n%s", out)
	}
	if b, _ := os.ReadFile(vendored); !strings.Contains(string(b), "<<<<<<< local") {
		t.Fatalf("no markers:\n%s", b)
	}
	if st := e.git(lib, "status", "--porcelain"); !strings.Contains(st, "UU ") {
		t.Errorf("status:\n%s", st)
	}
	// Refused while markers are left: non-zero, and the files named.
	if js := e.jsFail(lib, "resolve", "--continue"); js["skill"] != "tdd" {
		t.Errorf("continue refused: %v", js)
	}
	if _, _, code := e.skd(lib, "sync"); code == 0 {
		t.Error("skd sync committed conflict markers")
	}
	// Abort, start again, finish.
	e.ok(lib, "resolve", "--abort")
	if b, _ := os.ReadFile(vendored); string(b) != longMD("top mine", "bottom") {
		t.Fatalf("abort:\n%s", b)
	}
	e.ok(lib, "resolve", "tdd")
	os.WriteFile(vendored, []byte(longMD("top mine and upstream", "bottom")), 0o644)
	if out := e.ok(lib, "resolve", "--continue", "tdd"); !strings.Contains(out, "merged with upstream") {
		t.Errorf("continue:\n%s", out)
	}
	e.ok(lib, "sync")

	out = e.ok(lib, "history", up, "--diff", "2")
	if !strings.Contains(out, "-top") || !strings.Contains(out, "+top upstream") || !strings.Contains(out, "@@ ") {
		t.Errorf("history --diff 2:\n%s", out)
	}
	if js := e.js(lib, "history", up, "--diff", "2", "--skill", "tdd"); js["source"] != "snapshots" {
		t.Errorf("history --diff --json: %v", js)
	}
	if _, _, code := e.skd(lib, "history", up, "extra"); code == 0 {
		t.Error("two arguments without --diff")
	}
}
