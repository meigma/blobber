//go:build windows

package cache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

const lockFileName = ".lock"

func lockDir(dir string) (func() error, error) {
	f, err := openLockFile(dir)
	if err != nil {
		return nil, err
	}

	if err := lockFile(f, false); err != nil {
		f.Close()
		return nil, err
	}

	return unlockFunc(f), nil
}

func tryLockDir(dir string) (func() error, bool, error) {
	f, err := openLockFile(dir)
	if err != nil {
		return nil, false, err
	}

	if err := lockFile(f, true); err != nil {
		f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, false, nil
		}
		return nil, false, err
	}

	return unlockFunc(f), true, nil
}

func openLockFile(dir string) (*os.File, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("lock path is not a directory: %s", dir)
	}
	lockPath := filepath.Join(dir, lockFileName)
	return os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
}

func lockFile(f *os.File, nonblock bool) error {
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if nonblock {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	var ol windows.Overlapped
	return windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, &ol)
}

func unlockFunc(f *os.File) func() error {
	return func() error {
		var ol windows.Overlapped
		if err := windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ol); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	}
}
