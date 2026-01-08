package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheDir(t *testing.T) {
	t.Run("uses XDG_CACHE_HOME when set", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "/custom/cache")

		dir, err := CacheDir()
		require.NoError(t, err)
		assert.Equal(t, "/custom/cache/blobber", dir)
	})

	t.Run("defaults to ~/.cache when XDG_CACHE_HOME not set", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "")

		home, err := os.UserHomeDir()
		require.NoError(t, err)

		dir, err := CacheDir()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(home, ".cache", "blobber"), dir)
	})
}

func TestDir(t *testing.T) {
	t.Run("uses XDG_CONFIG_HOME when set", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/custom/config")

		dir, err := Dir()
		require.NoError(t, err)
		assert.Equal(t, "/custom/config/blobber", dir)
	})

	t.Run("defaults to ~/.config when XDG_CONFIG_HOME not set", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")

		home, err := os.UserHomeDir()
		require.NoError(t, err)

		dir, err := Dir()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(home, ".config", "blobber"), dir)
	})
}
