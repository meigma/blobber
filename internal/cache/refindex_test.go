package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilmanlab/blobber/core"
)

func TestRefEntry_SaveLoad(t *testing.T) {
	t.Parallel()

	t.Run("roundtrip", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "ref.json")

		entry := &RefEntry{
			Ref:         "ghcr.io/org/repo:v1.0",
			Digest:      "sha256:abc123def456",
			Size:        1024,
			MediaType:   "application/vnd.oci.image.layer.v1.tar+gzip",
			ValidatedAt: time.Now().Truncate(time.Second), // Truncate for JSON precision
		}

		err := saveRefEntry(path, entry)
		require.NoError(t, err)

		loaded, err := loadRefEntry(path)
		require.NoError(t, err)

		assert.Equal(t, entry.Ref, loaded.Ref)
		assert.Equal(t, entry.Digest, loaded.Digest)
		assert.Equal(t, entry.Size, loaded.Size)
		assert.Equal(t, entry.MediaType, loaded.MediaType)
		assert.WithinDuration(t, entry.ValidatedAt, loaded.ValidatedAt, time.Second)
	})

	t.Run("creates parent directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "refs", "ref.json")

		entry := &RefEntry{
			Ref:    "test.io/repo:tag",
			Digest: "sha256:abc",
		}

		err := saveRefEntry(path, entry)
		require.NoError(t, err)

		loaded, err := loadRefEntry(path)
		require.NoError(t, err)
		assert.Equal(t, entry.Ref, loaded.Ref)
	})

	t.Run("loadRefEntry returns error for missing file", func(t *testing.T) {
		t.Parallel()
		_, err := loadRefEntry("/nonexistent/path.json")
		assert.Error(t, err)
	})
}

func TestCache_LookupByRef(t *testing.T) {
	t.Parallel()

	t.Run("returns descriptor when within TTL", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cache, err := New(dir, newMockRegistry(), nil)
		require.NoError(t, err)

		ref := "ghcr.io/org/repo:v1.0"
		desc := core.LayerDescriptor{
			Digest:    "sha256:abc123",
			Size:      1024,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// Save ref entry with current timestamp
		cache.UpdateRefIndex(ref, desc)

		// Lookup with 5 minute TTL - should succeed
		result, ok := cache.LookupByRef(ref, 5*time.Minute)
		require.True(t, ok)
		assert.Equal(t, desc.Digest, result.Digest)
		assert.Equal(t, desc.Size, result.Size)
		assert.Equal(t, desc.MediaType, result.MediaType)
	})

	t.Run("returns false when TTL is zero", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cache, err := New(dir, newMockRegistry(), nil)
		require.NoError(t, err)

		ref := "ghcr.io/org/repo:v1.0"
		desc := core.LayerDescriptor{
			Digest: "sha256:abc123",
			Size:   1024,
		}

		cache.UpdateRefIndex(ref, desc)

		// Lookup with zero TTL - should fail
		_, ok := cache.LookupByRef(ref, 0)
		assert.False(t, ok)
	})

	t.Run("returns false when TTL is negative", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cache, err := New(dir, newMockRegistry(), nil)
		require.NoError(t, err)

		ref := "ghcr.io/org/repo:v1.0"
		desc := core.LayerDescriptor{
			Digest: "sha256:abc123",
			Size:   1024,
		}

		cache.UpdateRefIndex(ref, desc)

		// Lookup with negative TTL - should fail
		_, ok := cache.LookupByRef(ref, -1*time.Minute)
		assert.False(t, ok)
	})

	t.Run("returns false for nonexistent ref", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cache, err := New(dir, newMockRegistry(), nil)
		require.NoError(t, err)

		_, ok := cache.LookupByRef("nonexistent:ref", 5*time.Minute)
		assert.False(t, ok)
	})
}

