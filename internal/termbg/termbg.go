// Package termbg asks the terminal for its background colour (OSC 11)
// without ever blocking for long: the question is followed by a device
// attributes request (DA1) that every terminal answers, so a terminal
// that ignores OSC 11 replies to DA1 at once, and one that answers
// nothing is given up on after a timeout.
package termbg

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Timeout is how long Detect waits for the terminal by default.
const Timeout = 300 * time.Millisecond

// Detect reports whether the controlling terminal has a light background.
// ok is false when it could not tell (no terminal, not in the foreground,
// input already waiting, no answer within timeout, Windows); callers then
// assume dark. Keys typed while it waited are kept for Unread.
func Detect(timeout time.Duration) (light, ok bool) {
	reply, keys, ok := query(timeout)
	mu.Lock()
	unread = append(unread, keys...)
	mu.Unlock()
	if !ok {
		return false, false
	}
	r, g, b, ok := parse(reply)
	if !ok {
		return false, false
	}
	return luminance(r, g, b) > 0.5, true
}

var (
	mu     sync.Mutex
	unread []byte
)

// Unread returns, once, the bytes the user typed while Detect waited for
// the terminal: they were read together with its replies and are no
// longer in the terminal's input queue. Whoever reads the keyboard next
// should handle them first.
func Unread() []byte {
	mu.Lock()
	defer mu.Unlock()
	b := unread
	unread = nil
	return b
}

// parse finds "rgb:RRRR/GGGG/BBBB" (1–4 hex digits per channel) in an
// OSC 11 reply and returns each channel scaled to 0..1.
func parse(s string) (r, g, b float64, ok bool) {
	i := strings.Index(s, "]11;")
	if i < 0 {
		return 0, 0, 0, false
	}
	s = s[i+4:]
	if j := strings.IndexAny(s, "\x07\x1b"); j >= 0 {
		s = s[:j]
	}
	s = strings.TrimPrefix(s, "rgba:")
	s = strings.TrimPrefix(s, "rgb:")
	parts := strings.Split(s, "/")
	if len(parts) < 3 {
		return 0, 0, 0, false
	}
	var c [3]float64
	for k := 0; k < 3; k++ {
		p := parts[k]
		if p == "" || len(p) > 4 {
			return 0, 0, 0, false
		}
		v, err := strconv.ParseUint(p, 16, 16)
		if err != nil {
			return 0, 0, 0, false
		}
		c[k] = float64(v) / float64(uint64(1)<<(4*len(p))-1)
	}
	return c[0], c[1], c[2], true
}

// luminance is the relative luminance (WCAG) of an sRGB colour.
func luminance(r, g, b float64) float64 {
	lin := func(c float64) float64 {
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// split separates the terminal's replies in b — OSC sequences, ended by
// BEL or ESC \, and the DA1 answer ESC [ ? … c — from everything else,
// which the user typed. da1 reports whether a complete DA1 answer was
// seen. An unfinished sequence at the end may still be a reply: it is
// left out unless final, and then kept only if it cannot be one (a lone
// ESC or ESC [ is the start of a key).
func split(b []byte, final bool) (replies string, keys []byte, da1 bool) {
	var rep []byte
	for i := 0; i < len(b); {
		if b[i] != 0x1b {
			keys = append(keys, b[i])
			i++
			continue
		}
		rest := b[i+1:]
		switch {
		case len(rest) == 0 || (rest[0] == '[' && len(rest) == 1):
			if final {
				keys = append(keys, b[i:]...)
			}
			return string(rep), keys, da1
		case rest[0] == ']':
			end := -1
			for j := 1; j < len(rest); j++ {
				if rest[j] == 0x07 {
					end = j + 1
					break
				}
				if rest[j] == 0x1b && j+1 < len(rest) && rest[j+1] == '\\' {
					end = j + 2
					break
				}
			}
			if end < 0 {
				return string(rep), keys, da1
			}
			rep = append(rep, b[i:i+1+end]...)
			i += 1 + end
		case rest[0] == '[' && rest[1] == '?':
			end := -1
			for j := 2; j < len(rest); j++ {
				if rest[j] >= 0x40 && rest[j] <= 0x7e {
					end = j + 1
					break
				}
			}
			if end < 0 {
				return string(rep), keys, da1
			}
			rep = append(rep, b[i:i+1+end]...)
			da1 = da1 || rest[end-1] == 'c'
			i += 1 + end
		default:
			keys = append(keys, b[i])
			i++
		}
	}
	return string(rep), keys, da1
}

// tailGrace is how long gather waits for the rest of a key's escape
// sequence that arrived cut off right after the DA1 answer.
const tailGrace = 100 * time.Millisecond

// gather reads the terminal's replies and the keys typed meanwhile until
// the DA1 answer has arrived, or read fails (the wait was cancelled).
// grace is called when the answer is in but a key typed right after it
// is still incomplete; the next read should then end within tailGrace.
func gather(read func([]byte) (int, error), grace func()) (reply string, unread []byte, ok bool) {
	var got []byte
	buf := make([]byte, 256)
	graced := false
	for {
		n, err := read(buf)
		got = append(got, buf[:n]...)
		// Once DA1 is in, nothing after it is a reply: the end is keys.
		_, _, da1 := split(got, false)
		replies, keys, _ := split(got, da1 || err != nil)
		if da1 && !unfinished(keys) {
			return replies, keys, true
		}
		if err != nil {
			return replies, keys, da1 || osc11Reply(replies)
		}
		if da1 && !graced {
			graced = true
			grace()
		}
	}
}

// unfinished reports whether keys end in part of an escape sequence: ESC,
// ESC O, or ESC [ with parameters but no final byte yet.
func unfinished(keys []byte) bool {
	i := bytes.LastIndexByte(keys, 0x1b)
	if i < 0 {
		return false
	}
	rest := keys[i+1:]
	switch {
	case len(rest) == 0:
		return true
	case rest[0] == 'O':
		return len(rest) == 1
	case rest[0] == '[':
		for _, c := range rest[1:] {
			if c >= 0x40 && c <= 0x7e {
				return false
			}
		}
		return true
	}
	return false
}

// osc11Reply reports whether replies hold an answer to OSC 11.
func osc11Reply(replies string) bool { return strings.Contains(replies, "]11;") }
