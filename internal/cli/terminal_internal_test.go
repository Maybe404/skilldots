package cli

import (
	"strings"
	"testing"
)

// skd asks a person only when both stdin and stdout are terminals: with
// stdout piped (skd update | cat) a prompt would be drawn where nobody
// sees it and wait forever, so confirm refuses (or --yes proceeds) and
// update does not ask.
func TestAskOnlyWithBothTerminals(t *testing.T) {
	in, out := stdinIsTerminal, stdoutIsTerminal
	t.Cleanup(func() { stdinIsTerminal, stdoutIsTerminal = in, out; flags.yes = false })
	for _, c := range []struct{ stdin, stdout, want bool }{
		{true, true, true}, {true, false, false}, {false, true, false}, {false, false, false},
	} {
		stdinIsTerminal = func() bool { return c.stdin }
		stdoutIsTerminal = func() bool { return c.stdout }
		if got := isTerminal(); got != c.want {
			t.Errorf("stdin %v stdout %v: isTerminal %v, want %v", c.stdin, c.stdout, got, c.want)
		}
	}
	// A terminal on stdin, stdout piped: no prompt is drawn.
	stdinIsTerminal = func() bool { return true }
	stdoutIsTerminal = func() bool { return false }
	flags.yes = false
	var yes bool
	got := capture(t, func() { yes = confirm("Go?") })
	if yes || !strings.Contains(got, "without a terminal") || strings.Contains(got, "Go?") {
		t.Errorf("confirm with stdout piped: %v %q", yes, got)
	}
	flags.yes = true
	if !confirm("Go?") {
		t.Error("--yes did not proceed")
	}
}