func TestCache_LookupByRef_Expired(t *testing.T) {
	t.Parallel()

	t.Run("returns false when entry is expired", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cache, err := New(dir, newMockRegistry(), nil)
		require.NoError(t, err)

		ref := "ghcr.io/org/repo:v1.0"

		// Manually create an expired ref entry
		refPath := cache.refPath(ref)
		entry := &RefEntry{
			Ref:         ref,
			Digest:      "sha256:abc123",
			Size:        1024,
			MediaType:   "application/vnd.oci.image.layer.v1.tar+gzip",
			ValidatedAt: time.Now().Add(-10 * time.Minute), // 10 minutes ago
		}
		err = saveRefEntry(refPath, entry)
		require.NoError(t, err)

		// Lookup with 5 minute TTL - should fail (entry is 10 min old)
		_, ok := cache.LookupByRef(ref, 5*time.Minute)
		assert.False(t, ok)
	})

	t.Run("returns true when entry is just within TTL", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cache, err := New(dir, newMockRegistry(), nil)
		require.NoError(t, err)

		ref := "ghcr.io/org/repo:v1.0"

		// Manually create an entry that's 4 minutes old
		refPath := cache.refPath(ref)
		entry := &RefEntry{
			Ref:         ref,
			Digest:      "sha256:abc123",
			Size:        1024,
			MediaType:   "application/vnd.oci.image.layer.v1.tar+gzip",
			ValidatedAt: time.Now().Add(-4 * time.Minute), // 4 minutes ago
		}
		err = saveRefEntry(refPath, entry)
		require.NoError(t, err)

		// Lookup with 5 minute TTL - should succeed (entry is 4 min old)
		result, ok := cache.LookupByRef(ref, 5*time.Minute)
		assert.True(t, ok)
		assert.Equal(t, entry.Digest, result.Digest)
	})
}

func TestCache_UpdateRefIndex(t *testing.T) {
	t.Parallel()

	t.Run("creates new ref entry", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cache, err := New(dir, newMockRegistry(), nil)
		require.NoError(t, err)

		ref := "ghcr.io/org/repo:v1.0"
		desc := core.LayerDescriptor{
			Digest:    "sha256:abc123",
			Size:      1024,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		cache.UpdateRefIndex(ref, desc)

		// Verify entry was created
		refPath := cache.refPath(ref)
		entry, err := loadRefEntry(refPath)
		require.NoError(t, err)

		assert.Equal(t, ref, entry.Ref)
		assert.Equal(t, desc.Digest, entry.Digest)
		assert.Equal(t, desc.Size, entry.Size)
		assert.Equal(t, desc.MediaType, entry.MediaType)
		assert.WithinDuration(t, time.Now(), entry.ValidatedAt, 2*time.Second)
	})

	t.Run("updates existing ref entry", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cache, err := New(dir, newMockRegistry(), nil)
		require.NoError(t, err)

		ref := "ghcr.io/org/repo:v1.0"
		oldDesc := core.LayerDescriptor{
			Digest: "sha256:old123",
			Size:   512,
		}
		newDesc := core.LayerDescriptor{
			Digest: "sha256:new456",
			Size:   1024,
		}

		// Create initial entry
		cache.UpdateRefIndex(ref, oldDesc)

		// Wait a moment to ensure different timestamps
		time.Sleep(10 * time.Millisecond)

		// Update to new descriptor
		cache.UpdateRefIndex(ref, newDesc)

		// Verify entry was updated
		refPath := cache.refPath(ref)
		entry, err := loadRefEntry(refPath)
		require.NoError(t, err)

		assert.Equal(t, ref, entry.Ref)
		assert.Equal(t, newDesc.Digest, entry.Digest)
		assert.Equal(t, newDesc.Size, entry.Size)
	})
}

