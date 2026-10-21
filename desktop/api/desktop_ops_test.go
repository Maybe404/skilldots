package api

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func roundTrip[T any](t *testing.T, v any) T {
	t.Helper()
	var out T
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	return out
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeGitHub serves acme/skills from a local repository in place of
// github.com (through the isolated git configuration setup made).
func fakeGitHub(t *testing.T) (repo string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	up := t.TempDir()
	repo = filepath.Join(up, "acme", "skills.git")
	for _, s := range []string{"eng/tdd", "eng/review", "prod/plan"} {
		writeFile(t, filepath.Join(repo, "skills", s, "SKILL.md"), "---\nname: "+filepath.Base(s)+"\ndescription: "+filepath.Base(s)+" skill\n---\nbody\n")
	}
	runGit(t, repo, "init", "-q", "-b", "main")
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "init")
	writeFile(t, os.Getenv("GIT_CONFIG_GLOBAL"), "[user]\n\tname = t\n\temail = t@t\n[url \"file://"+up+"/\"]\n\tinsteadOf = https://github.com/\n")
	return repo
}

type previewOut struct {
	Source      source
	Commit      string
	Preselected []string
	Missing     []string
}

// show reports the names it could not find with the preview instead of
// failing, and preselects by name, category/name and directory alike.
func TestShowReportsMissingPreselection(t *testing.T) {
	s := setup(t)
	fakeGitHub(t)
	parsed := invoke(t, s, "parseSource", "npx skills add acme/skills --skill tdd --skill nope")
	p := roundTrip[previewOut](t, invoke(t, s, "show", parsed))
	if !slices.Equal(p.Missing, []string{"nope"}) {
		t.Fatalf("missing: %v", p.Missing)
	}
	if !slices.Equal(p.Preselected, []string{"tdd"}) {
		t.Fatalf("preselected: %v", p.Preselected)
	}
	if p.Commit == "" || p.Source.Repo != "acme/skills" {
		t.Fatalf("preview: %+v", p)
	}
	// A link to a folder preselects the skills there, by directory; a
	// link to a folder that does not exist is reported, not an error.
	parsed = invoke(t, s, "parseSource", "https://github.com/acme/skills/tree/main/skills/eng/review")
	if p = roundTrip[previewOut](t, invoke(t, s, "show", parsed)); !slices.Equal(p.Preselected, []string{"review"}) || len(p.Missing) != 0 {
		t.Fatalf("tree link: %+v", p)
	}
	parsed = invoke(t, s, "parseSource", "https://github.com/acme/skills/tree/main/skills/gone")
	if p = roundTrip[previewOut](t, invoke(t, s, "show", parsed)); len(p.Preselected) != 0 || len(p.Missing) != 1 {
		t.Fatalf("missing folder: %+v", p)
	}
	// Nothing named: nothing missing, nothing preselected, both still lists.
	parsed = invoke(t, s, "parseSource", "acme/skills")
	raw := invoke(t, s, "show", parsed)
	b, _ := json.Marshal(raw)
	var m map[string]json.RawMessage
	json.Unmarshal(b, &m)
	if string(m["missing"]) != "[]" || string(m["preselected"]) != "[]" {
		t.Fatalf("empty lists: %s", b)
	}
}

