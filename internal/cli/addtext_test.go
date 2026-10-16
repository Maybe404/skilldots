package cli_test

import (
	"os"
	"strings"
	"testing"
)

// A pasted install command works as the source; without a terminal the
// skills it names are the selection, and nothing in it is run.
func TestAddPastedCommand(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	for _, s := range []string{"eng/tdd", "eng/review", "prod/grill"} {
		e.skill("up/skills/"+s, s[strings.Index(s, "/")+1:])
	}
	up := e.repo("up")
	marker := e.p("ran")
	r := e.js("", "add", "npx -y skills@latest add "+up+" --skill tdd,grill -g && touch "+marker)
	if got := r["skills"]; len(got.([]any)) != 2 {
		t.Fatalf("add: %v", r)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the pasted text was run")
	}
	b, _ := os.ReadFile(e.p("lib/skills.toml"))
	if !strings.Contains(string(b), `use = ['tdd', 'grill']`) && !strings.Contains(string(b), `use = ["tdd", "grill"]`) {
		t.Fatalf("skills.toml:\n%s", b)
	}
	// A name the repository does not have fails without a terminal.
	_, errOut, code := e.skd("", "add", "npx skills add "+up+" -s nope")
	if code == 0 || !strings.Contains(errOut, "nope") {
		t.Fatalf("missing skill: exit %d\n%s", code, errOut)
	}
	// Without a source and without a terminal it says what to give.
	_, errOut, code = e.skd("", "add")
	if code == 0 || !strings.Contains(errOut, "give a source") {
		t.Fatalf("no source: exit %d\n%s", code, errOut)
	}
}