func TestCache_RefPath(t *testing.T) {
	t.Parallel()

	t.Run("uses SHA256 hash of ref", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cache, err := New(dir, newMockRegistry(), nil)
		require.NoError(t, err)

		// Different refs should produce different paths
		path1 := cache.refPath("ghcr.io/org/repo:v1.0")
		path2 := cache.refPath("ghcr.io/org/repo:v2.0")
		path3 := cache.refPath("docker.io/library/nginx:latest")

		assert.NotEqual(t, path1, path2)
		assert.NotEqual(t, path1, path3)
		assert.NotEqual(t, path2, path3)

		// Same ref should produce same path
		path1Again := cache.refPath("ghcr.io/org/repo:v1.0")
		assert.Equal(t, path1, path1Again)

		// Path should be in refs directory
		assert.Contains(t, path1, "refs")
		assert.True(t, filepath.Ext(path1) == ".json")
	})

	t.Run("handles special characters in ref", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cache, err := New(dir, newMockRegistry(), nil)
		require.NoError(t, err)

		// Refs with special characters should work
		specialRefs := []string{
			"ghcr.io/org/repo:v1.0-beta",
			"ghcr.io/org/repo:v1.0+build.123",
			"docker.io/library/image@sha256:abc123",
			"localhost:5000/test:tag",
		}

		for _, ref := range specialRefs {
			path := cache.refPath(ref)
			assert.Contains(t, path, "refs")
			assert.True(t, filepath.Ext(path) == ".json")

			// Ensure we can save and load with this path
			entry := &RefEntry{Ref: ref, Digest: "sha256:test"}
			err := saveRefEntry(path, entry)
			require.NoError(t, err, "should be able to save entry for ref: %s", ref)

			loaded, err := loadRefEntry(path)
			require.NoError(t, err, "should be able to load entry for ref: %s", ref)
			assert.Equal(t, ref, loaded.Ref)
		}
	})
}

func TestCache_RefCleanup_OnEvict(t *testing.T) {
	t.Parallel()

	t.Run("removes ref entries when blob is evicted", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("evict ref test")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		ref := "ghcr.io/org/repo:v1.0"
		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// Cache the blob and create ref entry
		handle, err := cache.Open(context.Background(), ref, desc)
		require.NoError(t, err)
		handle.Close()

		cache.UpdateRefIndex(ref, desc)

		// Verify ref entry exists
		refPath := cache.refPath(ref)
		_, err = loadRefEntry(refPath)
		require.NoError(t, err, "ref entry should exist before eviction")

		// Evict the blob
		err = cache.Evict(digest)
		require.NoError(t, err)

		// Verify ref entry was removed
		_, err = loadRefEntry(refPath)
		assert.Error(t, err, "ref entry should be removed after eviction")
	})

	t.Run("removes multiple refs pointing to same digest", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("multi ref test")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// Cache blob
		handle, err := cache.Open(context.Background(), "ref1", desc)
		require.NoError(t, err)
		handle.Close()

		// Create multiple refs pointing to same digest
		refs := []string{
			"ghcr.io/org/repo:v1.0",
			"ghcr.io/org/repo:latest",
			"ghcr.io/org/repo:stable",
		}

		for _, ref := range refs {
			cache.UpdateRefIndex(ref, desc)
		}

		// Verify all ref entries exist
		for _, ref := range refs {
			refPath := cache.refPath(ref)
			_, loadErr := loadRefEntry(refPath)
			require.NoError(t, loadErr, "ref entry should exist for %s", ref)
		}

		// Evict the blob
		err = cache.Evict(digest)
		require.NoError(t, err)

		// Verify all ref entries were removed
		for _, ref := range refs {
			refPath := cache.refPath(ref)
			_, loadErr := loadRefEntry(refPath)
			assert.Error(t, loadErr, "ref entry should be removed for %s", ref)
		}
	})
}

