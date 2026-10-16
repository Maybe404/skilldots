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
	p := filepath.Join(CachePath(src.Repo), ".git", name)
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
