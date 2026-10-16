//go:build rawoutput

package cli

import "io"

// guard leaves w as it is: a rawoutput build is for tests that check what
// the commands print before the last line of defence.
func guard(w io.Writer) io.Writer { return w }