func TestUpdateDiffThroughTheBridge(t *testing.T) {
	s := setup(t)
	repo := fakeGitHub(t)
	parsed := invoke(t, s, "parseSource", "npx skills add acme/skills --skill tdd")
	p := roundTrip[previewOut](t, invoke(t, s, "show", parsed))
	invoke(t, s, "add", object{"Source": p.Source, "Commit": p.Commit}, object{"skills": []string{"tdd"}, "agents": []string{"agents"}})

	writeFile(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), "---\nname: tdd\ndescription: tdd skill\n---\nbody\nmore\n")
	runGit(t, repo, "commit", "-q", "-am", "more")
	d := roundTrip[struct {
		Repo, Commit, Latest string
		Skills               []struct {
			Name, Status string
			Local        bool
			Files        []struct {
				Path, Status   string
				Added, Removed int
			}
		}
	}](t, invoke(t, s, "updateDiff", "acme/skills", ""))
	if d.Repo != "acme/skills" || d.Commit == d.Latest || len(d.Skills) != 1 {
		t.Fatalf("%+v", d)
	}
	sk := d.Skills[0]
	if sk.Name != "tdd" || sk.Status != "changed" || sk.Local || len(sk.Files) != 1 || sk.Files[0].Path != "SKILL.md" || sk.Files[0].Added != 1 {
		t.Fatalf("%+v", sk)
	}
	if _, err := s.Invoke(context.Background(), "updateDiff", mustArgs(t, "nobody/nothing")); err == nil {
		t.Fatal("diff of a pack not subscribed")
	}
}

// With no library yet, status still counts what the agents have (for the
// first-run screen) without creating anything.
func TestStatusWithoutLibraryCountsUnmanaged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SKILLDOTS_LIBRARY", "")
	writeFile(t, filepath.Join(home, ".claude", "skills", "one", "SKILL.md"), "---\nname: one\ndescription: one\n---\n")
	writeFile(t, filepath.Join(home, ".agents", "skills", "two", "SKILL.md"), "---\nname: two\ndescription: two\n---\n")
	s := New("", Native{})
	st := roundTrip[struct {
		Root      *string
		Unmanaged int
	}](t, invoke(t, s, "status"))
	if st.Root != nil || st.Unmanaged != 2 {
		t.Fatalf("status: %+v", st)
	}
	if _, err := os.Stat(filepath.Join(home, ".skilldots")); !os.IsNotExist(err) {
		t.Fatal("status made a library")
	}
}

// An agent not installed here gets no link: the bridge says which skills
// wanted it, the skill still names it, and the agents list says which
// agents read the shared directory.
func TestAbsentAgentAndSharedReaders(t *testing.T) {
	s := setup(t) // Claude Code is installed, Codex is not
	invoke(t, s, "newSkill", object{"name": "here", "agents": []string{"claude"}})
	got := roundTrip[struct{ Absent map[string][]string }](t, invoke(t, s, "enable", ref{Key: "skills:here"}, []string{"codex"}))
	if len(got.Absent["codex"]) != 1 || got.Absent["codex"][0] != "here" {
		t.Fatalf("absent: %+v", got.Absent)
	}
	home, _ := os.UserHomeDir()
	if _, err := os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
		t.Fatal("made a directory for an agent not installed")
	}
	skills := roundTrip[[]struct {
		Name            string
		Targets, Absent []string
	}](t, invoke(t, s, "skills"))
	for _, sk := range skills {
		if sk.Name == "here" && (!slices.Contains(sk.Targets, "codex") || !slices.Equal(sk.Absent, []string{"codex"})) {
			t.Fatalf("skill: %+v", sk)
		}
	}
	agents := roundTrip[[]struct {
		Name        string
		ReadsShared bool
	}](t, invoke(t, s, "agents"))
	shared := map[string]bool{}
	for _, a := range agents {
		shared[a.Name] = a.ReadsShared
	}
	if !shared["codex"] || shared["claude"] || shared["agents"] {
		t.Fatalf("readsShared: %+v", shared)
	}
	if applied := roundTrip[struct{ Absent map[string][]string }](t, invoke(t, s, "apply", object{"force": false})); len(applied.Absent["codex"]) != 1 {
		t.Fatalf("apply absent: %+v", applied)
	}
}

