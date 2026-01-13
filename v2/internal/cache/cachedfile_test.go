package cache

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachedFile_FullRead(t *testing.T) {
	t.Parallel()

	// Create file cache.
	cacheDir := t.TempDir()
	fc, err := NewFileCache(cacheDir)
	require.NoError(t, err)

	// Create a test file using fstest.MapFS.
	content := []byte("hello world")
	testFS := fstest.MapFS{
		"test.txt": &fstest.MapFile{Data: content, Mode: 0o644},
	}

	// Open the file.
	inner, err := testFS.Open("test.txt")
	require.NoError(t, err)

	// Get the underlying fileCache for newCachedFile.
	fileCache := fc.(*fileCache)
	blobDigest := digest.FromString("test-blob")

	// Wrap with caching.
	wrapped := newCachedFile(inner, fileCache, blobDigest, "test.txt", int64(len(content)), "")

	// Read all content.
	data, err := io.ReadAll(wrapped)
	require.NoError(t, err)
	assert.Equal(t, content, data)

	// Close the file.
	err = wrapped.Close()
	require.NoError(t, err)

	// Verify file was cached.
	cachedPath := filepath.Join(fileCache.blobDir(blobDigest), "test.txt")
	cachedContent, err := os.ReadFile(cachedPath)
	require.NoError(t, err)
	assert.Equal(t, content, cachedContent)

	entry, err := fileCache.loadEntry(blobDigest)
	require.NoError(t, err)
	assert.False(t, isCompleteEntry(entry))
	assert.Equal(t, int64(len(content)), entry.Size)
}

func TestCachedFile_PartialRead(t *testing.T) {
	t.Parallel()

	// Create file cache.
	cacheDir := t.TempDir()
	fc, err := NewFileCache(cacheDir)
	require.NoError(t, err)

	// Create a test file.
	content := []byte("hello world")
	testFS := fstest.MapFS{
		"test.txt": &fstest.MapFile{Data: content, Mode: 0o644},
	}

	inner, err := testFS.Open("test.txt")
	require.NoError(t, err)

	fileCache := fc.(*fileCache)
	blobDigest := digest.FromString("test-blob-partial")

	wrapped := newCachedFile(inner, fileCache, blobDigest, "partial.txt", int64(len(content)), "")

	// Read only part of the content.
	buf := make([]byte, 5)
	n, err := wrapped.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, []byte("hello"), buf)

	// Close without reading the rest.
	err = wrapped.Close()
	require.NoError(t, err)

	// Verify file was NOT cached (partial read).
	cachedPath := filepath.Join(fileCache.blobDir(blobDigest), "partial.txt")
	_, err = os.Stat(cachedPath)
	assert.True(t, os.IsNotExist(err), "partial read should not cache file")

	_, err = fileCache.loadEntry(blobDigest)
	assert.True(t, os.IsNotExist(err), "partial read should not create cache metadata")
}

func TestCachedFile_ChecksumMismatch(t *testing.T) {
	t.Parallel()

	// Create file cache.
	cacheDir := t.TempDir()
	fc, err := NewFileCache(cacheDir)
	require.NoError(t, err)

	// Create a test file.
	content := []byte("hello world")
	testFS := fstest.MapFS{
		"test.txt": &fstest.MapFile{Data: content, Mode: 0o644},
	}

	inner, err := testFS.Open("test.txt")
	require.NoError(t, err)

	fileCache := fc.(*fileCache)
	blobDigest := digest.FromString("test-blob-checksum")

	// Use wrong checksum.
	wrongChecksum := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	wrapped := newCachedFile(inner, fileCache, blobDigest, "checksum.txt", int64(len(content)), wrongChecksum)

	// Read all content.
	data, err := io.ReadAll(wrapped)
	require.NoError(t, err)
	assert.Equal(t, content, data)

	// Close the file.
	err = wrapped.Close()
	require.NoError(t, err)

	// Verify file was NOT cached (checksum mismatch).
	cachedPath := filepath.Join(fileCache.blobDir(blobDigest), "checksum.txt")
	_, err = os.Stat(cachedPath)
	assert.True(t, os.IsNotExist(err), "checksum mismatch should not cache file")

	_, err = fileCache.loadEntry(blobDigest)
	assert.True(t, os.IsNotExist(err), "checksum mismatch should not create cache metadata")
}

func TestCachedFile_ChecksumMatch(t *testing.T) {
	t.Parallel()

	// Create file cache.
	cacheDir := t.TempDir()
	fc, err := NewFileCache(cacheDir)
	require.NoError(t, err)

	// Create a test file.
	content := []byte("hello world")
	testFS := fstest.MapFS{
		"test.txt": &fstest.MapFile{Data: content, Mode: 0o644},
	}

	inner, err := testFS.Open("test.txt")
	require.NoError(t, err)

	fileCache := fc.(*fileCache)
	blobDigest := digest.FromString("test-blob-valid-checksum")

	// Use correct checksum.
	correctChecksum := "sha256:" + digest.FromBytes(content).Encoded()
	wrapped := newCachedFile(inner, fileCache, blobDigest, "valid.txt", int64(len(content)), correctChecksum)

	// Read all content.
	data, err := io.ReadAll(wrapped)
	require.NoError(t, err)
	assert.Equal(t, content, data)

	// Close the file.
	err = wrapped.Close()
	require.NoError(t, err)

	// Verify file was cached (checksum matched).
	cachedPath := filepath.Join(fileCache.blobDir(blobDigest), "valid.txt")
	cachedContent, err := os.ReadFile(cachedPath)
	require.NoError(t, err)
	assert.Equal(t, content, cachedContent)
}
