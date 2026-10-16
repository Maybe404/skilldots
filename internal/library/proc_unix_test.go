//go:build unix

package library

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

// TestSessionProbe is not a test: run by git below, it reports whether it
// is in a session of its own and can reach a terminal.
func TestSessionProbe(t *testing.T) {
	if os.Getenv("SKD_SESSION_PROBE") == "" {
		t.Skip("helper")
	}
	sid, _ := syscall.Getsid(0)
	tty := "none"
	if f, err := os.Open("/dev/tty"); err == nil {
		f.Close()
		tty = "open"
	}
	fmt.Printf("own-session=%v tty=%s\n", sid == syscall.Getpgrp(), tty)
	os.Exit(0)
}

// A cancellable git runs in a session of its own, without a controlling
// terminal: a prompt that reads /dev/tty (an ssh passphrase through a
// custom core.sshCommand) fails at once instead of stopping git in the
// background (SIGTTIN) where nobody sees it.
func TestCancellableGitHasNoTerminal(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("SKD_SESSION_PROBE", "1")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	probe := "!" + os.Args[0] + " -test.run=^TestSessionProbe$"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out, err := GitContext(ctx, t.TempDir(), "-c", "alias.probe="+probe, "probe")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "own-session=true tty=none") {
		t.Fatalf("git's child: %q", out)
	}
}
