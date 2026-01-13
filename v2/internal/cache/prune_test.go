package cache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPruneFileCache_MaxAge(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	blobsDir := filepath.Join(cacheDir, "blobs")
	require.NoError(t, os.MkdirAll(blobsDir, 0o700))

	// Create three blobs with different access times.
	now := time.Now()

	// Old blob (accessed 2 hours ago).
	oldDir := filepath.Join(blobsDir, "old-blob")
	require.NoError(t, os.MkdirAll(oldDir, 0o700))
	writeEntry(t, oldDir, blobEntry{
		Digest:       "sha256:old",
		Size:         1000,
		LastAccessed: now.Add(-2 * time.Hour),
	})
	require.NoError(t, os.WriteFile(filepath.Join(oldDir, "file.txt"), []byte("old"), 0o600))

	// Recent blob (accessed 30 minutes ago).
	recentDir := filepath.Join(blobsDir, "recent-blob")
	require.NoError(t, os.MkdirAll(recentDir, 0o700))
	writeEntry(t, recentDir, blobEntry{
		Digest:       "sha256:recent",
		Size:         2000,
		LastAccessed: now.Add(-30 * time.Minute),
	})
	require.NoError(t, os.WriteFile(filepath.Join(recentDir, "file.txt"), []byte("recent"), 0o600))

	// Very recent blob (accessed 5 minutes ago).
	newDir := filepath.Join(blobsDir, "new-blob")
	require.NoError(t, os.MkdirAll(newDir, 0o700))
	writeEntry(t, newDir, blobEntry{
		Digest:       "sha256:new",
		Size:         3000,
		LastAccessed: now.Add(-5 * time.Minute),
	})
	require.NoError(t, os.WriteFile(filepath.Join(newDir, "file.txt"), []byte("new"), 0o600))

	// Prune entries older than 1 hour.
	err := PruneFileCache(context.Background(), cacheDir, PruneStrategy{
		MaxAge: 1 * time.Hour,
	})
	require.NoError(t, err)

	// Old blob should be removed.
	_, err = os.Stat(oldDir)
	assert.True(t, os.IsNotExist(err), "old blob should be removed")

	// Recent and new blobs should remain.
	_, err = os.Stat(recentDir)
	assert.NoError(t, err, "recent blob should remain")

	_, err = os.Stat(newDir)
	assert.NoError(t, err, "new blob should remain")
}

func TestPruneFileCache_MaxSize(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	blobsDir := filepath.Join(cacheDir, "blobs")
	require.NoError(t, os.MkdirAll(blobsDir, 0o700))

	now := time.Now()

	// Create three blobs with different sizes and access times.
	// Oldest (1000 bytes, accessed 3 hours ago).
	oldestDir := filepath.Join(blobsDir, "oldest")
	require.NoError(t, os.MkdirAll(oldestDir, 0o700))
	writeEntry(t, oldestDir, blobEntry{
		Digest:       "sha256:oldest",
		Size:         1000,
		LastAccessed: now.Add(-3 * time.Hour),
	})

	// Middle (2000 bytes, accessed 2 hours ago).
	middleDir := filepath.Join(blobsDir, "middle")
	require.NoError(t, os.MkdirAll(middleDir, 0o700))
	writeEntry(t, middleDir, blobEntry{
		Digest:       "sha256:middle",
		Size:         2000,
		LastAccessed: now.Add(-2 * time.Hour),
	})

	// Newest (3000 bytes, accessed 1 hour ago).
	newestDir := filepath.Join(blobsDir, "newest")
	require.NoError(t, os.MkdirAll(newestDir, 0o700))
	writeEntry(t, newestDir, blobEntry{
		Digest:       "sha256:newest",
		Size:         3000,
		LastAccessed: now.Add(-1 * time.Hour),
	})

	// Total: 6000 bytes. Prune to 4000 bytes max.
	err := PruneFileCache(context.Background(), cacheDir, PruneStrategy{
		MaxSize: 4000,
	})
	require.NoError(t, err)

	// Oldest should be removed (leaves 5000, need to remove more).
	_, err = os.Stat(oldestDir)
	assert.True(t, os.IsNotExist(err), "oldest blob should be removed")

	// Middle should also be removed (leaves 3000, under limit).
	_, err = os.Stat(middleDir)
	assert.True(t, os.IsNotExist(err), "middle blob should be removed")

	// Newest should remain.
	_, err = os.Stat(newestDir)
	assert.NoError(t, err, "newest blob should remain")
}

