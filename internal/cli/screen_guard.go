//go:build !rawoutput

package cli

import "io"

func guard(w io.Writer) io.Writer { return &screenWriter{w: w} }
