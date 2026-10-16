package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Maybe404/skilldots/internal/engine"
)

// ClaudeSettingsPath is Claude Code's user settings file:
// $CLAUDE_CONFIG_DIR/settings.json, else ~/.claude/settings.json.
func ClaudeSettingsPath() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "settings.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

// LinksForClaude reports whether e is linked where Claude Code reads
// skills: the claude target, or any target whose directory is Claude's
// (skills/ beside ClaudeSettingsPath).
func LinksForClaude(e engine.Entry) bool {
	dir := filepath.Join(filepath.Dir(ClaudeSettingsPath()), "skills")
	for _, t := range e.Targets {
		if t.Name == "claude" || filepath.Clean(t.Path) == dir {
			return true
		}
	}
	return false
}

// ClaudeDisabled returns the skills turned off inside Claude Code: its
// settings' skillOverrides, {"name": "off"}. skd only reads the file,
// never writes it; a missing or malformed file is no skills.
func ClaudeDisabled() map[string]bool {
	off := map[string]bool{}
	b, err := os.ReadFile(ClaudeSettingsPath())
	if err != nil {
		return off
	}
	var settings struct {
		SkillOverrides map[string]json.RawMessage `json:"skillOverrides"`
	}
	if json.Unmarshal(b, &settings) != nil {
		return off
	}
	for name, raw := range settings.SkillOverrides {
		var s string
		var on bool
		switch {
		case json.Unmarshal(raw, &s) == nil:
			if v := strings.ToLower(strings.TrimSpace(s)); v == "off" || v == "disabled" {
				off[name] = true
			}
		case json.Unmarshal(raw, &on) == nil && !on:
			off[name] = true
		}
	}
	return off
}
