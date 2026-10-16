//go:build !rawoutput

package cli

import (
	"bytes"
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The build people run puts the last line of defence in front of the
// terminal: everything printed for a person goes through a screenWriter
// on os.Stdout or os.Stderr. (A rawoutput build leaves it out on purpose,
// so the hostile gate sees what the commands print; this test is not in
// that build.)
func TestOutputIsGuarded(t *testing.T) {
	for name, c := range map[string]struct {
		w    any
		file *os.File
	}{"stdout": {stdout, os.Stdout}, "stderr": {stderr, os.Stderr}} {
		sw, ok := c.w.(*screenWriter)
		if !ok {
			t.Errorf("%s is %T, not a screenWriter", name, c.w)
			continue
		}
		if sw.w != c.file {
			t.Errorf("%s writes to %v, not the terminal", name, sw.w)
		}
	}
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)
	lipgloss.SetColorProfile(termenv.Ascii)
	var b bytes.Buffer
	guard(&b).Write([]byte("a\x1b]0;t\x07‮b\n"))
	if b.String() != "a]0;tb\n" {
		t.Errorf("guard: %q", b.String())
	}
}
