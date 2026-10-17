package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/safetext"
)

// capture returns what f writes to stdout and stderr, as the commands
// write it: without the screen writer behind them, so what is checked is
// their own cleaning.
func capture(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	out, errw := stdout, stderr
	stdout, stderr = w, w
	defer func() { stdout, stderr = out, errw }()
	f()
	w.Close()
	b, _ := io.ReadAll(r)
	return string(b)
}

// Warnings and errors carry outside text (file names, git's output): it is
// cleaned on the way to the terminal, line breaks kept.
func TestMessagesAreCleaned(t *testing.T) {
	usePalette(darkPalette)
	msg := "git fetch: fatal:\nremote: \x1b]0;pwned\x07\x1b[2J\u202eevil"
	for _, l := range []app.Level{app.Problem, app.Done, app.Warn, app.Fail, app.Detail, app.Info} {
		out := capture(t, func() { logLine(l, msg) })
		// The styling skd adds is SGR only; anything else is the message's.
		plain := strings.ReplaceAll(out, "\n", "")
		for _, sgr := range []string{"\x1b[0m", "\x1b[1m"} {
			plain = strings.ReplaceAll(plain, sgr, "")
		}
		if strings.Contains(plain, "\x1b]") || strings.Contains(plain, "\x1b[2J") || strings.Contains(out, "\u202e") || strings.Contains(out, "\x07") {
			t.Errorf("level %v: %q", l, out)
		}
		if !strings.Contains(out, "remote: ") {
			t.Errorf("level %v lost the message: %q", l, out)
		}
	}
	if got := errText(errors.New(msg)); safetext.Has(strings.ReplaceAll(got, "\n", "")) || strings.Count(got, "\n") != 1 {
		t.Errorf("errText: %q", got)
	}
}