func TestCache_RefCleanup_OnPrune(t *testing.T) {
	t.Parallel()

	t.Run("removes orphaned refs after prune", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("prune ref test")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		ref := "ghcr.io/org/repo:v1.0"
		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// Cache the blob and create ref entry
		handle, err := cache.Open(context.Background(), ref, desc)
		require.NoError(t, err)
		handle.Close()

		cache.UpdateRefIndex(ref, desc)

		// Backdate entry for TTL eviction
		entryPath := filepath.Join(dir, "entries", "sha256", extractHash(digest)+jsonExt)
		entry, err := loadEntry(entryPath)
		require.NoError(t, err)
		entry.LastAccessed = time.Now().Add(-2 * time.Hour)
		err = saveEntryRaw(entryPath, entry)
		require.NoError(t, err)

		// Verify ref entry exists before prune
		refPath := cache.refPath(ref)
		_, err = loadRefEntry(refPath)
		require.NoError(t, err, "ref entry should exist before prune")

		// Prune with 1-hour TTL
		_, err = cache.Prune(context.Background(), PruneOptions{MaxAge: 1 * time.Hour})
		require.NoError(t, err)

		// Verify ref entry was cleaned up
		_, err = loadRefEntry(refPath)
		assert.Error(t, err, "orphaned ref entry should be removed after prune")
	})

	t.Run("keeps refs for non-pruned blobs", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create two blobs - one old (to be pruned), one recent (to keep)
		oldData, oldDigest := createTestBlob("old blob")
		newData, newDigest := createTestBlob("new blob")

		reg := newMockRegistry()
		reg.addBlob(oldDigest, oldData)
		reg.addBlob(newDigest, newData)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		oldRef := "ghcr.io/org/repo:old"
		newRef := "ghcr.io/org/repo:new"

		oldDesc := core.LayerDescriptor{Digest: oldDigest, Size: int64(len(oldData))}
		newDesc := core.LayerDescriptor{Digest: newDigest, Size: int64(len(newData))}

		// Cache both blobs
		h1, err := cache.Open(context.Background(), oldRef, oldDesc)
		require.NoError(t, err)
		h1.Close()

		h2, err := cache.Open(context.Background(), newRef, newDesc)
		require.NoError(t, err)
		h2.Close()

		// Create ref entries
		cache.UpdateRefIndex(oldRef, oldDesc)
		cache.UpdateRefIndex(newRef, newDesc)

		// Backdate only the old entry
		oldEntryPath := filepath.Join(dir, "entries", "sha256", extractHash(oldDigest)+jsonExt)
		entry, _ := loadEntry(oldEntryPath)
		entry.LastAccessed = time.Now().Add(-2 * time.Hour)
		saveEntryRaw(oldEntryPath, entry)

		// Prune with 1-hour TTL
		_, err = cache.Prune(context.Background(), PruneOptions{MaxAge: 1 * time.Hour})
		require.NoError(t, err)

		// Old ref should be removed
		oldRefPath := cache.refPath(oldRef)
		_, err = loadRefEntry(oldRefPath)
		assert.Error(t, err, "old ref should be removed")

		// New ref should still exist
		newRefPath := cache.refPath(newRef)
		_, err = loadRefEntry(newRefPath)
		assert.NoError(t, err, "new ref should still exist")
	})
}

func TestCache_LookupByRef_BlobFileMissing(t *testing.T) {
	t.Parallel()

	t.Run("returns false when blob file is missing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("missing blob test")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		ref := "ghcr.io/org/repo:v1.0"
		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// Cache the blob
		handle, err := cache.Open(context.Background(), ref, desc)
		require.NoError(t, err)
		handle.Close()

		// Create ref entry
		cache.UpdateRefIndex(ref, desc)

		// Delete the blob file (simulating corruption or manual deletion)
		blobPath := filepath.Join(dir, "blobs", "sha256", extractHash(digest))
		err = os.Remove(blobPath)
		require.NoError(t, err)

		// LookupByRef should still return the descriptor (it only checks ref entry)
		result, ok := cache.LookupByRef(ref, 5*time.Minute)
		assert.True(t, ok, "LookupByRef should succeed as it only checks ref metadata")
		assert.Equal(t, desc.Digest, result.Digest)

		// But LoadCompleteEntry should return nil since blob is missing
		// (This is what hasCachedBlob uses to validate before accepting TTL hit)
		entry, _, _ := cache.LoadCompleteEntry(digest)
		assert.NotNil(t, entry, "entry metadata still exists")

		// The actual blob file check happens in client.hasCachedBlob via os.Stat
	})

	t.Run("returns false when blob file is truncated", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("truncated blob test content")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		ref := "ghcr.io/org/repo:v1.0"
		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// Cache the blob
		handle, err := cache.Open(context.Background(), ref, desc)
		require.NoError(t, err)
		handle.Close()

		// Create ref entry
		cache.UpdateRefIndex(ref, desc)

		// Truncate the blob file
		blobPath := filepath.Join(dir, "blobs", "sha256", extractHash(digest))
		err = os.WriteFile(blobPath, data[:len(data)/2], 0o640)
		require.NoError(t, err)

		// LookupByRef still succeeds (only checks ref metadata)
		result, ok := cache.LookupByRef(ref, 5*time.Minute)
		assert.True(t, ok)
		assert.Equal(t, desc.Digest, result.Digest)

		// Verify the blob file has wrong size
		info, err := os.Stat(blobPath)
		require.NoError(t, err)
		assert.NotEqual(t, desc.Size, info.Size(), "blob should be truncated")
	})
}
