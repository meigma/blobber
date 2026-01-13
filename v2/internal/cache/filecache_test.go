package cache

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileCache_Dir(t *testing.T) {
	t.Parallel()

	cache, err := NewFileCache(t.TempDir())
	require.NoError(t, err)

	d := digest.FromString("test-blob")

	dir, err := cache.Dir(d)
	require.NoError(t, err)
	assert.DirExists(t, dir)

	// Calling Dir again returns the same path.
	dir2, err := cache.Dir(d)
	require.NoError(t, err)
	assert.Equal(t, dir, dir2)
}

func TestFileCache_IsComplete_NotComplete(t *testing.T) {
	t.Parallel()

	cache, err := NewFileCache(t.TempDir())
	require.NoError(t, err)

	d := digest.FromString("missing-blob")
	assert.False(t, cache.IsComplete(d))
}

func TestFileCache_MarkComplete(t *testing.T) {
	t.Parallel()

	cache, err := NewFileCache(t.TempDir())
	require.NoError(t, err)

	d := digest.FromString("test-blob")

	// Create the directory first.
	_, err = cache.Dir(d)
	require.NoError(t, err)

	// Not complete yet.
	assert.False(t, cache.IsComplete(d))

	// Mark complete.
	err = cache.MarkComplete(d, 12345)
	require.NoError(t, err)

	// Now complete.
	assert.True(t, cache.IsComplete(d))
}

func TestFileCache_Get_NotCached(t *testing.T) {
	t.Parallel()

	cache, err := NewFileCache(t.TempDir())
	require.NoError(t, err)

	d := digest.FromString("missing-blob")

	_, err = cache.Get(d, "file.txt")
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestFileCache_Get_NotComplete(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	cache, err := NewFileCache(cacheDir)
	require.NoError(t, err)

	d := digest.FromString("incomplete-blob")

	// Create directory and file, but don't mark complete.
	dir, err := cache.Dir(d)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0o644)
	require.NoError(t, err)

	// Should fail because blob is not marked complete.
	_, err = cache.Get(d, "file.txt")
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestFileCache_Get_Success(t *testing.T) {
	t.Parallel()

	cache, err := NewFileCache(t.TempDir())
	require.NoError(t, err)

	d := digest.FromString("test-blob")
	expectedContent := "hello world"

	// Create directory and file.
	dir, err := cache.Dir(d)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "file.txt"), []byte(expectedContent), 0o644)
	require.NoError(t, err)

	// Mark complete.
	err = cache.MarkComplete(d, int64(len(expectedContent)))
	require.NoError(t, err)

	// Get should succeed.
	f, err := cache.Get(d, "file.txt")
	require.NoError(t, err)
	defer f.Close()

	content, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, string(content))
}

func TestFileCache_Get_NestedPath(t *testing.T) {
	t.Parallel()

	cache, err := NewFileCache(t.TempDir())
	require.NoError(t, err)

	d := digest.FromString("test-blob")

	// Create directory structure.
	dir, err := cache.Dir(d)
	require.NoError(t, err)

	nestedDir := filepath.Join(dir, "data", "config")
	err = os.MkdirAll(nestedDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(nestedDir, "settings.json"), []byte(`{"key": "value"}`), 0o644)
	require.NoError(t, err)

	err = cache.MarkComplete(d, 100)
	require.NoError(t, err)

	// Get with nested path.
	f, err := cache.Get(d, "data/config/settings.json")
	require.NoError(t, err)
	defer f.Close()

	content, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, `{"key": "value"}`, string(content))
}

func TestFileCache_Get_FileNotFound(t *testing.T) {
	t.Parallel()

	cache, err := NewFileCache(t.TempDir())
	require.NoError(t, err)

	d := digest.FromString("test-blob")

	// Create directory and mark complete, but no files.
	_, err = cache.Dir(d)
	require.NoError(t, err)

	err = cache.MarkComplete(d, 0)
	require.NoError(t, err)

	// Get should fail for nonexistent file.
	_, err = cache.Get(d, "nonexistent.txt")
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestFileCache_TouchAccess(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	cache, err := NewFileCache(cacheDir)
	require.NoError(t, err)

	d := digest.FromString("test-blob")

	// Create and mark complete.
	_, err = cache.Dir(d)
	require.NoError(t, err)

	err = cache.MarkComplete(d, 100)
	require.NoError(t, err)

	// Touch access.
	err = cache.TouchAccess(d)
	require.NoError(t, err)

	// Verify the cache.json was updated (we can't easily check the time,
	// but we can verify it didn't error).
	assert.True(t, cache.IsComplete(d))
}

func TestFileCache_DifferentDigests(t *testing.T) {
	t.Parallel()

	cache, err := NewFileCache(t.TempDir())
	require.NoError(t, err)

	d1 := digest.FromString("blob-1")
	d2 := digest.FromString("blob-2")

	// Create both.
	dir1, err := cache.Dir(d1)
	require.NoError(t, err)
	dir2, err := cache.Dir(d2)
	require.NoError(t, err)

	// Directories should be different.
	assert.NotEqual(t, dir1, dir2)

	// Write different content.
	err = os.WriteFile(filepath.Join(dir1, "file.txt"), []byte("content-1"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir2, "file.txt"), []byte("content-2"), 0o644)
	require.NoError(t, err)

	// Mark both complete.
	require.NoError(t, cache.MarkComplete(d1, 9))
	require.NoError(t, cache.MarkComplete(d2, 9))

	// Get from each.
	f1, err := cache.Get(d1, "file.txt")
	require.NoError(t, err)
	content1, _ := io.ReadAll(f1)
	f1.Close()

	f2, err := cache.Get(d2, "file.txt")
	require.NoError(t, err)
	content2, _ := io.ReadAll(f2)
	f2.Close()

	assert.Equal(t, "content-1", string(content1))
	assert.Equal(t, "content-2", string(content2))
}
