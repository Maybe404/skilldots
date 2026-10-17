package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The first command after upgrading from an skd that kept no record of
// what it placed (no placed.json) treats the app's links as that skd
// did: disabling the app, for every agent or one, or taking it out of
// skills.toml by hand removes them outright, never to the trash and
// never left behind.
func TestOlderAppLinksGoOnTheFirstCommand(t *testing.T) {
	for _, first := range [][]string{{"disable", "x"}, {"disable", "x", "--agent", "claude"}, {"apply"}} {
		e := newEnv(t)
		lib := e.p("lib")
		e.ok("", "init", "--no-skill", "--targets", "agents,claude", lib)
		app := e.p("Apps/X.app/Skills/x")
		e.skill("Apps/X.app/Skills/x", "x")
		e.skill("lib/skills/mine", "mine")
		b, _ := os.ReadFile(filepath.Join(lib, "skills.toml"))
		os.WriteFile(filepath.Join(lib, "skills.toml"), append(b, []byte("\n[[app]]\nname = \"x\"\npath = \""+app+"\"\n")...), 0o644)
		e.ok(lib, "apply")
		// What the older skd left: the same links, no record of them.
		placed := filepath.Join(filepath.Dir(e.configPath()), "placed.json")
		if err := os.Remove(placed); err != nil {
			t.Fatal(err)
		}
		if first[0] == "apply" {
			b, _ := os.ReadFile(filepath.Join(lib, "skills.toml"))
			s := string(b)
			os.WriteFile(filepath.Join(lib, "skills.toml"), []byte(s[:strings.Index(s, "[[app]]")]), 0o644)
		}
		out := e.ok(lib, first...)
		gone := []string{"agents", "claude"}
		if len(first) == 4 {
			gone = []string{"claude"}
			isLink(t, e.p(".agents/skills/x"), app)
		}
		for _, tn := range gone {
			if _, err := os.Lstat(e.p("." + tn + "/skills/x")); err == nil {
				t.Fatalf("%v: the link in %s is still there\n%s", first, tn, out)
			}
		}
		if strings.Contains(out, "cannot tell") {
			t.Fatalf("%v:\n%s", first, out)
		}
		if _, err := os.Stat(filepath.Join(lib, ".trash")); err == nil {
			t.Fatalf("%v: something went to the trash\n%s", first, out)
		}
		isLink(t, e.p(".claude/skills/mine"), lib)
		if _, err := os.Stat(filepath.Join(app, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
}
