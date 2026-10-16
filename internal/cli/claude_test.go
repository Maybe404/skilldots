package cli_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestListMarksClaudeOff(t *testing.T) {
	e := newEnv(t)
	e.ok("", "init", "--no-skill", e.p("lib"))
	e.ok("", "new", "notes")
	e.ok("", "new", "other")
	e.write(".claude/settings.json", `{"skillOverrides": {"notes": "off"}}`)
	out, _, code := e.skd("", "list", "--json")
	if code != 0 {
		t.Fatal(out)
	}
	var entries []struct {
		Name           string
		ClaudeDisabled bool
	}
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, en := range entries {
		got[en.Name] = en.ClaudeDisabled
	}
	if !got["notes"] || got["other"] {
		t.Fatalf("list --json: %s", out)
	}
	if out := e.ok("", "list", "--flat"); !strings.Contains(out, "off in Claude Code") || !strings.Contains(out, "--agent claude") {
		t.Fatalf("list:\n%s", out)
	}
	// A broken settings file is ignored, quietly.
	e.write(".claude/settings.json", `{`)
	out, errOut, code := e.skd("", "list", "--flat")
	if code != 0 || strings.Contains(out+errOut, "off in Claude Code") || strings.Contains(errOut, "settings") {
		t.Fatalf("broken settings: %d\n%s\n%s", code, out, errOut)
	}
	if b, _ := os.ReadFile(e.p(".claude/settings.json")); string(b) != "{" {
		t.Fatal("settings.json was written")
	}
}
