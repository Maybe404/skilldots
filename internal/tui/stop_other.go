//go:build !unix

package tui

import (
	"time"

	"github.com/Maybe404/skilldots/internal/i18n"
)

// stopDescendants is not available here: without a process tree to walk,
// the job's processes are left to end on their own.
func stopDescendants(int, time.Duration) error {
	return i18n.New("cannot stop child processes on this system")
}
