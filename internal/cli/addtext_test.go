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

// A pasted command may arrive unquoted (several arguments); anything else
// takes exactly one source, and a stray argument is refused before
// anything is fetched (no cache clone or lock file for the garbage).
func TestAddExtraArguments(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.skill("up/skills/eng/tdd", "tdd")
	up := e.repo("up")
	_, errOut, code := e.skd("", "add", up, "extra")
	if code == 0 || !strings.Contains(errOut, "one source") {
		t.Fatalf("add <src> extra: exit %d\n%s", code, errOut)
	}
	if entries, _ := os.ReadDir(e.p("cache")); len(entries) != 0 {
		t.Fatalf("cache touched: %v", entries)
	}
	// Unquoted install command: joined.
	r := e.js("", "add", "npx", "skills", "add", up, "--skill", "tdd")
	if got := r["skills"]; len(got.([]any)) != 1 {
		t.Fatalf("unquoted command: %v", r)
	}
}

// A source starting with "-" would be an option to git: refused, from a
// plain argument, a pasted command or a skills.sh link, and nothing runs.
func TestAddRefusesDashSources(t *testing.T) {
	e := newEnv(t)
	e.ok("", "init", "--no-skill", e.p("lib"))
	marker := e.p("pwned")
	for _, src := range []string{
		"--upload-pack=touch " + marker,
		"-oProxyCommand=touch " + marker,
		"npx skills add -- --upload-pack=x/y",
		"skills.sh/-uevil/x",
		"o/r@--upload-pack=x",
	} {
		out, errOut, code := e.skd("", "add", "--all", "--", src)
		if code == 0 {
			t.Errorf("%q accepted", src)
		}
		if strings.Contains(out+errOut, "fetching") || strings.Contains(errOut, "unknown flag") {
			t.Errorf("%q: fetched or not parsed\n%s\n%s", src, out, errOut)
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("git ran an option from the source")
	}
	if entries, _ := os.ReadDir(e.p("cache")); len(entries) != 0 {
		t.Fatalf("cache touched: %v", entries)
	}
}

// A repository that is not there is said in plain words, in either
// language, with git's own message after it.
func TestAddMissingRepositorySaysSo(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	missing := e.p("nowhere/repo")
	for lang, want := range map[string]string{"en": "there is no git repository at " + missing, "zh": missing + " 不是 git 仓库"} {
		e.setLang(lang)
		_, errOut, code := e.skd(lib, "add", missing)
		if code == 0 || !strings.Contains(errOut, want) || !strings.Contains(errOut, "git clone") {
			t.Errorf("%s: exit %d\n%s", lang, code, errOut)
		}
	}
}