// A skill on Agents reaches the agents that read ~/.agents/skills: those
// among its targets get no link of their own (viaShared), and the bridge
// says which installed agents read it there at all (sharedReach). With
// Agents paused, each agent gets its own link again.
func TestViaSharedAndSharedReach(t *testing.T) {
	s := setup(t)
	home, _ := os.UserHomeDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil { // Codex is installed
		t.Fatal(err)
	}
	invoke(t, s, "newSkill", object{"name": "both", "agents": []string{"agents", "codex"}})
	invoke(t, s, "newSkill", object{"name": "shared", "agents": []string{"agents"}})
	type row struct {
		Name                                    string
		Targets, ViaShared, SharedReach, Absent []string
	}
	find := func() map[string]row {
		out := map[string]row{}
		for _, r := range roundTrip[[]row](t, invoke(t, s, "skills")) {
			out[r.Name] = r
		}
		return out
	}
	got := find()
	if b := got["both"]; !slices.Contains(b.Targets, "codex") || !slices.Equal(b.ViaShared, []string{"codex"}) || !slices.Equal(b.SharedReach, []string{"codex"}) {
		t.Fatalf("both: %+v", b)
	}
	if sh := got["shared"]; slices.Contains(sh.Targets, "codex") || len(sh.ViaShared) != 0 || !slices.Equal(sh.SharedReach, []string{"codex"}) {
		t.Fatalf("shared: %+v", sh)
	}
	if _, err := os.Lstat(filepath.Join(home, ".codex", "skills", "both")); !os.IsNotExist(err) {
		t.Fatal("a skill read through ~/.agents/skills was linked into Codex too")
	}
	// Gemini CLI reads the shared folder too but is not installed: not listed.
	invoke(t, s, "setAgentEnabled", "agents", false)
	got = find()
	if b := got["both"]; len(b.ViaShared) != 0 || len(b.SharedReach) != 0 {
		t.Fatalf("paused Agents still serves: %+v", b)
	}
	if _, err := os.Lstat(filepath.Join(home, ".codex", "skills", "both")); err != nil {
		t.Fatalf("with Agents paused, Codex has no link of its own: %v", err)
	}
}

// Restoring a removed skill links it again in the same call (the app
// layer applies); the result says so.
func TestRestoreLinksAgain(t *testing.T) {
	s := setup(t)
	invoke(t, s, "newSkill", object{"name": "back", "agents": []string{"claude"}})
	invoke(t, s, "remove", ref{Key: "skills:back"})
	home, _ := os.UserHomeDir()
	link := filepath.Join(home, ".claude", "skills", "back")
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("removed skill still linked")
	}
	rows := roundTrip[[]struct {
		ID, Reason string
		Skills     []string
	}](t, invoke(t, s, "trash"))
	if len(rows) != 1 || rows[0].Reason != "remove" {
		t.Fatalf("trash: %+v", rows)
	}
	out := roundTrip[struct {
		Linked bool
		Skills []string
	}](t, invoke(t, s, "restore", rows[0].ID))
	if !out.Linked {
		t.Fatalf("restore: %+v", out)
	}
	if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
		t.Fatalf("not linked again: %v", err)
	}
}

// Scan rows carry where an entry came from and whether it differs.
func TestScanRowsCarryUpstream(t *testing.T) {
	s := setup(t)
	home, _ := os.UserHomeDir()
	writeFile(t, filepath.Join(home, ".claude", "skills", "loose", "SKILL.md"), "---\nname: loose\ndescription: d\n---\n")
	b, _ := json.Marshal(invoke(t, s, "scan"))
	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r["name"] == "loose" {
			if _, ok := r["differs"]; !ok {
				t.Fatalf("no differs: %v", r)
			}
			if _, ok := r["upstream"]; !ok {
				t.Fatalf("no upstream: %v", r)
			}
			return
		}
	}
	t.Fatalf("loose not found: %s", b)
}

