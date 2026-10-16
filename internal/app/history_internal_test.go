package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/pack"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// The ref fillCommits deepens the shallow cache clone with comes from
// skills.toml: one written as an option (--upload-pack=…) is a refspec to
// git, never an option that runs a command.
func TestFillCommitsRefIsNeverAnOption(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(home, "cache"))
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	up := filepath.Join(home, "up")
	os.MkdirAll(filepath.Join(up, "skills", "tdd"), 0o755)
	git := []string{"git", "-c", "user.name=t", "-c", "user.email=t@t"}
	run(t, home, "git", "init", "-q", up)
	var from string
	for i, f := range []string{"a", "b", "c"} {
		os.WriteFile(filepath.Join(up, "skills", "tdd", f), []byte(f), 0o644)
		run(t, up, "git", "add", "-A")
		run(t, up, append(git, "commit", "-qm", "tdd "+f)...)
		if i == 0 {
			from = run(t, up, "git", "rev-parse", "HEAD")
		}
	}
	clone := pack.CachePath("acme/skills")
	os.MkdirAll(filepath.Dir(clone), 0o755)
	run(t, home, "git", "clone", "-q", "--depth", "1", "file://"+up, clone)
	to := run(t, clone, "git", "rev-parse", "HEAD")

	marker := filepath.Join(home, "ran")
	r := &UpdateRecord{Repo: "acme/skills", From: from, To: to, Changed: []string{"tdd"}, Commits: []UpstreamCommit{}}
	fillCommits(context.Background(), r, "--upload-pack=touch "+marker+";", nil)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the ref was taken as an option: git ran the command in it")
	}
	if !r.CommitsUnknown {
		t.Errorf("a ref git cannot fetch should leave the commits unknown: %+v", r)
	}
	// The same clone with an ordinary ref deepens and finds the commits.
	r = &UpdateRecord{Repo: "acme/skills", From: from, To: to, Changed: []string{"tdd"}, Commits: []UpstreamCommit{}}
	fillCommits(context.Background(), r, "", nil)
	if r.CommitsUnknown || len(r.Commits) != 2 {
		t.Errorf("ordinary ref: %+v", r)
	}
}
