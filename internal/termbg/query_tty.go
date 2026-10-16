//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package termbg

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/muesli/cancelreader"
	"golang.org/x/sys/unix"
)

// query writes OSC 11 and DA1 to /dev/tty and reads until the DA1 answer
// arrives or timeout passes. A read still pending at the timeout is
// cancelled, so nothing is left to swallow later keystrokes.
//
// The terminal is put in cbreak mode, not raw: ISIG stays on, so ctrl+c
// and ctrl+\ still interrupt skd (the terminal is restored first); input
// is otherwise passed through as in raw mode (no CR→LF, no flow control). Keys
// the user typed before the query are left alone: when input is already
// waiting nothing is asked. Keys typed during the wait are read with the
// replies; they are returned as unread for the dashboard to replay.
func query(timeout time.Duration) (reply string, unread []byte, ok bool) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", nil, false
	}
	defer tty.Close()
	fd := int(tty.Fd())
	// Only the foreground process may change the terminal: a background
	// job doing so would be stopped by SIGTTOU (and hang until resumed).
	if pg, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP); err != nil || pg != syscall.Getpgrp() {
		return "", nil, false
	}
	old, err := unix.IoctlGetTermios(fd, getTermios)
	if err != nil {
		return "", nil, false
	}
	cb := *old
	cb.Lflag &^= unix.ICANON | unix.ECHO
	// Keys arrive as the dashboard's raw mode would deliver them: Enter as
	// CR (ICRNL would make it LF, read as ctrl+j), ctrl+s and ctrl+q as
	// keys rather than flow control (IXON), ctrl+v and ctrl+o as keys
	// rather than literal-next and discard-output (IEXTEN). ISIG stays.
	cb.Iflag &^= unix.ICRNL | unix.IXON
	cb.Lflag &^= unix.IEXTEN
	cb.Cc[unix.VMIN], cb.Cc[unix.VTIME] = 1, 0
	// Set without flushing: type-ahead stays in the input queue.
	if err := unix.IoctlSetTermios(fd, setTermios, &cb); err != nil {
		return "", nil, false
	}
	restore := sync.OnceFunc(func() { unix.IoctlSetTermios(fd, setTermios, old) })
	defer restore()

	// A signal from the keyboard ends skd as it would have without the
	// query, but with the terminal back in its own mode.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP)
	done := make(chan struct{})
	defer func() {
		signal.Stop(sigs)
		close(done)
	}()
	go func() {
		select {
		case sig := <-sigs:
			restore()
			signal.Reset(sig)
			syscall.Kill(syscall.Getpid(), sig.(syscall.Signal))
		case <-done:
		}
	}()

	// Out of canonical mode a partly typed line counts too.
	if n, err := unix.IoctlGetInt(fd, inputQueued); err != nil || n > 0 {
		return "", nil, false
	}
	r, err := cancelreader.NewReader(tty)
	if err != nil {
		return "", nil, false
	}
	defer r.Close()
	if _, err := tty.WriteString("\x1b]11;?\x1b\\\x1b[c"); err != nil {
		return "", nil, false
	}
	timer := time.AfterFunc(timeout, func() { r.Cancel() })
	defer timer.Stop()
	var got []byte
	buf := make([]byte, 256)
	for {
		n, err := r.Read(buf)
		got = append(got, buf[:n]...)
		replies, keys, da1 := split(got, false)
		if da1 {
			return replies, keys, true
		}
		if err != nil {
			replies, keys, _ = split(got, true)
			return replies, keys, osc11Reply(replies)
		}
	}
}
