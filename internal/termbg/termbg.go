// Package termbg asks the terminal for its background colour (OSC 11)
// without ever blocking for long: the question is followed by a device
// attributes request (DA1) that every terminal answers, so a terminal
// that ignores OSC 11 replies to DA1 at once, and one that answers
// nothing is given up on after a timeout.
package termbg

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// Timeout is how long Detect waits for the terminal by default.
const Timeout = 300 * time.Millisecond

// Detect reports whether the controlling terminal has a light background.
// ok is false when it could not tell (no terminal, not in the foreground,
// no answer within timeout, Windows); callers then assume dark.
func Detect(timeout time.Duration) (light, ok bool) {
	reply, ok := query(timeout)
	if !ok {
		return false, false
	}
	r, g, b, ok := parse(reply)
	if !ok {
		return false, false
	}
	return luminance(r, g, b) > 0.5, true
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

// da1Done reports whether s ends with a complete DA1 reply, ESC [ ? … c.
func da1Done(s string) bool {
	i := strings.LastIndex(s, "\x1b[?")
	return i >= 0 && strings.HasSuffix(s, "c") && !strings.ContainsAny(s[i+3:len(s)-1], "\x1b")
}
