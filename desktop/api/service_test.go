package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func invoke(t *testing.T, s *Service, name string, args ...any) any {
	t.Helper()
	raw := []json.RawMessage{}
	for _, a := range args {
		b, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, b)
	}
	out, err := s.Invoke(context.Background(), name, raw)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return out
}
func setup(t *testing.T) *Service {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SKILLDOTS_LIBRARY", "")
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(home, "cache"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	s := New("", Native{})
	invoke(t, s, "initLibrary", filepath.Join(home, "library"))
	return s
}
func TestDesktopDistributionRoundTrip(t *testing.T) {
	s := setup(t)
	invoke(t, s, "newSkill", object{"name": "desktop-check", "description": "真实后端", "private": false, "agents": []string{"agents", "claude"}})
	home, _ := os.UserHomeDir()
	link := filepath.Join(home, ".claude", "skills", "desktop-check")
	if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	invoke(t, s, "setAgentEnabled", "claude", false)
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("suspended agent still has a managed link")
	}
	invoke(t, s, "setAgentEnabled", "claude", true)
	r := ref{Key: "skills:desktop-check"}
	text := invoke(t, s, "readFile", r, "SKILL.md").(string)
	invoke(t, s, "writeFile", r, "SKILL.md", text+"\nEdited in the desktop.\n")
	invoke(t, s, "disable", r, []string{"claude"})
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("disabled link remains")
	}
	invoke(t, s, "enable", r, []string{"claude"})
	if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	invoke(t, s, "remove", r)
	items := invoke(t, s, "trash")
	b, _ := json.Marshal(items)
	var rows []struct {
		ID         string
		Restorable bool
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].Restorable {
		t.Fatalf("trash: %s", b)
	}
	invoke(t, s, "restore", rows[0].ID)
	if got := invoke(t, s, "readFile", r, "SKILL.md"); got != text+"\nEdited in the desktop.\n" {
		t.Fatalf("restored content differs: %s", got)
	}
	for _, name := range []string{"deleteForever", "emptyTrash"} {
		if _, err := s.Invoke(context.Background(), name, nil); err == nil {
			t.Fatalf("permanent deletion exposed: %s", name)
		}
	}
}
func TestDesktopFilesCannotEscapeSkill(t *testing.T) {
	s := setup(t)
	invoke(t, s, "newSkill", object{"name": "safe", "agents": []string{"agents"}})
	r := ref{Key: "skills:safe"}
	home, _ := os.UserHomeDir()
	outside := filepath.Join(home, "outside.txt")
	if err := os.WriteFile(outside, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.root, "skills", "safe", "link.txt")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../../outside.txt", outside, "link.txt", ".git/config"} {
		raw := []json.RawMessage{}
		for _, a := range []any{r, path, "changed"} {
			b, _ := json.Marshal(a)
			raw = append(raw, b)
		}
		if _, err := s.Invoke(context.Background(), "writeFile", raw); err == nil {
			t.Fatalf("write escaped through %q", path)
		}
	}
	if b, err := os.ReadFile(outside); err != nil || string(b) != "untouched" {
		t.Fatalf("outside file changed: %q %v", b, err)
	}
}

func TestHiddenAgentDoesNotChangeDistribution(t *testing.T) {
	s := setup(t)
	invoke(t, s, "newSkill", object{"name": "visible", "agents": []string{"claude"}})
	manifestPath := filepath.Join(s.root, "skills.toml")
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	invoke(t, s, "setAgentHidden", "claude", true)
	home, _ := os.UserHomeDir()
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "visible", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil || string(before) != string(after) {
		t.Fatal("hiding changed manifest", err)
	}
	p, err := readPreferences()
	if err != nil || !p.HiddenAgents["claude"] {
		t.Fatal("hidden preference not persisted", err)
	}
	invoke(t, s, "setAgentHidden", "claude", false)
	p, err = readPreferences()
	v, present := p.HiddenAgents["claude"]
	if err != nil || !present || v {
		t.Fatal("explicit visibility not persisted", err)
	}
}
