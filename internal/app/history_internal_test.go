package app

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// A record another skd appends while an apply settles the history (reads
// it, takes applyError out, replaces it) is not lost with the old file:
// both hold the history lock, so the append waits for the replacement.
func TestSettleKeepsConcurrentAppend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(home, "cache"))
	s := &Session{Lib: &library.Library{Root: home}}
	failed := `{"time":"2026-01-01T00:00:00Z","repo":"acme/skills","from":"a","to":"b","applyError":"permission denied"}` + "\n"
	os.WriteFile(s.HistoryPath(), []byte(failed), 0o644)
	appended := make(chan error, 1)
	settleHook = func() {
		go func() {
			// Another session, as another process would have.
			o := &Session{Lib: &library.Library{Root: home}}
			appended <- o.appendHistory([]*UpdateRecord{{Repo: "other/pack", From: "c", To: "d", Commits: []UpstreamCommit{}}})
		}()
		select {
		case err := <-appended: // no lock: the append is already done
			appended <- err
		case <-time.After(300 * time.Millisecond): // it waits for the lock
		}
	}
	defer func() { settleHook = nil }()
	s.settleApplyErrors(nil)
	if err := <-appended; err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(s.HistoryPath())
	if got := string(b); !strings.Contains(got, "other/pack") || strings.Contains(got, "applyError") || !strings.Contains(got, "acme/skills") {
		t.Fatalf("history after a concurrent append:\n%s", got)
	}
}

// The same across processes: the lock is a file lock too.
func TestSettleKeepsAppendOfAnotherProcess(t *testing.T) {
	if root := os.Getenv("SKD_TEST_APPEND_ROOT"); root != "" {
		t.Skip("helper of the test below")
	}
	home := t.TempDir()
	cache := filepath.Join(home, "cache")
	t.Setenv("SKILLDOTS_CACHE", cache)
	s := &Session{Lib: &library.Library{Root: home}}
	failed := `{"time":"2026-01-01T00:00:00Z","repo":"acme/skills","from":"a","to":"b","applyError":"permission denied"}` + "\n"
	os.WriteFile(s.HistoryPath(), []byte(failed), 0o644)

	helper := exec.Command(os.Args[0], "-test.run=^TestAppendHelperProcess$", "-test.count=1")
	helper.Env = append(os.Environ(), "SKD_TEST_APPEND_ROOT="+home, "SKILLDOTS_CACHE="+cache)
	in, _ := helper.StdinPipe()
	var out strings.Builder
	helper.Stdout = &out
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- helper.Wait() }()
	settleHook = func() {
		in.Write([]byte("go\n"))
		select {
		case err := <-done: // no lock across processes: it appended already
			done <- err
		case <-time.After(500 * time.Millisecond):
		}
	}
	defer func() { settleHook = nil }()
	s.settleApplyErrors(nil)
	in.Close()
	if err := <-done; err != nil {
		t.Fatalf("helper: %v\n%s", err, out.String())
	}
	b, _ := os.ReadFile(s.HistoryPath())
	if got := string(b); !strings.Contains(got, "other/pack") || strings.Contains(got, "applyError") {
		t.Fatalf("history after another process appended:\n%s\nhelper: %s", got, out.String())
	}
}

// TestAppendHelperProcess appends one record to the history of
// $SKD_TEST_APPEND_ROOT once a line arrives on stdin.
func TestAppendHelperProcess(t *testing.T) {
	root := os.Getenv("SKD_TEST_APPEND_ROOT")
	if root == "" {
		t.Skip("run by TestSettleKeepsAppendOfAnotherProcess")
	}
	bufio.NewReader(os.Stdin).ReadString('\n')
	s := &Session{Lib: &library.Library{Root: root}}
	if err := s.appendHistory([]*UpdateRecord{{Repo: "other/pack", From: "c", To: "d", Commits: []UpstreamCommit{}}}); err != nil {
		t.Fatal(err)
	}
}
