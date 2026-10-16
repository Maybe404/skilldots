package cli_test

import (
	"os"
	"strings"
	"testing"
)

func TestHistoryCommand(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.skill("up/skills/eng/tdd", "tdd")
	up := e.repo("up")
	e.ok("", "add", up, "--skill", "tdd")
	if out := e.ok("", "history"); !strings.Contains(out, "no updates recorded yet") {
		t.Fatalf("empty history:\n%s", out)
	}
	e.write("up/skills/eng/tdd/SKILL.md", "---\nname: tdd\ndescription: d\n---\nnew body\n")
	e.git(up, "add", "-A")
	e.git(up, "commit", "-qm", "tdd: sharper wording")
	if out := e.ok("", "update"); !strings.Contains(out, "skd history") {
		t.Fatalf("update does not say where the record is:\n%s", out)
	}
	// Offline: the cache is gone and history still answers.
	os.RemoveAll(e.p("cache"))
	out := e.ok("", "history", up)
	for _, want := range []string{up, "changed", "tdd", "tdd: sharper wording"} {
		if !strings.Contains(out, want) {
			t.Fatalf("history lacks %q:\n%s", want, out)
		}
	}
	r := e.js("", "history", "-n", "1")
	recs := r["records"].([]any)
	if len(recs) != 1 || recs[0].(map[string]any)["repo"] != up {
		t.Fatalf("history --json: %v", r)
	}
	// It is committed with the library.
	e.ok("", "sync")
	if files := e.git(lib, "ls-files"); !strings.Contains(files, "skills.history.jsonl") {
		t.Fatalf("history not committed:\n%s", files)
	}
}
