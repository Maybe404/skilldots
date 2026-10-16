//go:build linux

package termbg

import "golang.org/x/sys/unix"

const (
	getTermios  = unix.TCGETS
	setTermios  = unix.TCSETS
	inputQueued = unix.TIOCINQ
)
