//go:build !unix && !windows

package pack

import "os"

// tryLock has no file lock to take here; the in-process lock still holds.
func tryLock(f *os.File) (bool, error) { return true, nil }

func unlockFile(f *os.File) {}
