//go:build !windows

package cache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const lockFileName = ".lock"

func lockDir(dir string) (func() error, error) {
	f, err := openLockFile(dir)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
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

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
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

func unlockFunc(f *os.File) func() error {
	return func() error {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	}
}
