//go:build unix

package library

import (
	"os/exec"
	"syscall"
)

// cancelTree runs cmd in a session of its own, so cancelling it reaches
// git's children too: they are sent SIGTERM as a group. The new session
// has no controlling terminal, so whatever git starts that wants to ask
// the user something on /dev/tty (an ssh passphrase through a custom
// core.sshCommand) fails at once instead of being stopped as a background
// reader; and a terminal hangup, which skd handles itself, is not
// delivered to them.
func cancelTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		// git leads its session and the group of the same id, and has
		// not been waited for yet, so the id cannot belong to anyone else.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
}
