package cli_test

import (
	"encoding/json"
	"os"
	"testing"
)

// Pausing an agent takes out skd's links there and keeps every skill's
// selection: editing a skill's agents meanwhile does not drop it, and
// resuming puts the links back.
func TestPauseAndResumeAgent(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	if err := os.MkdirAll(e.p(".claude/skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	e.skill("lib/skills/mine", "mine")
	e.ok(lib, "targets", "default", "agents", "claude")
	isLink(t, e.p(".claude/skills/mine"), lib)

	e.ok(lib, "targets", "pause", "claude")
	if _, err := os.Lstat(e.p(".claude/skills/mine")); !os.IsNotExist(err) {
		t.Fatalf("paused agent keeps its link: %v", err)
	}
	isLink(t, e.p(".agents/skills/mine"), lib)

	var rows []struct {
		Name            string
		Paused, Default bool
	}
	if err := json.Unmarshal([]byte(e.ok(lib, "targets", "--json")), &rows); err != nil {
		t.Fatal(err)
	}
	paused := map[string]bool{}
	for _, r := range rows {
		paused[r.Name] = r.Paused
	}
	if !paused["claude"] || paused["agents"] {
		t.Fatalf("targets --json paused = %v", paused)
	}

	// An edit of the skill's agents while claude is paused keeps claude.
	e.ok(lib, "disable", "skills:mine", "--agent", "agents")
	e.ok(lib, "targets", "resume", "claude")
	isLink(t, e.p(".claude/skills/mine"), lib)
	if _, err := os.Lstat(e.p(".agents/skills/mine")); !os.IsNotExist(err) {
		t.Fatalf("agents link back after disable: %v", err)
	}
	if _, _, code := e.skd(lib, "targets", "resume", "claude"); code == 0 {
		t.Fatal("resuming an agent that is not paused succeeded")
	}
	if _, _, code := e.skd(lib, "targets", "pause", "no-such-agent"); code == 0 {
		t.Fatal("pausing an unknown agent succeeded")
	}
}
