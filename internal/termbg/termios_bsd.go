//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package termbg

import "golang.org/x/sys/unix"

const (
	getTermios = unix.TIOCGETA
	setTermios = unix.TIOCSETA
	// inputQueued is FIONREAD, _IOR('f', 127, int) on every BSD; x/sys
	// does not export it there.
	inputQueued = 0x4004667f
)
