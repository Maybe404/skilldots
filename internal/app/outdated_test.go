package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Maybe404/skilldots/internal/app"
)

// The recorded check is what list, the menu and the dashboard read: it
// follows the lock without fetching, and goes stale after a day.
func TestRecordedOutdated(t *testing.T) {
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	_, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd")
	runCLI(t, root, "update")

	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if s.RecordedOutdated() != nil {
		t.Fatal("a record before any check")
	}
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "more.md"), "more\n")
	write(t, filepath.Join(repo, "skills", "eng", "fresh", "SKILL.md"), skillMD("fresh"))
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "c2")

	out, err := s.Outdated(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Updates() != 1 || !slices.Equal(out.Packs[0].Changed, []string{"tdd"}) || !slices.Equal(out.Packs[0].New, []string{"fresh"}) {
		t.Fatalf("outdated: %+v", out)
	}
	if _, err := os.Stat(app.OutdatedPath()); err != nil {
		t.Fatal(err)
	}

	s, _ = app.Open(root)
	rec := s.RecordedOutdated()
	if rec == nil || rec.Updates() != 1 || rec.Stale(time.Now()) || !rec.Stale(time.Now().Add(25*time.Hour)) {
		t.Fatalf("recorded: %+v", rec)
	}

	// Adding the new skill outside update: it is no longer offered.
	runCLI(t, root, "add", "acme/skills", "--skill", "fresh")
	s, _ = app.Open(root)
	if rec := s.RecordedOutdated(); rec != nil {
		// add vendored the new commit: the record is out of date, so
		// nothing is known until the next check.
		t.Fatalf("after add moved the pack on: %+v", rec)
	}

	// update refreshes the record of the packs it updated.
	if _, err := s.Outdated(nil, nil); err != nil {
		t.Fatal(err)
	}
	runCLI(t, root, "update")
	s, _ = app.Open(root)
	if rec := s.RecordedOutdated(); rec == nil || rec.Updates() != 0 || len(rec.Packs) != 1 {
		t.Fatalf("after update: %+v", rec)
	}
}

// A pack just added has a baseline for "new" and "gone" too: a check
// (which replaces the cache clone) between add and the first update
// must not hide what appeared upstream.
func TestNewAfterAddSurvivesCheck(t *testing.T) {
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	_, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd,grill")
	write(t, filepath.Join(repo, "skills", "eng", "a3", "SKILL.md"), skillMD("a3"))
	os.RemoveAll(filepath.Join(repo, "skills", "prod", "grill"))
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "a3")

	s, _ := app.Open(root)
	out, err := s.Outdated(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st := out.Packs[0]; !slices.Equal(st.New, []string{"a3"}) || !slices.Equal(st.Gone, []string{"grill"}) {
		t.Fatalf("outdated right after add: %+v", st)
	}
	if got := cliUpdateNew(t, root); !slices.Equal(got, []string{"a3"}) {
		t.Fatalf("first update after add and a check: upstream new = %v, want [a3]", got)
	}
}
