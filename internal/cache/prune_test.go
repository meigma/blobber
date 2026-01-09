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

	"github.com/meigma/blobber/core"
)

// saveEntryRaw writes an entry without modifying timestamps.
// Used in tests to set specific LastAccessed values.
func saveEntryRaw(path string, entry *Entry) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func TestCache_Size(t *testing.T) {
	t.Parallel()

	t.Run("empty cache returns zero", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cache, err := New(dir, newMockRegistry(), nil)
		require.NoError(t, err)

		size, err := cache.Size()
		require.NoError(t, err)
		assert.Equal(t, int64(0), size)
	})

	t.Run("returns total size of cached blobs", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data1, digest1 := createTestBlob("first blob content")
		data2, digest2 := createTestBlob("second blob with more content")

		reg := newMockRegistry()
		reg.addBlob(digest1, data1)
		reg.addBlob(digest2, data2)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc1 := core.LayerDescriptor{Digest: digest1, Size: int64(len(data1))}
		desc2 := core.LayerDescriptor{Digest: digest2, Size: int64(len(data2))}

		h1, err := cache.Open(context.Background(), "test.io/repo:tag1", desc1)
		require.NoError(t, err)
		h1.Close()

		h2, err := cache.Open(context.Background(), "test.io/repo:tag2", desc2)
		require.NoError(t, err)
		h2.Close()

		size, err := cache.Size()
		require.NoError(t, err)
		assert.Equal(t, int64(len(data1)+len(data2)), size)
	})
}

func TestCache_Entries(t *testing.T) {
	t.Parallel()

	t.Run("empty cache returns empty slice", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cache, err := New(dir, newMockRegistry(), nil)
		require.NoError(t, err)

		entries, err := cache.Entries()
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("returns all entries sorted by access time", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data1, digest1 := createTestBlob("entry one")
		data2, digest2 := createTestBlob("entry two")

		reg := newMockRegistry()
		reg.addBlob(digest1, data1)
		reg.addBlob(digest2, data2)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc1 := core.LayerDescriptor{Digest: digest1, Size: int64(len(data1))}
		desc2 := core.LayerDescriptor{Digest: digest2, Size: int64(len(data2))}

		// Cache first blob
		h1, err := cache.Open(context.Background(), "test.io/repo:tag1", desc1)
		require.NoError(t, err)
		h1.Close()

		// Wait a bit to ensure different access times
		time.Sleep(10 * time.Millisecond)

		// Cache second blob (more recently accessed)
		h2, err := cache.Open(context.Background(), "test.io/repo:tag2", desc2)
		require.NoError(t, err)
		h2.Close()

		entries, err := cache.Entries()
		require.NoError(t, err)
		require.Len(t, entries, 2)

		// Should be sorted by LastAccessed, most recent first
		assert.Equal(t, digest2, entries[0].Digest)
		assert.Equal(t, digest1, entries[1].Digest)
	})
}

