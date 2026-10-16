package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestNoQueryBeforeDetect runs skd on a pseudo-terminal nobody answers
// (script(1) with no terminal behind it). Before, Bubble Tea's package
// init made termenv ask for the background colour and wait five seconds
// on every command. Now only skd's own bounded query is sent, and only
// for --theme auto.
func TestNoQueryBeforeDetect(t *testing.T) {
	if _, err := exec.LookPath("script"); err != nil || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		t.Skip("needs script(1)")
	}
	e := newEnv(t)
	e.ok("", "init", "--no-skill", e.p("lib"))
	var vars []string
	for _, v := range e.vars {
		if !strings.HasPrefix(v, "NO_COLOR=") && !strings.HasPrefix(v, "TERM=") {
			vars = append(vars, v)
		}
	}
	vars = append(vars, "TERM=xterm-256color")
	for _, c := range []struct {
		theme       string
		ownQuery    bool
		maxDuration time.Duration
	}{{"auto", true, 3 * time.Second}, {"dark", false, 3 * time.Second}} {
		out := filepath.Join(t.TempDir(), "typescript")
		args := []string{"-q", out, skdBin, "list", "--theme", c.theme}
		if runtime.GOOS == "linux" {
			args = []string{"-q", "-e", "-c", skdBin + " list --theme " + c.theme, out}
		}
		cmd := exec.Command("script", args...)
		cmd.Env, cmd.Dir = vars, e.p("lib")
		start := time.Now()
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("script: %v\n%s", err, b)
		}
		took := time.Since(start)
		b, _ := os.ReadFile(out)
		s := string(b)
		if !strings.Contains(s, "skilldots") {
			t.Fatalf("%s: no output:\n%q", c.theme, s)
		}
		if strings.Contains(s, "\x1b[6n") {
			t.Errorf("%s: termenv queried the terminal (OSC 11 + CSI 6n)", c.theme)
		}
		if got := strings.Contains(s, "\x1b]11;?"); got != c.ownQuery {
			t.Errorf("%s: background query sent = %v", c.theme, got)
		}
		if took > c.maxDuration {
			t.Errorf("%s: took %v", c.theme, took)
		}
	}
}
