// Package config provides configuration management for the blobber CLI.
package config

import (
	"os"
	"path/filepath"
)

// CacheDir returns the blobber cache directory.
// Uses XDG_CACHE_HOME/blobber, defaulting to ~/.cache/blobber.
func CacheDir() (string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "blobber"), nil
}

// Dir returns the blobber config directory.
// Uses XDG_CONFIG_HOME/blobber, defaulting to ~/.config/blobber.
func Dir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "blobber"), nil
}
