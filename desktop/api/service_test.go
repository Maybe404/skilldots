package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Maybe404/skilldots/internal/app"
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

func TestDesktopHistoryAndOutdatedKeepDetails(t *testing.T) {
	now := time.Now().UTC()
	o := outdated(&app.Outdated{Checked: now, Packs: []app.PackStatus{{
		Repo: "example/skills", Commit: "old", Latest: "new",
		Changed: []string{"changed"}, Moved: []string{"moved"}, New: []string{"new-skill"},
		Gone: []string{"gone"}, Pending: []string{"pending"}, Checked: now,
		Commits: []app.UpstreamCommit{{Hash: "abc", Subject: "update skill"}}, MoreCommits: true, CommitsUnknown: true,
	}}})
	b, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Packs []struct {
			Commits                     []struct{ ID, Title string }
			MoreCommits, CommitsUnknown bool
		} `json:"packs"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Packs) != 1 || len(got.Packs[0].Commits) != 1 || got.Packs[0].Commits[0].ID != "abc" || !got.Packs[0].MoreCommits || !got.Packs[0].CommitsUnknown {
		t.Fatalf("outdated details lost: %s", b)
	}

	rows := historyWithBackups([]app.UpdateRecord{{
		Time: now, Repo: "example/skills", From: "old", To: "new",
		Dropped: []string{"dropped"}, New: []string{"upstream-new"},
		MoreCommits: true, CommitsUnknown: true, ApplyError: "apply failed", Omitted: 2,
		Commits: []app.UpstreamCommit{{Hash: "def", Subject: "another update"}},
	}}, []string{"skill/SKILL.md"})
	b, err = json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var historyRows []struct {
		Dropped, New, Backups       []string
		MoreCommits, CommitsUnknown bool
		ApplyError                  string
		Omitted                     int
	}
	if err := json.Unmarshal(b, &historyRows); err != nil {
		t.Fatal(err)
	}
	if len(historyRows) != 1 || len(historyRows[0].Dropped) != 1 || len(historyRows[0].New) != 1 || len(historyRows[0].Backups) != 1 || !historyRows[0].MoreCommits || !historyRows[0].CommitsUnknown || historyRows[0].ApplyError == "" || historyRows[0].Omitted != 2 {
		t.Fatalf("history details lost: %s", b)
	}
}

func TestCloneFailureCleansDestinationAndKeepsLibrary(t *testing.T) {
	s := setup(t)
	oldRoot := s.root
	dir := filepath.Join(t.TempDir(), "cloned")
	_, err := s.cloneLibrary(filepath.Join(t.TempDir(), "missing-repository"), dir)
	if err == nil {
		t.Fatal("clone unexpectedly succeeded")
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("failed clone left destination: %v", statErr)
	}
	if s.root != oldRoot {
		t.Fatalf("service root changed after failed clone: %q", s.root)
	}
	status := invoke(t, s, "status").(map[string]any)
	if status["root"] != oldRoot {
		t.Fatalf("remembered library changed after failed clone: %#v", status["root"])
	}
}

func TestNewSkillDescriptionAndRetry(t *testing.T) {
	s := setup(t)
	invoke(t, s, "newSkill", object{"name": "atomic-check", "description": "description survives one transaction", "private": false, "agents": []string{"agents"}})
	body, err := os.ReadFile(filepath.Join(s.root, "skills", "atomic-check", "SKILL.md"))
	if err != nil || !strings.Contains(string(body), "description: \"description survives one transaction\"") {
		t.Fatalf("description was not written with the skill: %s (%v)", body, err)
	}
	if _, err := s.Invoke(context.Background(), "newSkill", mustArgs(t, object{"name": "atomic-check", "description": "second attempt", "private": false, "agents": []string{"agents"}})); err == nil {
		t.Fatal("duplicate new skill unexpectedly succeeded")
	}
}

func TestNewSkillApplyFailureRollsBackLibrary(t *testing.T) {
	s := setup(t)
	ses, err := s.session()
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(s.root, "skills.toml"))
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	targetRoot := filepath.Join(home, ".agents", "skills")
	outside := t.TempDir()
	if err := os.RemoveAll(targetRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(targetRoot), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, targetRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := ses.NewForTargetsWithDescription("rollback-check", false, []string{"agents"}, "will roll back"); err == nil {
		t.Fatal("new skill unexpectedly succeeded through a changed target")
	}
	if _, statErr := os.Stat(filepath.Join(s.root, "skills", "rollback-check")); !os.IsNotExist(statErr) {
		t.Fatalf("failed new skill left directory: %v", statErr)
	}
	after, err := os.ReadFile(filepath.Join(s.root, "skills.toml"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("failed new skill changed manifest: %v", err)
	}
}

func mustArgs(t *testing.T, values ...any) []json.RawMessage {
	t.Helper()
	args := make([]json.RawMessage, 0, len(values))
	for _, value := range values {
		b, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		args = append(args, b)
	}
	return args
}
