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

// Upstream's commit subjects never put control characters on the
// terminal, and a 5 MB subject does not break history (CLI or JSON).
func TestHistoryUpstreamText(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.skill("up/skills/eng/tdd", "tdd")
	up := e.repo("up")
	e.ok("", "add", up, "--skill", "tdd")
	for i, msg := range []string{"tdd: \x1b]0;pwned\x07 \x1b[31mred\x1b[0m", strings.Repeat("y", 5<<20)} {
		e.write("up/skills/eng/tdd/n.md", strings.Repeat("x", i+1))
		e.write("msg", msg)
		e.git(up, "add", "-A", "skills")
		e.git(up, "commit", "-q", "--cleanup=verbatim", "-F", e.p("msg"))
	}
	e.ok("", "update")
	out := e.ok("", "history")
	if strings.ContainsAny(out, "\x1b\x07") || !strings.Contains(out, "red") || len(out) > 8192 {
		t.Fatalf("history prints %d bytes:\n%q", len(out), out)
	}
	if r := e.js("", "history"); len(r["records"].([]any)) != 1 {
		t.Fatalf("history --json: %v", r)
	}
	// A line written by something else, too long for any record: skipped.
	f, _ := os.OpenFile(e.p("lib/skills.history.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"repo":"` + strings.Repeat("z", 5<<20) + "\"}\n")
	f.Close()
	out, errOut, code := e.skd("", "history")
	if code != 0 || !strings.Contains(out, "red") || !strings.Contains(errOut, "1 line(s) could not be read") {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if r := e.js("", "history"); len(r["records"].([]any)) != 1 || r["skipped"].(float64) != 1 {
		t.Fatalf("history --json: %v", r)
	}
}
