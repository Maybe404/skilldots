//go:build !unix

package library

import "os/exec"

// cancelTree keeps exec's default: cancelling kills git itself. Windows
// has no process groups to signal; children git started may outlive it.
func cancelTree(cmd *exec.Cmd) {}