// Adopting takes each row as the confirmation showed it: a row whose
// proposal is now of another kind is refused.
func TestAdoptChecksTheKindShown(t *testing.T) {
	s := setup(t)
	home, _ := os.UserHomeDir()
	writeFile(t, filepath.Join(home, ".claude", "skills", "loose", "SKILL.md"), "---\nname: loose\ndescription: d\n---\n")
	rows := roundTrip[[]struct{ ID, Proposal string }](t, invoke(t, s, "scan"))
	var row struct{ ID, Proposal string }
	for _, r := range rows {
		if r.ID == "claude/loose" {
			row = r
		}
	}
	if row.Proposal != "own" {
		t.Fatalf("scan: %+v", rows)
	}
	for _, picks := range []any{[]string{row.ID}, []object{{"id": row.ID, "kind": "pack"}}, []object{{"id": "claude/nothing", "kind": "own"}}} {
		if _, err := s.Invoke(context.Background(), "adopt", mustArgs(t, picks)); err == nil {
			t.Fatalf("adopted with %v", picks)
		}
	}
	if _, err := os.Stat(filepath.Join(s.root, "skills", "loose")); !os.IsNotExist(err) {
		t.Fatal("a refused adopt changed the library")
	}
	out := roundTrip[struct{ Adopted int }](t, invoke(t, s, "adopt", []object{{"id": row.ID, "kind": "own"}}))
	if out.Adopted != 1 {
		t.Fatalf("adopted %d", out.Adopted)
	}
	if _, err := os.Stat(filepath.Join(s.root, "skills", "loose", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestTrashDeletionNeedsConfirmation(t *testing.T) {
	s := setup(t)
	for _, n := range []string{"a", "b", "c"} {
		invoke(t, s, "newSkill", object{"name": n, "agents": []string{"agents"}})
		invoke(t, s, "remove", ref{Key: "skills:" + n})
	}
	type entry struct {
		ID   string
		Size int64
	}
	rows := roundTrip[[]entry](t, invoke(t, s, "trash"))
	if len(rows) != 3 || rows[0].Size <= 0 {
		t.Fatalf("trash: %+v", rows)
	}
	usage := roundTrip[struct{ Count, Bytes int64 }](t, invoke(t, s, "trashUsage"))
	if usage.Count != 3 || usage.Bytes <= 0 {
		t.Fatalf("usage: %+v", usage)
	}

	invoke(t, s, "deleteTrash", rows[0].ID)
	if left := roundTrip[[]entry](t, invoke(t, s, "trash")); len(left) != 2 {
		t.Fatalf("after deleting one: %+v", left)
	}
	if _, err := s.Invoke(context.Background(), "deleteTrash", mustArgs(t, rows[0].ID)); err == nil {
		t.Fatal("deleted the same entry twice")
	}

	// The entries the person confirmed must be exactly the trash's: not
	// just as many (one in and one out since then is refused too).
	shown := []string{}
	for _, e := range roundTrip[[]entry](t, invoke(t, s, "trash")) {
		shown = append(shown, e.ID)
	}
	invoke(t, s, "newSkill", object{"name": "late", "agents": []string{"agents"}})
	invoke(t, s, "remove", ref{Key: "skills:late"})
	invoke(t, s, "deleteTrash", shown[0])
	now := roundTrip[[]entry](t, invoke(t, s, "trash"))
	if len(now) != len(shown) {
		t.Fatalf("setup: %d entries, shown %d", len(now), len(shown))
	}
	for _, args := range [][]json.RawMessage{nil, mustArgs(t, object{}), mustArgs(t, object{"count": 2}), mustArgs(t, object{"ids": shown}), mustArgs(t, object{"ids": []string{now[0].ID}})} {
		if _, err := s.Invoke(context.Background(), "emptyTrash", args); err == nil {
			t.Fatalf("emptied with %s", args)
		}
	}
	if left := roundTrip[[]entry](t, invoke(t, s, "trash")); len(left) != 2 {
		t.Fatal("a refused empty deleted something")
	}
	ids := []string{now[0].ID, now[1].ID}
	out := roundTrip[struct{ Deleted int }](t, invoke(t, s, "emptyTrash", object{"ids": ids}))
	if out.Deleted != 2 {
		t.Fatalf("deleted %d", out.Deleted)
	}
	if left := roundTrip[[]entry](t, invoke(t, s, "trash")); len(left) != 0 {
		t.Fatalf("left: %+v", left)
	}
	if _, err := os.Stat(filepath.Join(s.root, "skills.toml")); err != nil {
		t.Fatal("emptying the trash touched the library")
	}
}
