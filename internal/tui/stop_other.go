//go:build !unix

package tui

import (
	"errors"
	"time"
)

// stopDescendants is not available here: without a process tree to walk,
// the job's processes are left to end on their own.
func stopDescendants(int, time.Duration) error {
	return errors.New("cannot stop child processes on this system")
}
