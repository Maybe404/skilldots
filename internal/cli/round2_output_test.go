package cli_test

import (
	"os"
	"strings"
	"testing"
)

// skd trash restore takes the repository of a purged pack; purge lists
// the links it takes out once; the trash table says what "!" means; the
// list header and apply count different things with different words;
// an apply replacing an entry says so in words, with ~ for the home
// directory; and update's rows line up.
func TestRoundTwoOutput(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	for _, n := range []string{"grill-me", "tdd"} {
		e.skill("up/skills/"+n, n)
	}
	up := e.repo("up")
	e.ok(lib, "add", up, "--all")

	// The header counts skills in agents; apply counts links.
	if out := e.ok(lib, "list"); !strings.Contains(out, "2 in agents") {
		t.Errorf("list header:\n%s", out)
	}
	if out := e.ok(lib, "apply"); !strings.Contains(out, "up to date, 2 links") {
		t.Errorf("apply:\n%s", out)
	}

	// update: the "see changes" row lines up with the others.
	e.write("up/skills/tdd/SKILL.md", "---\nname: tdd\ndescription: d2\n---\nbody\n")
	e.git(up, "commit", "-qam", "tdd")
	out := e.ok(lib, "update", "--yes")
	var col int
	for _, l := range strings.Split(out, "\n") {
		for _, label := range []string{"changed", "see changes"} {
			if strings.HasPrefix(l, "  "+label) {
				rest := strings.TrimPrefix(l, "  "+label)
				c := len(label) + len(rest) - len(strings.TrimLeft(rest, " "))
				if col == 0 {
					col = c
				} else if c != col {
					t.Errorf("update rows do not line up:\n%s", out)
				}
			}
		}
	}
	if col == 0 {
		t.Errorf("update:\n%s", out)
	}

	// purge lists its links once.
	out = e.ok(lib, "purge", up, "--yes")
	if n := strings.Count(out, "grill-me"); n != 1 {
		t.Errorf("purge names grill-me %d times:\n%s", n, out)
	}
	// Restored by the repository's name.
	out = e.ok(lib, "trash", "restore", up)
	if !strings.Contains(out, "selected again in skills.toml: grill-me, tdd") {
		t.Errorf("restore by repository:\n%s", out)
	}
	isLink(t, e.p(".agents/skills/tdd"), lib)

	// A link of the user's in the way: replaced in words, ~ for home,
	// and the trash says what its "!" means.
	e.ok(lib, "new", "mine")
	os.Remove(e.p(".agents/skills/mine"))
	os.Symlink(e.p("elsewhere"), e.p(".agents/skills/mine"))
	os.MkdirAll(e.p("elsewhere"), 0o755)
	out = e.ok(lib, "apply", "--force")
	if strings.Contains(out, "↻") || !strings.Contains(out, "relink") || strings.Contains(out, e.home) || !strings.Contains(out, "~/.agents/skills/mine") {
		t.Errorf("apply --force:\n%s", out)
	}
	if out := e.ok(lib, "trash"); !strings.Contains(out, "! marks an entry skd does not restore") {
		t.Errorf("trash:\n%s", out)
	}
}
