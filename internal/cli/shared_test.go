package cli_test

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

// sharedEnv is a machine with Claude Code, Codex (which reads
// ~/.agents/skills too) and Kiro (which does not), and a library whose
// default targets are agents and claude.
func sharedEnv(t *testing.T) (*env, string) {
	t.Helper()
	e := newEnv(t)
	e.installed(".claude", ".codex", ".kiro")
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.ok(lib, "targets", "default", "agents", "claude")
	return e, lib
}

// viaShared is the ViaShared of skill name in `skd list --json`.
func viaShared(t *testing.T, e *env, lib, name string) []string {
	t.Helper()
	var entries []struct {
		LinkName  string
		ViaShared []string
	}
	out := e.ok(lib, "list", "--json")
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, x := range entries {
		if x.LinkName == name {
			return x.ViaShared
		}
	}
	t.Fatalf("no %s in\n%s", name, out)
	return nil
}

func absentPath(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Lstat(p); !os.IsNotExist(err) {
		t.Fatalf("%s is there: %v", p, err)
	}
}

// Putting a skill on Codex that is already in the shared directory makes
// no second link: Codex reads it there, and skd says so.
func TestSharedEnableNamedAgent(t *testing.T) {
	e, lib := sharedEnv(t)
	e.ok(lib, "new", "x")
	out := e.ok(lib, "enable", "x", "--agent", "codex")
	if !strings.Contains(out, "codex reads x from the shared directory") {
		t.Fatalf("enable said:\n%s", out)
	}
	absentPath(t, e.p(".codex/skills/x"))
	isLink(t, e.p(".agents/skills/x"), lib)
	if v := viaShared(t, e, lib, "x"); !slices.Equal(v, []string{"codex"}) {
		t.Fatalf("ViaShared %v", v)
	}
	if out := e.ok(lib, "list", "--agent", "codex"); !strings.Contains(out, "x") || !strings.Contains(out, "through agents") {
		t.Fatalf("list --agent codex:\n%s", out)
	}
	// Taking it off codex alone changes nothing on disk: it says how.
	if out := e.ok(lib, "disable", "x", "--agent", "codex"); !strings.Contains(out, "still reads x from the shared directory") {
		t.Fatalf("disable said:\n%s", out)
	}
}

// "*" with defaults that do not name agents: agents comes in as an
// installed agent, and Codex still gets no link of its own.
func TestSharedEveryAgentWithoutAgentsInDefaults(t *testing.T) {
	e, lib := sharedEnv(t)
	e.ok(lib, "targets", "default", "claude")
	e.installed(".agents") // other agents use the shared directory here
	e.skill("lib/skills/w", "w")
	b, _ := os.ReadFile(e.p("lib/skills.toml"))
	os.WriteFile(e.p("lib/skills.toml"), append(b, []byte("\n[[local]]\nname = \"w\"\ntargets = [\"*\"]\n")...), 0o644)
	e.ok(lib, "apply")
	for _, d := range []string{".agents", ".claude", ".kiro"} {
		isLink(t, e.p(d+"/skills/w"), lib)
	}
	absentPath(t, e.p(".codex/skills/w"))
}

// Adopting: a copy in Codex's own directory of a skill the library links
// into the shared directory goes to the trash (Codex reads the library's
// one there), and a skill found only in Codex's directory is adopted
// with no link of its own there either.
func TestSharedAdopt(t *testing.T) {
	e, lib := sharedEnv(t)
	e.ok(lib, "new", "y")
	b, _ := os.ReadFile(e.p("lib/skills/y/SKILL.md"))
	e.write(".codex/skills/y/SKILL.md", string(b))
	e.skill(".codex/skills/z", "z")
	if out := e.ok(lib, "adopt", "-n"); !strings.Contains(out, "codex already reads the library's copy from ~/.agents/skills") {
		t.Fatalf("adopt -n:\n%s", out)
	}
	e.ok(lib, "adopt", "--yes")
	absentPath(t, e.p(".codex/skills/y"))
	absentPath(t, e.p(".codex/skills/z"))
	isLink(t, e.p(".agents/skills/z"), lib)
	isLink(t, e.p(".agents/skills/y"), lib)
	if v := viaShared(t, e, lib, "z"); !slices.Contains(v, "codex") {
		t.Fatalf("z ViaShared %v", v)
	}
	trash := e.ok(lib, "trash")
	if !strings.Contains(trash, "y") || !strings.Contains(trash, "z") {
		t.Fatalf("the copies are not in the trash:\n%s", trash)
	}
}

// With the shared directory paused, Codex gets its own links; editing
// agents meanwhile (a pack's, a skill's) keeps Codex selected, and after
// resume each agent has the skill once.
func TestSharedPausedAndEdited(t *testing.T) {
	e, lib := sharedEnv(t)
	e.skill("up/skills/pk", "pk")
	up := e.repo("up")
	e.ok(lib, "add", up, "--skill", "pk")
	e.ok(lib, "enable", "pk", "--agent", "codex")
	e.skill("lib/skills/w", "w")
	b, _ := os.ReadFile(e.p("lib/skills.toml"))
	os.WriteFile(e.p("lib/skills.toml"), append(b, []byte("\n[[local]]\nname = \"w\"\ntargets = [\"agents\", \"claude\", \"*\"]\n")...), 0o644)
	e.ok(lib, "apply")
	absentPath(t, e.p(".codex/skills/pk"))
	absentPath(t, e.p(".codex/skills/w"))

	e.ok(lib, "targets", "pause", "agents")
	isLink(t, e.p(".codex/skills/pk"), lib)
	isLink(t, e.p(".codex/skills/w"), lib)
	// Editing the pack's agents while paused does not drop Codex.
	e.ok(lib, "disable", up, "--agent", "claude")
	isLink(t, e.p(".codex/skills/pk"), lib)
	// Nor does editing one skill's.
	e.ok(lib, "disable", "w", "--agent", "kiro")
	isLink(t, e.p(".codex/skills/w"), lib)

	e.ok(lib, "targets", "resume", "agents")
	for _, n := range []string{"pk", "w"} {
		isLink(t, e.p(".agents/skills/"+n), lib)
		absentPath(t, e.p(".codex/skills/"+n))
	}
}

// When the shared directory's entry is not skd's (a directory of the
// user's that apply skips), Codex keeps a link of its own.
func TestSharedNotServedKeepsOwnLink(t *testing.T) {
	e, lib := sharedEnv(t)
	e.skill("lib/skills/beta", "beta")
	b, _ := os.ReadFile(e.p("lib/skills.toml"))
	os.WriteFile(e.p("lib/skills.toml"), append(b, []byte("\n[[local]]\nname = \"beta\"\ntargets = [\"agents\", \"codex\"]\n")...), 0o644)
	e.ok(lib, "apply")
	absentPath(t, e.p(".codex/skills/beta"))
	// The user replaces the shared entry with a directory of their own.
	os.Remove(e.p(".agents/skills/beta"))
	e.skill(".agents/skills/beta", "beta")
	e.ok(lib, "apply")
	isLink(t, e.p(".codex/skills/beta"), lib)
	if v := viaShared(t, e, lib, "beta"); len(v) != 0 {
		t.Fatalf("ViaShared %v while the shared entry is the user's", v)
	}
	if st, err := os.Lstat(e.p(".agents/skills/beta")); err != nil || st.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("the user's directory was touched: %v", err)
	}
}
