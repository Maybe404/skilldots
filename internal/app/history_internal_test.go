package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/manifest"
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

// A record of a huge update (thousands of long skill names) is cut to fit
// one line the reader takes: it reads back, saying how much was left out.
func TestHugeRecordReadsBack(t *testing.T) {
	root := t.TempDir()
	s := &Session{Lib: &library.Library{Root: root, Manifest: manifest.New(nil)}}
	name := strings.Repeat("n", MaxName)
	r := &UpdateRecord{Repo: "acme/skills", From: "a", To: "b", Commits: []UpstreamCommit{{Hash: "c", Subject: "s"}}}
	for i := 0; i < 8000; i++ {
		r.Changed = append(r.Changed, fmt.Sprintf("%s%d", name, i))
	}
	r.New = append([]string{}, r.Changed[:3000]...)
	r.sanitize()
	if err := s.appendHistory([]*UpdateRecord{r}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(s.HistoryPath())
	if len(b) > maxRecord+1 {
		t.Errorf("wrote a %d-byte line", len(b))
	}
	recs, skipped, err := s.History("", 0)
	if err != nil || skipped != 0 || len(recs) != 1 {
		t.Fatalf("History: %v skipped %d, %d records", err, skipped, len(recs))
	}
	got := recs[0]
	if got.Omitted == 0 || len(got.Changed)+len(got.New)+got.Omitted != 11000 || len(got.Commits) != 1 {
		t.Errorf("kept %d changed, %d new, omitted %d, %d commits", len(got.Changed), len(got.New), got.Omitted, len(got.Commits))
	}
	if rows := RecordRows(got); !strings.Contains(rows[len(rows)-1][1], fmt.Sprint(got.Omitted)) {
		t.Errorf("rows do not say what was left out: %v", rows[len(rows)-1])
	}
	// A record of ordinary size is written whole.
	small := &UpdateRecord{Repo: "x/y", From: "a", To: "b", Changed: []string{"tdd"}}
	if line, _ := small.marshalFit(); small.Omitted != 0 || !strings.Contains(string(line), "tdd") {
		t.Errorf("small record cut: %s", line)
	}
}
