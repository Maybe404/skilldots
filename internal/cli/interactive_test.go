package cli_test

import (
	"context"
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
	return e.script(dir, keys, out, sargs, args)
}

// interactiveShell is interactive for a shell command line, where "skd"
// is the binary under test: a terminal on stdin with skd's stdout piped,
// say.
func (e *env) interactiveShell(dir string, keys []typed, line string) string {
	e.t.Helper()
	if _, err := exec.LookPath("script"); err != nil || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		e.t.Skip("needs script(1)")
	}
	if dir == "" {
		dir = e.home
	}
	line = strings.ReplaceAll(line, "skd ", skdBin+" --theme dark ")
	out := filepath.Join(e.t.TempDir(), "typescript")
	sargs := []string{"-q", out, "sh", "-c", line}
	if runtime.GOOS == "linux" {
		sargs = []string{"-q", "-e", "-c", line, out}
	}
	return e.script(dir, keys, out, sargs, []string{line})
}

func (e *env) script(dir string, keys []typed, out string, sargs, args []string) string {
	e.t.Helper()
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

// Without a person to ask, update does what --yes does and asks nothing:
// stdin not a terminal (an agent), or stdout not one (skd update | cat,
// where a prompt would wait unseen).
func TestUpdateWithoutAPersonDoesNotAsk(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.skill("up/skills/eng/tdd", "tdd")
	up := e.repo("up")
	e.ok("", "add", up, "--skill", "tdd")
	change := func(f string) {
		e.write("up/skills/eng/tdd/"+f, f)
		e.git(up, "add", "-A")
		e.git(up, "commit", "-qm", f)
	}
	updated := func(f string) bool {
		_, err := os.Stat(filepath.Join(lib, "vendor", up, "eng", "tdd", f))
		return err == nil
	}
	// stdin not a terminal.
	change("a.md")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, skdBin, "update")
	cmd.Dir, cmd.Env = lib, e.vars
	b, err := cmd.CombinedOutput()
	out := string(b)
	if ctx.Err() != nil || err != nil {
		t.Fatalf("update without a terminal: %v (%v)\n%s", err, ctx.Err(), out)
	}
	if strings.Contains(out, "checking upstream") || !updated("a.md") {
		t.Fatalf("stdin not a terminal: asked or did not update:\n%s", out)
	}
	// stdout not a terminal: nothing is typed, and it still finishes.
	change("b.md")
	tr := e.interactiveShell(lib, nil, "skd update | cat")
	if strings.Contains(tr, "Update 1 pack?") || !updated("b.md") {
		t.Fatalf("stdout not a terminal: asked or did not update:\n%s", tr)
	}
	if r := e.js("", "history"); len(r["records"].([]any)) != 2 {
		t.Fatalf("history: %v", r)
	}
}

// update <pack> at a terminal when upstream has nothing new offers to
// update anyway, as u in the dashboard does: that brings back what is
// missing from vendor/. Answering no changes nothing.
func TestUpdateAnywayAtATerminal(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.skill("up/skills/eng/tdd", "tdd")
	up := e.repo("up")
	e.ok("", "add", up, "--skill", "tdd")
	gone := filepath.Join(lib, "vendor", up, "eng", "tdd")
	link := e.p(".agents/skills/tdd")
	isLink(t, link, lib)
	os.RemoveAll(gone)
	os.Remove(link)
	out := e.interactive(lib, []typed{{3 * time.Second, "n"}}, "update", up)
	if !strings.Contains(out, "Update anyway") || !strings.Contains(out, "nothing updated") {
		t.Fatalf("transcript:\n%s", out)
	}
	if _, err := os.Stat(gone); err == nil {
		t.Fatal("no still updated")
	}
	out = e.interactive(lib, []typed{{3 * time.Second, "y"}}, "update", up)
	if _, err := os.Stat(filepath.Join(gone, "SKILL.md")); err != nil {
		t.Fatalf("update anyway did not re-vendor:\n%s", out)
	}
	isLink(t, link, lib)
	// Without naming a pack there is nothing to pick: it says how to
	// re-link or re-vendor instead.
	out = e.interactive(lib, nil, "update")
	if !strings.Contains(out, "skd apply re-links") || strings.Contains(out, "Update anyway") {
		t.Fatalf("transcript:\n%s", out)
	}
}
