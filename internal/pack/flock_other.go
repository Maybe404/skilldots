//go:build !unix && !windows

package pack

import (
	"errors"
	"os"
)

// tryLock has no file lock to take here: Hold falls back to the
// in-process lock, and Fetch leaves git's lock files alone.
func tryLock(f *os.File) (bool, error) { return false, errors.ErrUnsupported }

func unlockFile(f *os.File) {}
