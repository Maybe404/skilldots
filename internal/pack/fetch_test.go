package pack

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// fetchEnv makes an upstream repository with one commit and an empty
// cache, and returns the source and the upstream directory.
func fetchEnv(t *testing.T) (Source, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	d := t.TempDir()
	gc := filepath.Join(d, "gitconfig")
	os.WriteFile(gc, []byte("[user]\n\tname = t\n\temail = t@t\n[init]\n\tdefaultBranch = main\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", gc)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(d, "cache"))
	up := filepath.Join(d, "up")
	mk(t, up, "skills/a")
	gitT(t, up, "init", "-q")
	commit(t, up, "1")
	return Source{Repo: "acme/skills", URL: up}, up
}

func gitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commit(t *testing.T, up, msg string) {
	t.Helper()
	os.WriteFile(filepath.Join(up, "skills", "a", "more.md"), []byte(msg), 0o644)
	gitT(t, up, "add", "-A")
	gitT(t, up, "commit", "-qm", msg)
}

// fetchHeld fetches src under its Hold.
func fetchHeld(ctx context.Context, src Source) error {
	unlock, err := HoldContext(ctx, src.Repo)
	defer unlock()
	if err != nil {
		return err
	}
	_, _, err = FetchContext(ctx, src)
	return err
}

// lockFile writes a git lock file in the clone, age old.
func lockFile(t *testing.T, src Source, name string, age time.Duration) string {
	t.Helper()
	p := filepath.Join(CachePath(src.Repo), ".git", filepath.FromSlash(name))
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	then := time.Now().Add(-age)
	os.Chtimes(p, then, then)
	return p
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// A lock file a git outside skd is holding right now (here: just
// created) is never removed: Fetch waits for it and gives up with ctx.
func TestFetchLeavesLiveGitLock(t *testing.T) {
	src, up := fetchEnv(t)
	if err := fetchHeld(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	commit(t, up, "2")
	lock := lockFile(t, src, "index.lock", 0)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fetchHeld(ctx, src); err == nil {
		t.Fatal("fetch succeeded by taking a live git's index.lock")
	}
	if !exists(lock) {
		t.Fatal("a live git's index.lock was removed")
	}
}

// When the other git finishes (its lock file goes away), Fetch carries on.
func TestFetchWaitsForLiveGitLock(t *testing.T) {
	src, up := fetchEnv(t)
	if err := fetchHeld(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	commit(t, up, "2")
	lock := lockFile(t, src, "index.lock", 0)
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Remove(lock)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := fetchHeld(ctx, src); err != nil {
		t.Fatalf("fetch after the other git finished: %v", err)
	}
}

// Lock files left by a git killed long ago are cleared.
func TestFetchClearsStaleLock(t *testing.T) {
	src, up := fetchEnv(t)
	if err := fetchHeld(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	commit(t, up, "2")
	a := lockFile(t, src, "index.lock", 10*time.Minute)
	b := lockFile(t, src, "shallow.lock", 10*time.Minute)
	if err := fetchHeld(context.Background(), src); err != nil {
		t.Fatalf("fetch with stale locks: %v", err)
	}
	if exists(a) || exists(b) {
		t.Fatal("stale lock files still there")
	}
}

// Without the clone's file lock (here the lock file cannot be created)
// another skd may be using the clone: its git lock files stay, old or not.
func TestFetchWithoutFileLockLeavesLocks(t *testing.T) {
	src, up := fetchEnv(t)
	if err := fetchHeld(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	commit(t, up, "2")
	os.Remove(CachePath(src.Repo) + ".lock")
	if err := os.Mkdir(CachePath(src.Repo)+".lock", 0o755); err != nil {
		t.Fatal(err)
	}
	lock := lockFile(t, src, "index.lock", 10*time.Minute)
	if err := fetchHeld(context.Background(), src); err == nil {
		t.Fatal("fetch succeeded without the file lock by removing index.lock")
	}
	if !exists(lock) {
		t.Fatal("index.lock removed without holding the clone's file lock")
	}
}

// A failure that is not about a lock file leaves lock files alone.
func TestFetchOtherFailureLeavesLocks(t *testing.T) {
	src, up := fetchEnv(t)
	if err := fetchHeld(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(up)
	lock := lockFile(t, src, "index.lock", 10*time.Minute)
	if err := fetchHeld(context.Background(), src); err == nil {
		t.Fatal("fetch from a missing upstream succeeded")
	}
	if !exists(lock) {
		t.Fatal("index.lock removed after an unrelated failure")
	}
}

// A ref lock left by a git killed long ago (not one of the lock files
// fetch and reset take every time) is cleared too.
func TestFetchClearsStaleRefLock(t *testing.T) {
	src, up := fetchEnv(t)
	if err := fetchHeld(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	commit(t, up, "2")
	ref := lockFile(t, src, "refs/heads/main.lock", 0)
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	os.Chtimes(ref, old, old)
	orig := lockFile(t, src, "ORIG_HEAD.lock", 10*time.Minute)
	if err := fetchHeld(context.Background(), src); err != nil {
		t.Fatalf("fetch with stale ref locks: %v", err)
	}
	if exists(ref) || exists(orig) {
		t.Fatal("stale ref lock files still there")
	}
}

// git leaves a live ORIG_HEAD.lock alone (reset only warns) and so does
// Fetch.
func TestFetchLeavesLiveOrigHeadLock(t *testing.T) {
	src, up := fetchEnv(t)
	if err := fetchHeld(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	commit(t, up, "2")
	orig := lockFile(t, src, "ORIG_HEAD.lock", 0)
	if err := fetchHeld(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	if !exists(orig) {
		t.Fatal("a live ORIG_HEAD.lock was removed")
	}
}

// The lock files are read from git's message, in any language, exactly as
// named: ORIG_HEAD.lock is not HEAD.lock, and a path outside the clone's
// .git (the remote's repository) is not the clone's.
func TestLockPaths(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755)
	other := t.TempDir()
	os.MkdirAll(filepath.Join(other, ".git"), 0o755)
	abs := filepath.ToSlash(gitDir)
	for _, c := range []struct {
		msg  string
		want []string
	}{
		{"error: update_ref failed for ref 'ORIG_HEAD': cannot lock ref 'ORIG_HEAD': Unable to create '" + abs + "/ORIG_HEAD.lock': File exists.",
			[]string{"ORIG_HEAD.lock"}},
		{"错误：对引用 'HEAD' 执行 update_ref 失败：cannot lock ref 'HEAD': 无法创建 '" + abs + "/refs/heads/main.lock'：File exists。",
			[]string{filepath.Join("refs", "heads", "main.lock")}},
		{"致命错误：无法创建 '" + abs + "/index.lock'：文件已存在。\nfatal: Unable to create '" + abs + "/shallow.lock': File exists.",
			[]string{"index.lock", "shallow.lock"}},
		{"fatal: Unable to create '.git/index.lock': File exists.", []string{"index.lock"}},
		{"fatal: Unable to create '" + filepath.ToSlash(other) + "/.git/index.lock': File exists.", nil},
		{"fatal: Unable to create '" + abs + "/../x.lock': File exists.", nil},
		{"fatal: unable to access 'https://example.com/repo.git/': timeout", nil},
	} {
		var got []string
		for _, p := range lockPaths(dir, c.msg) {
			rel, _ := filepath.Rel(gitDir, p)
			got = append(got, rel)
		}
		if len(got) != len(c.want) {
			t.Errorf("%q: got %v, want %v", c.msg, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q: got %v, want %v", c.msg, got, c.want)
			}
		}
	}
}
