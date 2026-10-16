package cli

import (
	"io"
	"os"
	"sync"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/Maybe404/skilldots/internal/safetext"
)

// stdout and stderr are where everything skd prints for a person goes
// (JSON goes to os.Stdout as it is: it is data). Text from outside skd is
// cleaned where it is read or printed; these writers are the last line
// of defence behind that: whatever still holds a control character, a
// bidi control or an escape sequence other than skd's own colours is
// cleaned on the way out (safetext.Screen). A build with the rawoutput
// tag leaves them out, so the tests see what the commands themselves
// print (TestHostileLibraryPrintsClean).
var (
	stdout io.Writer = guard(os.Stdout)
	stderr io.Writer = guard(os.Stderr)
)

// screenWriter cleans each write with safetext.Screen. A write that ends
// inside a UTF-8 sequence keeps those bytes for the next one, so a rune
// split between two writes is not taken for invalid UTF-8.
type screenWriter struct {
	mu    sync.Mutex
	w     io.Writer
	carry []byte
}

func (s *screenWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := append(s.carry, p...)
	s.carry = nil
	// Keep an unfinished rune (at most 3 bytes) for the next write.
	for i := len(b) - 1; i >= 0 && i >= len(b)-3; i-- {
		if utf8.RuneStart(b[i]) {
			if !utf8.FullRune(b[i:]) {
				s.carry = append([]byte{}, b[i:]...)
				b = b[:i]
			}
			break
		}
	}
	// skd's colour codes stay unless colour is off (NO_COLOR, not a
	// terminal): then no escape sequence at all is skd's.
	out := safetext.Screen(string(b), lipgloss.ColorProfile() != termenv.Ascii)
	if _, err := io.WriteString(s.w, out); err != nil {
		return 0, err
	}
	return len(p), nil
}