func TestCache_Prune(t *testing.T) {
	t.Parallel()

	t.Run("no-op with zero limits", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("keep me")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{Digest: digest, Size: int64(len(data))}
		h, err := cache.Open(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		h.Close()

		result, err := cache.Prune(context.Background(), PruneOptions{})
		require.NoError(t, err)

		assert.Equal(t, 0, result.EntriesRemoved)
		assert.Equal(t, int64(0), result.BytesRemoved)
		assert.Equal(t, 1, result.EntriesRemaining)
		assert.Equal(t, int64(len(data)), result.BytesRemaining)
	})

	t.Run("evicts entries exceeding TTL", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("old content")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{Digest: digest, Size: int64(len(data))}
		h, err := cache.Open(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		h.Close()

		// Manually backdate the entry's LastAccessed time using raw write
		entryPath := filepath.Join(dir, "entries", "sha256", extractHash(digest)+".json")
		entry, err := loadEntry(entryPath)
		require.NoError(t, err)
		entry.LastAccessed = time.Now().Add(-2 * time.Hour)
		err = saveEntryRaw(entryPath, entry)
		require.NoError(t, err)

		// Prune with 1-hour TTL
		result, err := cache.Prune(context.Background(), PruneOptions{
			MaxAge: 1 * time.Hour,
		})
		require.NoError(t, err)

		assert.Equal(t, 1, result.EntriesRemoved)
		assert.Equal(t, int64(len(data)), result.BytesRemoved)
		assert.Equal(t, 0, result.EntriesRemaining)
	})

	t.Run("evicts LRU entries to meet size limit", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create three blobs of known sizes
		data1, digest1 := createTestBlob("1111111111") // 10 bytes
		data2, digest2 := createTestBlob("2222222222") // 10 bytes
		data3, digest3 := createTestBlob("3333333333") // 10 bytes

		reg := newMockRegistry()
		reg.addBlob(digest1, data1)
		reg.addBlob(digest2, data2)
		reg.addBlob(digest3, data3)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		// Cache blobs in order (oldest to newest)
		desc1 := core.LayerDescriptor{Digest: digest1, Size: int64(len(data1))}
		h1, err := cache.Open(context.Background(), "test.io/repo:tag1", desc1)
		require.NoError(t, err)
		h1.Close()

		// Backdate first entry
		entryPath1 := filepath.Join(dir, "entries", "sha256", extractHash(digest1)+".json")
		entry1, _ := loadEntry(entryPath1)
		entry1.LastAccessed = time.Now().Add(-3 * time.Minute)
		saveEntryRaw(entryPath1, entry1)

		desc2 := core.LayerDescriptor{Digest: digest2, Size: int64(len(data2))}
		h2, err := cache.Open(context.Background(), "test.io/repo:tag2", desc2)
		require.NoError(t, err)
		h2.Close()

		// Backdate second entry
		entryPath2 := filepath.Join(dir, "entries", "sha256", extractHash(digest2)+".json")
		entry2, _ := loadEntry(entryPath2)
		entry2.LastAccessed = time.Now().Add(-2 * time.Minute)
		saveEntryRaw(entryPath2, entry2)

		desc3 := core.LayerDescriptor{Digest: digest3, Size: int64(len(data3))}
		h3, err := cache.Open(context.Background(), "test.io/repo:tag3", desc3)
		require.NoError(t, err)
		h3.Close()

		// Prune to 20 bytes (should keep 2 most recent entries)
		result, err := cache.Prune(context.Background(), PruneOptions{
			MaxSize: 20,
		})
		require.NoError(t, err)

		assert.Equal(t, 1, result.EntriesRemoved)
		assert.Equal(t, int64(10), result.BytesRemoved)
		assert.Equal(t, 2, result.EntriesRemaining)
		assert.Equal(t, int64(20), result.BytesRemaining)

		// Verify oldest entry was removed
		entries, err := cache.Entries()
		require.NoError(t, err)
		require.Len(t, entries, 2)

		digests := []string{entries[0].Digest, entries[1].Digest}
		assert.NotContains(t, digests, digest1) // Oldest should be evicted
		assert.Contains(t, digests, digest2)
		assert.Contains(t, digests, digest3)
	})

	t.Run("combines TTL and size limits", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data1, digest1 := createTestBlob("old blob")
		data2, digest2 := createTestBlob("medium blob")
		data3, digest3 := createTestBlob("new blob")

		reg := newMockRegistry()
		reg.addBlob(digest1, data1)
		reg.addBlob(digest2, data2)
		reg.addBlob(digest3, data3)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		// Cache all blobs
		for i, tc := range []struct {
			digest string
			data   []byte
			age    time.Duration
		}{
			{digest1, data1, 3 * time.Hour}, // Old - should be TTL evicted
			{digest2, data2, 30 * time.Minute},
			{digest3, data3, 0},
		} {
			desc := core.LayerDescriptor{Digest: tc.digest, Size: int64(len(tc.data))}
			h, openErr := cache.Open(context.Background(), "test.io/repo:tag", desc)
			require.NoError(t, openErr)
			h.Close()

			if tc.age > 0 {
				entryPath := filepath.Join(dir, "entries", "sha256", extractHash(tc.digest)+".json")
				entry, _ := loadEntry(entryPath)
				entry.LastAccessed = time.Now().Add(-tc.age)
				saveEntryRaw(entryPath, entry)
			}
			_ = i
		}

		// Prune: TTL of 1 hour should remove data1
		result, err := cache.Prune(context.Background(), PruneOptions{
			MaxAge: 1 * time.Hour,
		})
		require.NoError(t, err)

		assert.Equal(t, 1, result.EntriesRemoved)
		assert.Equal(t, 2, result.EntriesRemaining)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("cancel test")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{Digest: digest, Size: int64(len(data))}
		h, err := cache.Open(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		h.Close()

		// Cancel context before pruning
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = cache.Prune(ctx, PruneOptions{MaxAge: 0})
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("empty cache returns zero result", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cache, err := New(dir, newMockRegistry(), nil)
		require.NoError(t, err)

		result, err := cache.Prune(context.Background(), PruneOptions{
			MaxSize: 100,
			MaxAge:  1 * time.Hour,
		})
		require.NoError(t, err)

		assert.Equal(t, 0, result.EntriesRemoved)
		assert.Equal(t, int64(0), result.BytesRemoved)
		assert.Equal(t, 0, result.EntriesRemaining)
		assert.Equal(t, int64(0), result.BytesRemaining)
	})
}

func TestCache_Prune_RemovesPartialFiles(t *testing.T) {
	t.Parallel()

	t.Run("removes partial files during eviction", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("partial file test")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{Digest: digest, Size: int64(len(data))}
		h, err := cache.Open(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		h.Close()

		// Create a stale partial file (simulating interrupted download)
		partialPath := filepath.Join(dir, "blobs", "sha256", extractHash(digest)+".partial")
		err = os.WriteFile(partialPath, []byte("partial data"), 0o600)
		require.NoError(t, err)

		// Backdate entry for TTL eviction using raw write
		entryPath := filepath.Join(dir, "entries", "sha256", extractHash(digest)+".json")
		entry, _ := loadEntry(entryPath)
		entry.LastAccessed = time.Now().Add(-2 * time.Hour)
		err = saveEntryRaw(entryPath, entry)
		require.NoError(t, err)

		// Prune
		_, err = cache.Prune(context.Background(), PruneOptions{MaxAge: 1 * time.Hour})
		require.NoError(t, err)

		// Verify all files are gone
		blobPath := filepath.Join(dir, "blobs", "sha256", extractHash(digest))
		assert.NoFileExists(t, blobPath)
		assert.NoFileExists(t, partialPath)
		assert.NoFileExists(t, entryPath)
	})
}
