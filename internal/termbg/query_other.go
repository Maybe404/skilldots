//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package termbg

import "time"

// query is not implemented here: Windows consoles do not answer OSC 11.
// Callers fall back to a dark background or --theme.
func query(time.Duration) (string, []byte, bool) { return "", nil, false }
