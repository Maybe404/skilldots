package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
)

func commit(t *testing.T, repo, msg string) {
	t.Helper()
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", msg)
}

func subjects(r app.UpdateRecord) []string {
	var out []string
	for _, c := range r.Commits {
		out = append(out, c.Subject)
	}
	return out
}

// Each update writes a record with what changed and the upstream commits
// that touched those skills, read from the shallow cache clone after
// fetching more of its history.
func TestUpdateHistory(t *testing.T) {
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	_, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd,review")

	write(t, filepath.Join(repo, "skills", "eng", "tdd", "a.md"), "a\n")
	commit(t, repo, "tdd: add a")
	write(t, filepath.Join(repo, "skills", "prod", "plan", "x.md"), "x\n")
	commit(t, repo, "plan: not selected")
	write(t, filepath.Join(repo, "README.md"), "readme\n")
	commit(t, repo, "docs: readme")
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "b.md"), "b\n")
	write(t, filepath.Join(repo, "skills", "eng", "fresh", "SKILL.md"), skillMD("fresh"))
	commit(t, repo, "tdd: add b, and fresh")
	os.RemoveAll(filepath.Join(repo, "skills", "eng", "review"))
	commit(t, repo, "review: retire")

	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	from := s.Lib.Lock.Packs["acme/skills"].Commit
	out, err := s.Update(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.History) != 1 {
		t.Fatalf("history: %+v", out.History)
	}
	r := out.History[0]
	if r.Repo != "acme/skills" || r.From != from || r.To == from || r.CommitsUnknown {
		t.Fatalf("record: %+v", r)
	}
	if !slices.Equal(r.Changed, []string{"tdd"}) || !slices.Equal(r.Gone, []string{"review"}) || !slices.Equal(r.New, []string{"fresh"}) {
		t.Fatalf("record lists: %+v", r)
	}
	want := []string{"review: retire", "tdd: add b, and fresh", "tdd: add a"}
	if got := subjects(r); !slices.Equal(got, want) {
		t.Fatalf("commits %q, want %q", got, want)
	}

	// The file is in the library, not ignored, and history reads it back
	// without the network.
	b, err := os.ReadFile(filepath.Join(root, app.HistoryFile))
	if err != nil || strings.Count(string(b), "\n") != 1 {
		t.Fatalf("history file: %v\n%s", err, b)
	}
	cmd := gitOut(t, root, "check-ignore", "-v", app.HistoryFile)
	if cmd != "" {
		t.Fatalf("history file is ignored: %s", cmd)
	}
	os.RemoveAll(os.Getenv("SKILLDOTS_CACHE"))
	s, _ = app.Open(root)
	recs, skipped, err := s.History("acme/skills", 0)
	if err != nil || skipped != 0 || len(recs) != 1 || !slices.Equal(subjects(recs[0]), want) {
		t.Fatalf("History: %v %d %+v", err, skipped, recs)
	}

	// Nothing new upstream: no record.
	s, _ = app.Open(root)
	if out, err := s.Update(nil, nil); err != nil || len(out.History) != 0 {
		t.Fatalf("second update: %v %+v", err, out)
	}
}

// More commits than MaxCommits are cut and flagged; a broken line (a bad
// merge) is skipped, not fatal.
func TestHistoryLimits(t *testing.T) {
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	_, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd")
	for i := range app.MaxCommits + 3 {
		write(t, filepath.Join(repo, "skills", "eng", "tdd", "n.md"), strings.Repeat("x", i+1))
		commit(t, repo, "tdd change")
	}
	runCLI(t, root, "update")
	f, _ := os.OpenFile(filepath.Join(root, app.HistoryFile), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("<<<<<<< HEAD\n{\"repo\":\"other/x\",\"time\":\"2020-01-01T00:00:00Z\"}\n")
	f.Close()
	s, _ := app.Open(root)
	recs, skipped, err := s.History("", 0)
	if err != nil || skipped != 1 || len(recs) != 2 {
		t.Fatalf("History: %v skipped %d %+v", err, skipped, recs)
	}
	if r := recs[0]; r.Repo != "acme/skills" || len(r.Commits) != app.MaxCommits || !r.MoreCommits {
		t.Fatalf("newest: %+v", recs[0])
	}
	if recs, _, _ := s.History("acme/skills", 1); len(recs) != 1 {
		t.Fatalf("-n 1: %+v", recs)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

// Upstream rewrote its history: the previous commit is not an ancestor of
// the new one, so the commits are unknown rather than wrong.
func TestHistoryAfterForcePush(t *testing.T) {
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	_, root := side(t, up)
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "a.md"), "a\n")
	commit(t, repo, "tdd: a")
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd")
	git(t, repo, "reset", "-q", "--hard", "HEAD~1")
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "b.md"), "b\n")
	commit(t, repo, "tdd: b instead")
	s, _ := app.Open(root)
	out, err := s.Update(nil, nil)
	if err != nil || len(out.History) != 1 {
		t.Fatalf("update: %v %+v", err, out)
	}
	if r := out.History[0]; !r.CommitsUnknown || len(r.Commits) != 0 {
		t.Fatalf("record: %+v", r)
	}
}
