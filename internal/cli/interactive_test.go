package cli_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// typed is input for an interactive run: after waiting, keys are typed.
type typed struct {
	wait time.Duration
	keys string
}

// interactive runs skd on a pseudo-terminal (script(1)) and types keys.
// It returns the transcript; the run is killed after 30 seconds.
func (e *env) interactive(dir string, keys []typed, args ...string) string {
	e.t.Helper()
	if _, err := exec.LookPath("script"); err != nil || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		e.t.Skip("needs script(1)")
	}
	if dir == "" {
		dir = e.home
	}
	out := filepath.Join(e.t.TempDir(), "typescript")
	sargs := append([]string{"-q", out, skdBin, "--theme", "dark"}, args...)
	if runtime.GOOS == "linux" {
		sargs = []string{"-q", "-e", "-c", strings.Join(append([]string{skdBin, "--theme", "dark"}, args...), " "), out}
	}
	cmd := exec.Command("script", sargs...)
	var vars []string
	for _, v := range e.vars {
		if !strings.HasPrefix(v, "TERM=") {
			vars = append(vars, v)
		}
	}
	cmd.Env, cmd.Dir = append(vars, "TERM=xterm"), dir
	in, err := cmd.StdinPipe()
	if err != nil {
		e.t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		e.t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	go func() {
		for _, k := range keys {
			time.Sleep(k.wait)
			io.WriteString(in, k.keys)
		}
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		<-done
		b, _ := os.ReadFile(out)
		e.t.Fatalf("skd %v did not finish:\n%s", args, b)
	}
	in.Close()
	b, _ := os.ReadFile(out)
	return string(b)
}

// At a terminal, update shows what would change, every pack ticked, and
// updates only the ones left ticked.
func TestUpdateAsksAtATerminal(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.skill("up/skills/eng/tdd", "tdd")
	e.skill("up2/skills/lint", "lint")
	up, up2 := e.repo("up"), e.repo("up2")
	e.ok("", "add", up, "--skill", "tdd")
	e.ok("", "add", up2, "--skill", "lint")
	e.write("up/skills/eng/tdd/x.md", "x\n")
	e.write("up2/skills/lint/y.md", "y\n")
	e.git(up, "add", "-A")
	e.git(up, "commit", "-qm", "tdd x")
	e.git(up2, "add", "-A")
	e.git(up2, "commit", "-qm", "lint y")
	lockBefore, _ := os.ReadFile(e.p("lib/skills.lock"))

	// ctrl+c: nothing changes.
	out := e.interactive(lib, []typed{{3 * time.Second, "\x03"}}, "update")
	if !strings.Contains(out, "changed") || !strings.Contains(out, "nothing updated") {
		t.Fatalf("transcript:\n%s", out)
	}
	if b, _ := os.ReadFile(e.p("lib/skills.lock")); string(b) != string(lockBefore) {
		t.Fatal("ctrl+c still updated")
	}
	// Untick the first pack, enter: only the second is updated.
	e.interactive(lib, []typed{{3 * time.Second, " "}, {500 * time.Millisecond, "\r"}}, "update")
	if _, err := os.Stat(filepath.Join(lib, "vendor", up2, "lint", "y.md")); err != nil {
		t.Fatalf("ticked pack not updated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(lib, "vendor", up, "eng", "tdd", "x.md")); err == nil {
		t.Fatal("unticked pack updated")
	}
	if r := e.js("", "history"); len(r["records"].([]any)) != 1 {
		t.Fatalf("history: %v", r)
	}
	// --yes keeps the old behaviour: no question, everything.
	e.ok("", "update", "--yes")
	if _, err := os.Stat(filepath.Join(lib, "vendor", up, "eng", "tdd", "x.md")); err != nil {
		t.Fatal("--yes did not update")
	}
}
