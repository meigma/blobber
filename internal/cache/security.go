package cache

import (
	"fmt"
	"os"
)

func ensureCacheFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("cache path is symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("cache path is not a regular file: %s", path)
	}
	return nil
}

func ensureCacheFileIfExists(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("cache path is symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("cache path is not a regular file: %s", path)
	}
	return nil
}
