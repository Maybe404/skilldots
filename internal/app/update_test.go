package app_test

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Maybe404/skilldots/internal/cli"
	"github.com/Maybe404/skilldots/internal/tui"
)

// cliUpdateNew runs `skd update --json` in process and returns each
// pack's "upstream new" names.
func cliUpdateNew(t *testing.T, root string) []string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	cmd := cli.Root()
	cmd.SetArgs([]string{"--library", root, "--json", "update"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	runErr := cmd.Execute()
	w.Close()
	os.Stdout = stdout
	b, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatalf("skd update: %v\n%s", runErr, b)
	}
	var out struct {
		Packs []struct{ UpstreamNew []string }
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("update --json: %v\n%s", err, b)
	}
	var names []string
	for _, p := range out.Packs {
		names = append(names, p.UpstreamNew...)
	}
	return names
}

func tuiUpdateNew(t *testing.T, root string) []string {
	t.Helper()
	reps, _, err := tui.NewOps(root).Update(nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range reps {
		names = append(names, r.UpstreamNew...)
	}
	return names
}

// "upstream new" means skills that appeared upstream since the last
// update, the same in `skd update` and the dashboard's u; the skills the
// user simply has not selected are not new every time.
func TestUpdateUpstreamNewAgrees(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	cliHome, cliRoot := side(t, up)
	runCLI(t, cliRoot, "add", "acme/skills", "--skill", "tdd")
	tuiHome, tuiRoot := side(t, up)
	if _, err := tui.NewOps(tuiRoot).Add("acme/skills", tui.AddRequest{Names: []string{"tdd"}}); err != nil {
		t.Fatal(err)
	}
	type sideT struct {
		name, home, root string
		update           func(*testing.T, string) []string
	}
	sides := []sideT{{"cli", cliHome, cliRoot, cliUpdateNew}, {"dashboard", tuiHome, tuiRoot, tuiUpdateNew}}
	check := func(step string, want []string) {
		t.Helper()
		for _, s := range sides {
			// Each side has its own HOME, cache and git config.
			t.Setenv("HOME", s.home)
			t.Setenv("SKILLDOTS_CACHE", filepath.Join(s.home, "cache"))
			t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(s.home, "gitconfig"))
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(s.home, ".config"))
			if got := s.update(t, s.root); !slices.Equal(got, want) {
				t.Errorf("%s: %s: upstream new = %v, want %v", step, s.name, got, want)
			}
		}
	}
	check("first update", nil)
	write(t, filepath.Join(repo, "skills", "eng", "fresh", "SKILL.md"), skillMD("fresh"))
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "fresh")
	check("after upstream added fresh", []string{"fresh"})
	check("updating again", nil)
	// The baseline is the library's record, not the cache, which can be
	// deleted at any time.
	for _, s := range sides {
		os.RemoveAll(filepath.Join(s.home, "cache"))
	}
	write(t, filepath.Join(repo, "skills", "eng", "fresh2", "SKILL.md"), skillMD("fresh2"))
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "fresh2")
	check("without a cache", []string{"fresh2"})
	check("without a cache, again", nil)
}
