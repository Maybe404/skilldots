//go:build unix

package pack

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes an exclusive advisory lock on f without waiting; ok is
// false when another process holds it.
func tryLock(f *os.File) (ok bool, err error) {
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func unlockFile(f *os.File) { syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
