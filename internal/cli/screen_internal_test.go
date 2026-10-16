package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/safetext"
	"github.com/Maybe404/skilldots/internal/targets"
)

// The last line of defence: whatever reaches it still holding a control
// character, a bidi control or an escape sequence other than a colour
// code is cleaned; colour codes stay only while colour is on; a rune
// split between two writes comes through whole.
func TestScreenWriter(t *testing.T) {
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)
	for _, c := range []struct {
		profile termenv.Profile
		want    string
	}{
		{termenv.TrueColor, "\x1b[1mok\x1b[0m ]0;t[2J 中文\n"},
		{termenv.Ascii, "[1mok[0m ]0;t[2J 中文\n"},
	} {
		lipgloss.SetColorProfile(c.profile)
		var b bytes.Buffer
		w := &screenWriter{w: &b}
		in := []byte("\x1b[1mok\x1b[0m \x1b]0;t\x07\x1b[2J\u202e \u009b中文\n")
		cut := bytes.Index(in, []byte("中")) + 1 // inside the rune
		if n, err := w.Write(in[:cut]); n != cut || err != nil {
			t.Fatalf("Write = %d, %v", n, err)
		}
		w.Write(in[cut:])
		if b.String() != c.want {
			t.Errorf("profile %v: %q, want %q", c.profile, b.String(), c.want)
		}
	}
}

// What an apply prints is cleaned too, though the lock and the manifest
// it comes from are checked when read (names, targets, reasons and the
// paths moved aside).
func TestPrintApplyCleans(t *testing.T) {
	usePalette(darkPalette)
	const evil = "\x1b]0;t\x07\u009b\u202e"
	out := &app.ApplyOutcome{Applied: true, Backups: []string{"/a/b" + evil + " → .trash/x" + evil}, Plan: engine.Plan{Ops: []engine.Op{
		{Action: "skip", Target: targets.Target{Name: "t" + evil}, Name: "n" + evil, Reason: "r" + evil},
		{Action: "create", Target: targets.Target{Name: "agents"}, Name: "ok"},
	}}}
	got := capture(t, func() { printApply(nil, out) })
	if strings.ContainsFunc(strings.ReplaceAll(got, "\n", ""), func(r rune) bool { return r != '\x1b' && safetext.Unsafe(r) }) || strings.Contains(got, "\x1b]") {
		t.Errorf("printApply: %q", got)
	}
	for _, want := range []string{"t]0;t", "n]0;t", "r]0;t", "/a/b]0;t"} {
		if !strings.Contains(got, want) {
			t.Errorf("printApply lost %q: %q", want, got)
		}
	}
}

// A colour sequence that hides text is dropped, also when the text and
// the background colours come in two writes.
func TestScreenWriterDropsHidingColours(t *testing.T) {
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)
	lipgloss.SetColorProfile(termenv.TrueColor)
	var b bytes.Buffer
	w := &screenWriter{w: &b}
	for _, s := range []string{"\x1b[8ma", "\x1b[31mb", "\x1b[41mc", "\x1b[0md\n"} {
		w.Write([]byte(s))
	}
	if b.String() != "a\x1b[31mbc\x1b[0md\n" {
		t.Errorf("%q", b.String())
	}
}
