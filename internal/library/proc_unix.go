//go:build unix

package library

import (
	"os/exec"
	"syscall"
)

// cancelTree runs cmd in a process group of its own, so cancelling it
// reaches git's children too: they are sent SIGTERM as a group.
func cancelTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// The group is git's: its pid is the group id, and git has not
		// been waited for yet, so the id cannot belong to anyone else.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
}
