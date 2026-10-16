package tui

import (
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// csiKeys are the escape sequences of the keys the dashboard uses.
var csiKeys = map[string]tea.KeyType{
	"[A": tea.KeyUp, "[B": tea.KeyDown, "[C": tea.KeyRight, "[D": tea.KeyLeft,
	"OA": tea.KeyUp, "OB": tea.KeyDown, "OC": tea.KeyRight, "OD": tea.KeyLeft,
	"[H": tea.KeyHome, "[F": tea.KeyEnd, "OH": tea.KeyHome, "OF": tea.KeyEnd,
	"[Z": tea.KeyShiftTab, "[3~": tea.KeyDelete, "[5~": tea.KeyPgUp, "[6~": tea.KeyPgDown,
}

// replayKeys turns bytes typed before the dashboard started (while the
// terminal was asked for its background) into the key messages Bubble Tea
// would have produced, one per key. Sequences it does not know are
// dropped rather than misread.
func replayKeys(b []byte) []tea.Msg {
	var out []tea.Msg
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == 0x1b:
			rest := b[i+1:]
			if len(rest) > 0 && (rest[0] == '[' || rest[0] == 'O') {
				// CSI or SS3: parameters, then a final byte.
				j := 1
				for j < len(rest) && (rest[j] < 0x40 || rest[j] > 0x7e) {
					j++
				}
				if j < len(rest) {
					if k, ok := csiKeys[string(rest[:j+1])]; ok {
						out = append(out, tea.KeyMsg{Type: k})
					}
					i += 2 + j
					continue
				}
				if rest[0] == '[' {
					i = len(b) // unfinished sequence
					continue
				}
			}
			if r, w := utf8.DecodeRune(rest); len(rest) > 0 && r != utf8.RuneError && r > ' ' && r != 0x7f {
				out = append(out, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: true})
				i += 1 + w
				continue
			}
			out = append(out, tea.KeyMsg{Type: tea.KeyEscape})
			i++
		case c == ' ':
			out = append(out, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			i++
		case c < 0x20 || c == 0x7f:
			// Control keys: Bubble Tea's key types are their byte values
			// (enter is ctrl+m, tab ctrl+i, backspace DEL).
			out = append(out, tea.KeyMsg{Type: tea.KeyType(c)})
			i++
		default:
			r, w := utf8.DecodeRune(b[i:])
			if r != utf8.RuneError {
				out = append(out, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			}
			i += w
		}
	}
	return out
}
