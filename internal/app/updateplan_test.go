package app_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
)

// The plan lists what update would change and changes nothing; updating
// only the chosen packs leaves the others' vendor, lock and links alone.
func TestPlanUpdateThenUpdateChosen(t *testing.T) {
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	other := filepath.Join(up, "beta", "tools.git")
	write(t, filepath.Join(other, "skills", "lint", "SKILL.md"), skillMD("lint"))
	git(t, other, "init", "-q")
	commit(t, other, "init")
	home, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd")
	runCLI(t, root, "add", "beta/tools", "--skill", "lint")

	write(t, filepath.Join(repo, "skills", "eng", "tdd", "x.md"), "x\n")
	commit(t, repo, "tdd x")
	write(t, filepath.Join(other, "skills", "lint", "y.md"), "y\n")
	commit(t, other, "lint y")
	// Only a move: not offered unless named.
	before := state(t, home, root)

	s, _ := app.Open(root)
	plan, err := s.PlanUpdate(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Repos(); !slices.Equal(got, []string{"acme/skills", "beta/tools"}) {
		t.Fatalf("plan: %v", got)
	}
	if st := plan.Find("acme/skills"); st.Summary() != "1 changed" || len(st.Rows()) != 1 || st.Rows()[0] != [2]string{"changed", "tdd"} {
		t.Fatalf("acme: %q %v", st.Summary(), st.Rows())
	}
	if after := state(t, home, root); !equalState(before, after) {
		t.Fatal("planning changed the library")
	}

	s, _ = app.Open(root)
	if _, err := s.Update([]string{"beta/tools"}, nil); err != nil {
		t.Fatal(err)
	}
	after := state(t, home, root)
	acmeLock := func(st map[string]string) string { return st["skills.lock"] }
	if acmeLock(after) == acmeLock(before) {
		t.Fatal("beta/tools was not updated")
	}
	// acme/skills is exactly as it was: its vendored tdd has no x.md.
	if _, err := os.Stat(filepath.Join(root, "vendor", "acme", "skills", "eng", "tdd", "x.md")); err == nil {
		t.Fatal("acme/skills was updated too")
	}
	s, _ = app.Open(root)
	recs, _, _ := s.History("", 0)
	if len(recs) != 1 || recs[0].Repo != "beta/tools" {
		t.Fatalf("history: %+v", recs)
	}

	// A named pack with only a new commit is still offered.
	write(t, filepath.Join(repo, "README.md"), "r\n")
	commit(t, repo, "readme")
	s, _ = app.Open(root)
	plan, _ = s.PlanUpdate(context.Background(), []string{"beta/tools"}, nil)
	if len(plan.Updates) != 0 {
		t.Fatalf("beta/tools has nothing new: %+v", plan.Updates)
	}
	if err := os.WriteFile(filepath.Join(other, "README.md"), []byte("r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit(t, other, "readme")
	s, _ = app.Open(root)
	plan, _ = s.PlanUpdate(context.Background(), []string{"beta/tools"}, nil)
	if len(plan.Updates) != 1 || plan.Updates[0].Summary() != "new commit, no skill changes" {
		t.Fatalf("named pack with a new commit: %+v", plan.Updates)
	}
	s, _ = app.Open(root)
	if plan, _ = s.PlanUpdate(context.Background(), nil, nil); slices.Contains(plan.Repos(), "beta/tools") {
		t.Fatalf("unnamed: a commit alone is offered: %v", plan.Repos())
	}
}

func equalState(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
