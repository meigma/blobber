package blobber

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestCacheEntry creates a cache entry and blob file for testing.
func createTestCacheEntry(t *testing.T, cacheDir, content string, age time.Duration) {
	t.Helper()

	data := []byte(content)
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	// Create directories
	blobsDir := filepath.Join(cacheDir, "blobs", "sha256")
	entriesDir := filepath.Join(cacheDir, "entries", "sha256")
	require.NoError(t, os.MkdirAll(blobsDir, 0o750))
	require.NoError(t, os.MkdirAll(entriesDir, 0o750))

	// Write blob file
	blobPath := filepath.Join(blobsDir, hashStr)
	require.NoError(t, os.WriteFile(blobPath, data, 0o600))

	// Write entry file
	entry := map[string]interface{}{
		"version":       1,
		"digest":        "sha256:" + hashStr,
		"size":          len(data),
		"media_type":    "application/octet-stream",
		"complete":      true,
		"verified":      true,
		"created_at":    time.Now().Add(-age).Format(time.RFC3339Nano),
		"last_accessed": time.Now().Add(-age).Format(time.RFC3339Nano),
	}
	entryData, _ := json.MarshalIndent(entry, "", "  ")
	entryPath := filepath.Join(entriesDir, hashStr+".json")
	require.NoError(t, os.WriteFile(entryPath, entryData, 0o600))
}

func TestCacheStats(t *testing.T) {
	t.Parallel()

	t.Run("empty cache", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		info, err := CacheStats(dir)
		require.NoError(t, err)

		assert.Equal(t, 0, info.EntryCount)
		assert.Equal(t, int64(0), info.TotalSize)
		assert.Empty(t, info.Entries)
	})

	t.Run("nonexistent cache directory", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "nonexistent")

		info, err := CacheStats(dir)
		require.NoError(t, err)

		assert.Equal(t, 0, info.EntryCount)
		assert.Equal(t, int64(0), info.TotalSize)
	})

	t.Run("cache with entries", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create test entries
		createTestCacheEntry(t, dir, "first blob", 0)
		createTestCacheEntry(t, dir, "second blob content", 0)

		info, err := CacheStats(dir)
		require.NoError(t, err)

		assert.Equal(t, 2, info.EntryCount)
		assert.Equal(t, int64(len("first blob")+len("second blob content")), info.TotalSize)
		require.Len(t, info.Entries, 2)

		for _, e := range info.Entries {
			assert.True(t, e.Complete)
			assert.NotEmpty(t, e.Digest)
			assert.Greater(t, e.Size, int64(0))
		}
	})

	t.Run("empty path returns error", func(t *testing.T) {
		t.Parallel()

		_, err := CacheStats("")
		assert.Error(t, err)
	})
}

func TestCacheClear(t *testing.T) {
	t.Parallel()

	t.Run("clears all entries", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create test entries
		createTestCacheEntry(t, dir, "blob one", 0)
		createTestCacheEntry(t, dir, "blob two", 0)

		// Verify entries exist
		info, err := CacheStats(dir)
		require.NoError(t, err)
		require.Equal(t, 2, info.EntryCount)

		// Clear
		err = CacheClear(dir)
		require.NoError(t, err)

		// Verify empty
		info, err = CacheStats(dir)
		require.NoError(t, err)
		assert.Equal(t, 0, info.EntryCount)
	})

	t.Run("nonexistent directory succeeds", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "nonexistent")

		err := CacheClear(dir)
		assert.NoError(t, err)
	})
}

func TestCachePrune(t *testing.T) {
	t.Parallel()

	t.Run("prunes by age", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create entries with different ages
		createTestCacheEntry(t, dir, "old blob", 2*time.Hour)
		createTestCacheEntry(t, dir, "new blob", 0)

		result, err := CachePrune(context.Background(), dir, CachePruneOptions{
			MaxAge: 1 * time.Hour,
		})
		require.NoError(t, err)

		assert.Equal(t, 1, result.EntriesRemoved)
		assert.Equal(t, 1, result.EntriesRemaining)
	})

	t.Run("prunes by size", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create entries
		createTestCacheEntry(t, dir, "1111111111", 2*time.Minute) // 10 bytes, older
		createTestCacheEntry(t, dir, "2222222222", 1*time.Minute) // 10 bytes, newer
		createTestCacheEntry(t, dir, "3333333333", 0)             // 10 bytes, newest

		result, err := CachePrune(context.Background(), dir, CachePruneOptions{
			MaxSize: 20, // Keep only 2 entries
		})
		require.NoError(t, err)

		assert.Equal(t, 1, result.EntriesRemoved)
		assert.Equal(t, 2, result.EntriesRemaining)
		assert.Equal(t, int64(20), result.BytesRemaining)
	})

	t.Run("nonexistent directory succeeds", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "nonexistent")

		result, err := CachePrune(context.Background(), dir, CachePruneOptions{
			MaxAge: 1 * time.Hour,
		})
		require.NoError(t, err)
		assert.Equal(t, 0, result.EntriesRemoved)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		createTestCacheEntry(t, dir, "test blob", 0)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := CachePrune(ctx, dir, CachePruneOptions{MaxAge: 0})
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestResolveCachePath(t *testing.T) {
	t.Parallel()

	t.Run("expands tilde", func(t *testing.T) {
		t.Parallel()

		home, _ := os.UserHomeDir()
		path, err := resolveCachePath("~/test")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(home, "test"), path)
	})

	t.Run("converts to absolute", func(t *testing.T) {
		t.Parallel()

		path, err := resolveCachePath("relative/path")
		require.NoError(t, err)
		assert.True(t, filepath.IsAbs(path))
	})

	t.Run("empty path returns error", func(t *testing.T) {
		t.Parallel()

		_, err := resolveCachePath("")
		assert.Error(t, err)
	})
}
