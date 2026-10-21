package app_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
)

// The diff before an update reads the files of the skills upstream
// changed only: the others are hashed as they stream.
func TestUpdateDiffReadsChangedSkillsOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	_, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd", "--skill", "review", "--skill", "grill")
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), skillMD("tdd")+"more\n")
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "tdd")

	var read []string
	app.SetFilesRead(func(dir string) { read = append(read, dir) })
	t.Cleanup(func() { app.SetFilesRead(nil) })
	ses, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	d, err := ses.UpdateDiff(context.Background(), "acme/skills", "")
	if err != nil || len(d.Skills) != 1 || d.Skills[0].Name != "tdd" {
		t.Fatalf("%+v %v", d, err)
	}
	for _, dir := range read {
		if base := filepath.Base(dir); base != "tdd" {
			t.Errorf("read %s, which upstream did not change", dir)
		}
	}
	if len(read) == 0 || !strings.HasSuffix(read[0], "tdd") {
		t.Fatalf("read %v", read)
	}
}
