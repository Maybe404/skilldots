//go:build !unix

package termbg

import "time"

// query is not implemented outside Unix: Windows consoles do not answer
// OSC 11. Callers fall back to a dark background or --theme.
func query(time.Duration) (string, bool) { return "", false }