func TestPruneFileCache_CombinedStrategy(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	blobsDir := filepath.Join(cacheDir, "blobs")
	require.NoError(t, os.MkdirAll(blobsDir, 0o700))

	now := time.Now()

	// Create blobs that test the combined strategy.
	// Very old blob (should be removed by MaxAge).
	veryOldDir := filepath.Join(blobsDir, "very-old")
	require.NoError(t, os.MkdirAll(veryOldDir, 0o700))
	writeEntry(t, veryOldDir, blobEntry{
		Digest:       "sha256:very-old",
		Size:         1000,
		LastAccessed: now.Add(-48 * time.Hour),
	})

	// Recent but should be removed by MaxSize (oldest of the recent ones).
	recentOldDir := filepath.Join(blobsDir, "recent-old")
	require.NoError(t, os.MkdirAll(recentOldDir, 0o700))
	writeEntry(t, recentOldDir, blobEntry{
		Digest:       "sha256:recent-old",
		Size:         5000,
		LastAccessed: now.Add(-6 * time.Hour),
	})

	// Recent and should remain.
	recentNewDir := filepath.Join(blobsDir, "recent-new")
	require.NoError(t, os.MkdirAll(recentNewDir, 0o700))
	writeEntry(t, recentNewDir, blobEntry{
		Digest:       "sha256:recent-new",
		Size:         3000,
		LastAccessed: now.Add(-1 * time.Hour),
	})

	// Apply combined strategy: MaxAge=24h, MaxSize=4000.
	err := PruneFileCache(context.Background(), cacheDir, PruneStrategy{
		MaxAge:  24 * time.Hour,
		MaxSize: 4000,
	})
	require.NoError(t, err)

	// Very old removed by MaxAge.
	_, err = os.Stat(veryOldDir)
	assert.True(t, os.IsNotExist(err), "very old blob should be removed by MaxAge")

	// Recent old removed by MaxSize.
	_, err = os.Stat(recentOldDir)
	assert.True(t, os.IsNotExist(err), "recent old blob should be removed by MaxSize")

	// Recent new should remain.
	_, err = os.Stat(recentNewDir)
	assert.NoError(t, err, "recent new blob should remain")
}

func TestPruneFileCache_EmptyCache(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()

	// Pruning an empty cache should not error.
	err := PruneFileCache(context.Background(), cacheDir, PruneStrategy{
		MaxAge:  1 * time.Hour,
		MaxSize: 1000,
	})
	assert.NoError(t, err)
}

func TestPruneFileCache_NoStrategy(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	blobsDir := filepath.Join(cacheDir, "blobs")
	require.NoError(t, os.MkdirAll(blobsDir, 0o700))

	// Create a blob.
	blobDir := filepath.Join(blobsDir, "test-blob")
	require.NoError(t, os.MkdirAll(blobDir, 0o700))
	writeEntry(t, blobDir, blobEntry{
		Digest:       "sha256:test",
		Size:         1000,
		LastAccessed: time.Now().Add(-24 * time.Hour),
	})

	// Pruning with zero strategy should not remove anything.
	err := PruneFileCache(context.Background(), cacheDir, PruneStrategy{})
	require.NoError(t, err)

	_, err = os.Stat(blobDir)
	assert.NoError(t, err, "blob should remain when no strategy is set")
}

func TestPruneFileCache_ContextCancellation(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	blobsDir := filepath.Join(cacheDir, "blobs")
	require.NoError(t, os.MkdirAll(blobsDir, 0o700))

	// Create multiple blobs to be pruned.
	now := time.Now()
	for i := range 10 {
		dir := filepath.Join(blobsDir, string(rune('a'+i)))
		require.NoError(t, os.MkdirAll(dir, 0o700))
		writeEntry(t, dir, blobEntry{
			Digest:       "sha256:test",
			Size:         100,
			LastAccessed: now.Add(-48 * time.Hour),
		})
	}

	// Cancel context immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := PruneFileCache(ctx, cacheDir, PruneStrategy{
		MaxAge: 1 * time.Hour,
	})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestPruneFileCache_SkipsIncompleteEntries(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	blobsDir := filepath.Join(cacheDir, "blobs")
	require.NoError(t, os.MkdirAll(blobsDir, 0o700))

	// Create a directory without cache.json (incomplete).
	incompleteDir := filepath.Join(blobsDir, "incomplete")
	require.NoError(t, os.MkdirAll(incompleteDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(incompleteDir, "file.txt"), []byte("data"), 0o600))

	// Create a complete entry.
	completeDir := filepath.Join(blobsDir, "complete")
	require.NoError(t, os.MkdirAll(completeDir, 0o700))
	writeEntry(t, completeDir, blobEntry{
		Digest:       "sha256:complete",
		Size:         1000,
		LastAccessed: time.Now().Add(-2 * time.Hour),
	})

	// Prune with MaxAge that would remove complete entry.
	err := PruneFileCache(context.Background(), cacheDir, PruneStrategy{
		MaxAge: 1 * time.Hour,
	})
	require.NoError(t, err)

	// Complete entry should be removed.
	_, err = os.Stat(completeDir)
	assert.True(t, os.IsNotExist(err), "complete blob should be removed")

	// Incomplete directory should remain (not considered in pruning).
	_, err = os.Stat(incompleteDir)
	assert.NoError(t, err, "incomplete directory should remain")
}

func TestPruneFileCache_PartialEntry(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	blobsDir := filepath.Join(cacheDir, "blobs")
	require.NoError(t, os.MkdirAll(blobsDir, 0o700))

	partialDir := filepath.Join(blobsDir, "partial")
	require.NoError(t, os.MkdirAll(partialDir, 0o700))
	writeEntry(t, partialDir, blobEntry{
		Digest:       "sha256:partial",
		Size:         100,
		LastAccessed: time.Now().Add(-2 * time.Hour),
		Complete:     boolPtr(false),
	})

	err := PruneFileCache(context.Background(), cacheDir, PruneStrategy{
		MaxAge: 1 * time.Hour,
	})
	require.NoError(t, err)

	_, err = os.Stat(partialDir)
	assert.True(t, os.IsNotExist(err), "partial entry should be removed")
}

// writeEntry writes a blobEntry to cache.json in the given directory.
func writeEntry(t *testing.T, dir string, entry blobEntry) {
	t.Helper()
	data, err := json.MarshalIndent(entry, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cache.json"), data, 0o600))
}
