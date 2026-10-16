//go:build unix

package termbg

import (
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/muesli/cancelreader"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// query writes OSC 11 and DA1 to /dev/tty in raw mode and reads until the
// DA1 answer arrives or timeout passes. A read still pending at the
// timeout is cancelled, so nothing is left to swallow later keystrokes.
func query(timeout time.Duration) (string, bool) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", false
	}
	defer tty.Close()
	fd := int(tty.Fd())
	// Only the foreground process may change the terminal: a background
	// job doing so would be stopped by SIGTTOU (and hang until resumed).
	if pg, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP); err != nil || pg != syscall.Getpgrp() {
		return "", false
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		return "", false
	}
	defer term.Restore(fd, old)
	r, err := cancelreader.NewReader(tty)
	if err != nil {
		return "", false
	}
	defer r.Close()
	if _, err := tty.WriteString("\x1b]11;?\x1b\\\x1b[c"); err != nil {
		return "", false
	}
	timer := time.AfterFunc(timeout, func() { r.Cancel() })
	defer timer.Stop()
	var got strings.Builder
	buf := make([]byte, 256)
	for {
		n, err := r.Read(buf)
		got.Write(buf[:n])
		if da1Done(got.String()) {
			return got.String(), true
		}
		if err != nil {
			return got.String(), strings.Contains(got.String(), "]11;")
		}
	}
}
