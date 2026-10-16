//go:build unix

package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Quitting during a slow check stops its git and everything git started,
// promptly, and the cache clone stays usable.
func TestCheckCancelLeavesNoGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home, up := sandbox(t)
	repo := filepath.Join(up, "acme", "skills.git")
	write(t, filepath.Join(repo, "skills", "tdd", "SKILL.md"), skillMD("tdd", "Test first."))
	run(t, repo, "init", "-q")
	run(t, repo, "add", "-A")
	run(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init")
	root := newLibrary(t, home)
	o := NewOps(root)
	if _, err := o.Add("acme/skills", AddRequest{Names: []string{"tdd"}}); err != nil {
		t.Fatal(err)
	}
	// upload-pack that takes 20 seconds to start.
	slow := filepath.Join(up, "slowup")
	write(t, slow, "#!/bin/sh\nsleep 20\nexec git-upload-pack \"$@\"\n")
	os.Chmod(slow, 0o755)
	gc := os.Getenv("GIT_CONFIG_GLOBAL")
	b, _ := os.ReadFile(gc)
	write(t, gc, string(b)+"[remote \"origin\"]\n\tuploadpack = "+slow+"\n")

	c := &checkControl{}
	ctx := c.begin()
	errc := make(chan error, 1)
	go func() {
		defer c.wg.Done()
		_, err := o.Check(ctx)
		errc <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		tbl, err := processTable()
		if err != nil {
			t.Skip("no ps:", err)
		}
		if len(descendants(tbl, os.Getpid())) >= 2 {
			break // git fetch and upload-pack are running
		}
		if time.Now().After(deadline) {
			t.Fatal("fetch never started")
		}
		time.Sleep(50 * time.Millisecond)
	}
	start := time.Now()
	if !c.stop(2 * time.Second) {
		t.Fatalf("check still running %v after quit", time.Since(start))
	}
	if err := <-errc; err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("cancelled check: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	tbl, _ := processTable()
	if d := descendants(tbl, os.Getpid()); len(d) > 0 {
		t.Fatalf("processes left after quitting: %v", d)
	}
	// Orphans are no longer our descendants: look for anything still
	// running from this test's upstream or its slow upload-pack.
	if left := running(t, up); left != "" {
		t.Fatalf("processes left after quitting:\n%s", left)
	}
	// Nothing stale left behind: the next check works.
	write(t, gc, string(b))
	if out, err := o.Check(context.Background()); err != nil || len(out.Failed) > 0 {
		t.Fatalf("check after a cancelled one: %+v, %v", out, err)
	}
}

// running lists processes whose command line mentions path.
func running(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command("ps", "-A", "-o", "pid=,command=").Output()
	if err != nil {
		t.Skip("no ps:", err)
	}
	var hits []string
	for _, l := range strings.Split(string(out), "\n") {
		if strings.Contains(l, path) && !strings.Contains(l, " ps ") {
			hits = append(hits, strings.TrimSpace(l))
		}
	}
	return strings.Join(hits, "\n")
}
