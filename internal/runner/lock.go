package runner

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// DefaultLockPath is held by `garmr run` while it is active.
const DefaultLockPath = "/run/garmr.lock"

// ErrLocked is returned when another process holds the lock.
var ErrLocked = errors.New("garmr run is already active")

// Lock is an exclusive flock on a file. It is released when the process exits.
type Lock struct{ f *os.File }

// Acquire takes the lock at path without blocking.
func Acquire(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return &Lock{f: f}, nil
}

// Release drops the lock.
func (l *Lock) Release() error {
	return l.f.Close()
}

// Held reports whether another process currently holds the lock at path.
func Held(path string) (bool, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return true, nil
	}
	return false, err
}
