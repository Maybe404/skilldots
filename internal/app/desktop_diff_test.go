package app_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/lock"
)

// The diff shown before an update shows, for a skill the user changed,
// what upstream changed since the version those changes were made on
// (what the update merges in), marks the files the user changed and the
// ones expected to conflict, and changes nothing.
func TestUpdateDiffMarksLocalEdits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	_, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd", "--skill", "review")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "vendor")

	write(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), skillMD("tdd")+"Write the test first.\n")
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "notes.md"), "notes\n")
	write(t, filepath.Join(repo, "skills", "prod", "ship", "SKILL.md"), skillMD("ship"))
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "tdd: more")

	matches, _ := filepath.Glob(filepath.Join(root, "vendor", "acme", "skills", "*", "tdd"))
	if len(matches) != 1 {
		t.Fatalf("vendored tdd: %v", matches)
	}
	dir := matches[0]
	write(t, filepath.Join(dir, "SKILL.md"), skillMD("tdd")+"My own rule.\n")
	write(t, filepath.Join(dir, "extra.md"), "mine\n")
	lockBefore, _ := os.ReadFile(filepath.Join(root, lock.FileName))

	ses, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	d, err := ses.UpdateDiff(context.Background(), "acme/skills", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Skills) != 1 || d.Skills[0].Name != "tdd" || d.Skills[0].Status != "changed" {
		t.Fatalf("skills: %+v", d.Skills)
	}
	sd := d.Skills[0]
	if !sd.Local || !sd.Base || sd.LocalUnknown || !slices.Equal(sd.LocalFiles, []string{"SKILL.md", "extra.md"}) {
		t.Fatalf("local edits: %+v", sd)
	}
	// Both appended a line at the end of SKILL.md: that conflicts.
	if !slices.Equal(sd.Conflicts, []string{"SKILL.md"}) {
		t.Fatalf("conflicts: %+v", sd.Conflicts)
	}
	byPath := map[string]app.FileDiff{}
	for _, f := range sd.Files {
		byPath[f.Path] = f
	}
	if f := byPath["notes.md"]; f.Status != "added" || f.Local || f.Added != 1 {
		t.Fatalf("notes.md: %+v", f)
	}
	// The user's own file is not upstream's change: not shown as removed.
	if f, ok := byPath["extra.md"]; ok {
		t.Fatalf("extra.md: %+v", f)
	}
	f := byPath["SKILL.md"]
	if f.Status != "modified" || !f.Local || !f.Conflict || f.Added != 1 || f.Removed != 0 || len(f.Hunks) != 1 {
		t.Fatalf("SKILL.md: %+v", f)
	}
	var lines []string
	for _, l := range f.Hunks[0].Lines {
		lines = append(lines, l.Op+l.Text)
	}
	if got := strings.Join(lines, "\n"); strings.Contains(got, "My own rule.") || !strings.Contains(got, "+Write the test first.") {
		t.Fatalf("hunk:\n%s", got)
	}
	if lockAfter, _ := os.ReadFile(filepath.Join(root, lock.FileName)); string(lockAfter) != string(lockBefore) {
		t.Fatal("UpdateDiff changed the lock")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "SKILL.md")); !strings.Contains(string(b), "My own rule.") {
		t.Fatal("UpdateDiff changed the vendored copy")
	}

	// Seen at the commit just fetched, the cache is used as it is.
	again, err := ses.UpdateDiff(context.Background(), "acme/skills", d.Latest)
	if err != nil || again.Latest != d.Latest || len(again.Skills) != 1 {
		t.Fatalf("cached diff: %+v %v", again, err)
	}
	if _, err := ses.UpdateDiff(context.Background(), "nobody/nothing", ""); err == nil {
		t.Fatal("diff of a pack not subscribed")
	}
}

// Without the version the edits were made on (no upstream snapshot, and
// never committed) the user's copy is compared with upstream, and the
// skill is still marked as edited.
func TestUpdateDiffUncommittedVendor(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	_, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd")
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), skillMD("tdd")+"more\n")
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "more")
	matches, _ := filepath.Glob(filepath.Join(root, "vendor", "acme", "skills", "*", "tdd"))
	write(t, filepath.Join(matches[0], "SKILL.md"), "changed by hand\n")
	git(t, root, "update-ref", "-d", "refs/skilldots/upstream/acme/skills")
	ses, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	d, err := ses.UpdateDiff(context.Background(), "acme/skills", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Skills) != 1 || !d.Skills[0].Local || d.Skills[0].Base || !d.Skills[0].LocalUnknown || len(d.Skills[0].LocalFiles) != 0 {
		t.Fatalf("%+v", d.Skills)
	}
}
