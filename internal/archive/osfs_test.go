package archive

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOSFS_Open(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	content := []byte("test content")
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.txt"), content, 0o644))

	fsys := OSFS(tmpDir)

	t.Run("valid file", func(t *testing.T) {
		t.Parallel()

		f, err := fsys.Open("file.txt")
		require.NoError(t, err)
		defer f.Close()

		data, err := io.ReadAll(f)
		require.NoError(t, err)
		assert.Equal(t, content, data)
	})

	t.Run("non-existent file", func(t *testing.T) {
		t.Parallel()

		_, err := fsys.Open("missing.txt")
		assert.Error(t, err)
	})

	t.Run("invalid path with dotdot", func(t *testing.T) {
		t.Parallel()

		_, err := fsys.Open("../escape")
		assert.ErrorIs(t, err, fs.ErrInvalid)
	})

	t.Run("invalid absolute path", func(t *testing.T) {
		t.Parallel()

		_, err := fsys.Open("/absolute/path")
		assert.ErrorIs(t, err, fs.ErrInvalid)
	})

	t.Run("current dir", func(t *testing.T) {
		t.Parallel()

		f, err := fsys.Open(".")
		require.NoError(t, err)
		defer f.Close()

		info, err := f.Stat()
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

func TestOSFS_ReadDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "b.txt"), []byte("b"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "subdir"), 0o755))

	fsys := OSFS(tmpDir)

	t.Run("root directory", func(t *testing.T) {
		t.Parallel()

		entries, err := fsys.ReadDir(".")
		require.NoError(t, err)
		assert.Len(t, entries, 3)

		names := make(map[string]bool)
		for _, e := range entries {
			names[e.Name()] = true
		}
		assert.True(t, names["a.txt"])
		assert.True(t, names["b.txt"])
		assert.True(t, names["subdir"])
	})

	t.Run("invalid path", func(t *testing.T) {
		t.Parallel()

		_, err := fsys.ReadDir("../escape")
		assert.ErrorIs(t, err, fs.ErrInvalid)
	})
}

func TestOSFS_Stat(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("content"), 0o644))

	fsys := OSFS(tmpDir)

	t.Run("valid file", func(t *testing.T) {
		t.Parallel()

		info, err := fsys.Stat("file.txt")
		require.NoError(t, err)
		assert.Equal(t, "file.txt", info.Name())
		assert.Equal(t, int64(7), info.Size())
		assert.False(t, info.IsDir())
	})

	t.Run("directory", func(t *testing.T) {
		t.Parallel()

		info, err := fsys.Stat(".")
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("invalid path", func(t *testing.T) {
		t.Parallel()

		_, err := fsys.Stat("../escape")
		assert.ErrorIs(t, err, fs.ErrInvalid)
	})
}

func TestOSFS_Lstat(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "target.txt")
	require.NoError(t, os.WriteFile(targetPath, []byte("target"), 0o644))

	fsys := OSFS(tmpDir)

	t.Run("regular file", func(t *testing.T) {
		t.Parallel()

		info, err := fsys.Lstat("target.txt")
		require.NoError(t, err)
		assert.Equal(t, "target.txt", info.Name())
		assert.False(t, info.Mode()&fs.ModeSymlink != 0)
	})

	t.Run("invalid path", func(t *testing.T) {
		t.Parallel()

		_, err := fsys.Lstat("../escape")
		assert.ErrorIs(t, err, fs.ErrInvalid)
	})
}

func TestOSFS_Symlink(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "target.txt")
	require.NoError(t, os.WriteFile(targetPath, []byte("target content"), 0o644))

	linkPath := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink("target.txt", linkPath); err != nil {
		if runtime.GOOS == osWindows {
			t.Skipf("skipping symlink test on windows: %v", err)
		}
		require.NoError(t, err)
	}

	fsys := OSFS(tmpDir)

	t.Run("Lstat returns symlink info", func(t *testing.T) {
		t.Parallel()

		info, err := fsys.Lstat("link.txt")
		require.NoError(t, err)
		assert.True(t, info.Mode()&fs.ModeSymlink != 0, "expected symlink mode")
	})

	t.Run("Stat follows symlink", func(t *testing.T) {
		t.Parallel()

		info, err := fsys.Stat("link.txt")
		require.NoError(t, err)
		assert.False(t, info.Mode()&fs.ModeSymlink != 0, "Stat should follow symlink")
		assert.Equal(t, int64(14), info.Size()) // "target content" length
	})

	t.Run("ReadLink returns target", func(t *testing.T) {
		t.Parallel()

		target, err := fsys.ReadLink("link.txt")
		require.NoError(t, err)
		assert.Equal(t, "target.txt", target)
	})

	t.Run("ReadLink on non-symlink fails", func(t *testing.T) {
		t.Parallel()

		_, err := fsys.ReadLink("target.txt")
		assert.Error(t, err)
	})

	t.Run("ReadLink invalid path", func(t *testing.T) {
		t.Parallel()

		_, err := fsys.ReadLink("../escape")
		assert.ErrorIs(t, err, fs.ErrInvalid)
	})
}

func TestOSFS_InterfaceCompliance(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	fsys := OSFS(tmpDir)

	// Verify interface compliance at runtime
	var _ fs.FS = fsys
	var _ fs.ReadDirFS = fsys
	var _ fs.StatFS = fsys
	var _ lstatFS = fsys
	var _ readLinkFS = fsys
}
