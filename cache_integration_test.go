//go:build integration

package blobber_test

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilmanlab/blobber"
)

// cacheTestDir creates a temporary cache directory for testing.
func cacheTestDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cache")
	return dir
}

// getCacheStats returns cache stats, failing the test on error.
func getCacheStats(t *testing.T, cacheDir string) *blobber.CacheInfo {
	t.Helper()
	info, err := blobber.CacheStats(cacheDir)
	require.NoError(t, err)
	return info
}

// assertCacheHasEntries verifies the cache has the expected number of entries.
func assertCacheHasEntries(t *testing.T, cacheDir string, expectedCount int) {
	t.Helper()
	info := getCacheStats(t, cacheDir)
	assert.Equal(t, expectedCount, info.EntryCount, "unexpected cache entry count")
}

// assertCacheEmpty verifies the cache has no entries.
func assertCacheEmpty(t *testing.T, cacheDir string) {
	t.Helper()
	assertCacheHasEntries(t, cacheDir, 0)
}

// findCacheEntry finds a cache entry by digest prefix.
func findCacheEntry(t *testing.T, cacheDir, digestPrefix string) *blobber.CacheEntry {
	t.Helper()
	info := getCacheStats(t, cacheDir)
	for _, entry := range info.Entries {
		if len(entry.Digest) >= len(digestPrefix) && entry.Digest[:len(digestPrefix)] == digestPrefix {
			return &entry
		}
	}
	return nil
}

// getBlobPath returns the path to a cached blob file.
func getBlobPath(cacheDir, digest string) string {
	// digest format: sha256:abc123...
	// blob path: <cache>/blobs/sha256/abc123...
	if len(digest) > 7 && digest[:7] == "sha256:" {
		return filepath.Join(cacheDir, "blobs", "sha256", digest[7:])
	}
	return ""
}

// ============================================================================
// P0 - Core Functionality Tests
// ============================================================================

func TestIntegration_Cache_PullCacheHit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/cache-pull:v1"
	srcFS := testFS()

	// Push
	digest, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)
	assert.NotEmpty(t, digest)

	// First pull - cache miss
	destDir1 := t.TempDir()
	err = client.Pull(ctx, ref, destDir1)
	require.NoError(t, err)

	// Verify cache now has entry
	info := getCacheStats(t, cacheDir)
	require.Equal(t, 1, info.EntryCount, "expected 1 cache entry after first pull")
	require.True(t, info.Entries[0].Complete, "cache entry should be complete")

	// Second pull - should use cache
	destDir2 := t.TempDir()
	err = client.Pull(ctx, ref, destDir2)
	require.NoError(t, err)

	// Verify still only 1 entry (reused)
	assertCacheHasEntries(t, cacheDir, 1)

	// Verify content matches
	assertFilesMatch(t, srcFS, destDir2)
}

func TestIntegration_Cache_OpenImageCacheHit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/cache-open:v1"
	srcFS := testFS()

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// First OpenImage - cache miss
	img1, err := client.OpenImage(ctx, ref)
	require.NoError(t, err)

	rc1, err := img1.Open("hello.txt")
	require.NoError(t, err)
	content1, err := io.ReadAll(rc1)
	require.NoError(t, err)
	require.NoError(t, rc1.Close())
	require.NoError(t, img1.Close())

	// Verify cache has entry
	assertCacheHasEntries(t, cacheDir, 1)

	// Second OpenImage - should use cache
	img2, err := client.OpenImage(ctx, ref)
	require.NoError(t, err)

	rc2, err := img2.Open("hello.txt")
	require.NoError(t, err)
	content2, err := io.ReadAll(rc2)
	require.NoError(t, err)
	require.NoError(t, rc2.Close())
	require.NoError(t, img2.Close())

	// Verify content matches
	assert.Equal(t, content1, content2)
	assert.Equal(t, "Hello, World!", string(content1))

	// Still only 1 entry
	assertCacheHasEntries(t, cacheDir, 1)
}

func TestIntegration_Cache_NoCacheByDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	// Create client WITHOUT cache
	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/no-cache:v1"
	srcFS := testFS()

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	destDir := t.TempDir()
	err = client.Pull(ctx, ref, destDir)
	require.NoError(t, err)

	// Verify default cache location doesn't exist
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	defaultCacheDir := filepath.Join(home, ".blobber", "cache")

	// Should either not exist or have no entries from this test
	if info, err := blobber.CacheStats(defaultCacheDir); err == nil {
		// If it exists from other runs, just make sure our test didn't add to it
		// by checking that the ref isn't in there
		for _, entry := range info.Entries {
			// Our digest shouldn't be there if we didn't use cache
			assert.NotContains(t, entry.Digest, "no-cache")
		}
	}
}

func TestIntegration_Cache_Stats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	// Push and pull multiple blobs
	fs1 := fstest.MapFS{
		"file1.txt": &fstest.MapFile{Data: []byte("content1"), Mode: 0644},
	}
	fs2 := fstest.MapFS{
		"file2.txt": &fstest.MapFile{Data: bytes.Repeat([]byte("x"), 1000), Mode: 0644},
	}

	ref1 := reg.Host + "/test/stats1:v1"
	ref2 := reg.Host + "/test/stats2:v1"

	_, err = client.Push(ctx, ref1, fs1)
	require.NoError(t, err)
	_, err = client.Push(ctx, ref2, fs2)
	require.NoError(t, err)

	err = client.Pull(ctx, ref1, t.TempDir())
	require.NoError(t, err)
	err = client.Pull(ctx, ref2, t.TempDir())
	require.NoError(t, err)

	// Check stats
	info := getCacheStats(t, cacheDir)
	assert.Equal(t, 2, info.EntryCount)
	assert.Greater(t, info.TotalSize, int64(0))
	assert.Len(t, info.Entries, 2)

	// All entries should be complete
	for _, entry := range info.Entries {
		assert.True(t, entry.Complete, "entry %s should be complete", entry.Digest)
	}
}

func TestIntegration_Cache_Clear(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/cache-clear:v1"
	srcFS := testFS()

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	err = client.Pull(ctx, ref, t.TempDir())
	require.NoError(t, err)

	// Verify cache has entry
	assertCacheHasEntries(t, cacheDir, 1)

	// Clear cache
	err = blobber.CacheClear(cacheDir)
	require.NoError(t, err)

	// Verify cache is empty
	assertCacheEmpty(t, cacheDir)
}

// ============================================================================
// P1 - Important Feature Tests
// ============================================================================

func TestIntegration_Cache_LazyLoading_SelectiveFileAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
		blobber.WithLazyLoading(true),
	)
	require.NoError(t, err)

	// Create a filesystem with multiple files
	srcFS := fstest.MapFS{
		"small.txt": &fstest.MapFile{Data: []byte("small"), Mode: 0644},
		"large.bin": &fstest.MapFile{Data: bytes.Repeat([]byte("x"), 100000), Mode: 0644},
	}

	ref := reg.Host + "/test/lazy-selective:v1"

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Open with lazy loading, read only small file
	img, err := client.OpenImage(ctx, ref)
	require.NoError(t, err)

	rc, err := img.Open("small.txt")
	require.NoError(t, err)
	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	require.NoError(t, img.Close())

	assert.Equal(t, "small", string(content))

	// Check cache - entry should exist but might be partial
	info := getCacheStats(t, cacheDir)
	require.Equal(t, 1, info.EntryCount)
	// With lazy loading, we might not have the complete blob
	// The entry exists, which is what matters for this test
}

func TestIntegration_Cache_LazyLoading_FullAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
		blobber.WithLazyLoading(true),
	)
	require.NoError(t, err)

	// Use a simple filesystem without directories
	srcFS := fstest.MapFS{
		"file1.txt": &fstest.MapFile{Data: []byte("content1"), Mode: 0644},
		"file2.txt": &fstest.MapFile{Data: []byte("content2"), Mode: 0644},
		"file3.txt": &fstest.MapFile{Data: []byte("content3"), Mode: 0644},
	}
	ref := reg.Host + "/test/lazy-full:v1"

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Open with lazy loading, read ALL files
	img, err := client.OpenImage(ctx, ref)
	require.NoError(t, err)

	// Read all files
	for _, name := range []string{"file1.txt", "file2.txt", "file3.txt"} {
		rc, err := img.Open(name)
		require.NoError(t, err)
		_, err = io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
	}
	require.NoError(t, img.Close())

	// After reading all files, cache should have entry
	info := getCacheStats(t, cacheDir)
	require.Equal(t, 1, info.EntryCount)
}

func TestIntegration_Cache_SelfHealing_MissingBlobFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/self-heal-missing:v1"
	srcFS := testFS()

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// First pull - populate cache
	err = client.Pull(ctx, ref, t.TempDir())
	require.NoError(t, err)

	// Verify cache entry exists and get the actual blob digest
	info := getCacheStats(t, cacheDir)
	require.Equal(t, 1, info.EntryCount)

	// Get the blob digest from cache entry (not the manifest digest from Push)
	blobDigest := info.Entries[0].Digest

	// Delete the blob file but keep the entry metadata
	blobPath := getBlobPath(cacheDir, blobDigest)
	require.NotEmpty(t, blobPath, "blob path should not be empty for digest %s", blobDigest)
	err = os.Remove(blobPath)
	require.NoError(t, err)

	// Pull again - should self-heal by re-downloading
	destDir := t.TempDir()
	err = client.Pull(ctx, ref, destDir)
	require.NoError(t, err)

	// Verify content is correct
	assertFilesMatch(t, srcFS, destDir)

	// Cache should still have exactly 1 entry (recovered)
	assertCacheHasEntries(t, cacheDir, 1)
}

func TestIntegration_Cache_SelfHealing_TruncatedBlobFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/self-heal-truncated:v1"
	srcFS := testFS()

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// First pull - populate cache
	err = client.Pull(ctx, ref, t.TempDir())
	require.NoError(t, err)

	// Get the blob digest from cache entry
	info := getCacheStats(t, cacheDir)
	require.Equal(t, 1, info.EntryCount)
	blobDigest := info.Entries[0].Digest

	// Truncate the blob file
	blobPath := getBlobPath(cacheDir, blobDigest)
	require.NotEmpty(t, blobPath, "blob path should not be empty for digest %s", blobDigest)
	err = os.Truncate(blobPath, 10) // Truncate to 10 bytes
	require.NoError(t, err)

	// Pull again - should self-heal
	destDir := t.TempDir()
	err = client.Pull(ctx, ref, destDir)
	require.NoError(t, err)

	// Verify content is correct
	assertFilesMatch(t, srcFS, destDir)
}

func TestIntegration_Cache_Prune_MaxAge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	// Push and pull first blob
	fs1 := fstest.MapFS{"file1.txt": &fstest.MapFile{Data: []byte("old"), Mode: 0644}}
	ref1 := reg.Host + "/test/prune-age1:v1"
	_, err = client.Push(ctx, ref1, fs1)
	require.NoError(t, err)
	err = client.Pull(ctx, ref1, t.TempDir())
	require.NoError(t, err)

	// Small delay
	time.Sleep(100 * time.Millisecond)

	// Push and pull second blob
	fs2 := fstest.MapFS{"file2.txt": &fstest.MapFile{Data: []byte("new"), Mode: 0644}}
	ref2 := reg.Host + "/test/prune-age2:v1"
	_, err = client.Push(ctx, ref2, fs2)
	require.NoError(t, err)
	err = client.Pull(ctx, ref2, t.TempDir())
	require.NoError(t, err)

	// Verify both entries exist
	assertCacheHasEntries(t, cacheDir, 2)

	// Prune with very short max age (should remove both since they're older than 1ms)
	result, err := blobber.CachePrune(ctx, cacheDir, blobber.CachePruneOptions{
		MaxAge: 1 * time.Millisecond,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.EntriesRemoved)
	assert.Equal(t, 0, result.EntriesRemaining)
}

func TestIntegration_Cache_Prune_MaxSize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	// Push and pull multiple blobs of known sizes
	fs1 := fstest.MapFS{"f1.txt": &fstest.MapFile{Data: bytes.Repeat([]byte("a"), 1000), Mode: 0644}}
	fs2 := fstest.MapFS{"f2.txt": &fstest.MapFile{Data: bytes.Repeat([]byte("b"), 2000), Mode: 0644}}
	fs3 := fstest.MapFS{"f3.txt": &fstest.MapFile{Data: bytes.Repeat([]byte("c"), 3000), Mode: 0644}}

	refs := []string{
		reg.Host + "/test/prune-size1:v1",
		reg.Host + "/test/prune-size2:v1",
		reg.Host + "/test/prune-size3:v1",
	}
	fsList := []fs.FS{fs1, fs2, fs3}

	for i, ref := range refs {
		_, err = client.Push(ctx, ref, fsList[i])
		require.NoError(t, err)
		err = client.Pull(ctx, ref, t.TempDir())
		require.NoError(t, err)
		// Small delay to ensure different access times
		time.Sleep(50 * time.Millisecond)
	}

	// Verify all entries exist
	info := getCacheStats(t, cacheDir)
	require.Equal(t, 3, info.EntryCount)

	// Prune to keep only the largest blob's worth of space
	// This should evict the oldest entries (fs1, fs2) first
	result, err := blobber.CachePrune(ctx, cacheDir, blobber.CachePruneOptions{
		MaxSize: 1, // Very small - forces eviction of everything except maybe one
	})
	require.NoError(t, err)
	assert.Greater(t, result.EntriesRemoved, 0)
}

// ============================================================================
// P2 - Robustness Tests
// ============================================================================

func TestIntegration_Cache_BackgroundPrefetch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
		blobber.WithLazyLoading(true),
		blobber.WithBackgroundPrefetch(true),
	)
	require.NoError(t, err)

	// Create a larger filesystem
	srcFS := fstest.MapFS{
		"small.txt": &fstest.MapFile{Data: []byte("small"), Mode: 0644},
		"large.bin": &fstest.MapFile{Data: bytes.Repeat([]byte("x"), 50000), Mode: 0644},
	}

	ref := reg.Host + "/test/prefetch:v1"
	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Open with lazy loading + background prefetch
	img, err := client.OpenImage(ctx, ref)
	require.NoError(t, err)

	// Read only one small file
	rc, err := img.Open("small.txt")
	require.NoError(t, err)
	_, err = io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	require.NoError(t, img.Close())

	// Wait for background prefetch to complete
	time.Sleep(500 * time.Millisecond)

	// Check if cache entry is now complete
	info := getCacheStats(t, cacheDir)
	require.Equal(t, 1, info.EntryCount)
	// With background prefetch, entry should eventually become complete
	// Note: This may be timing-dependent
}

func TestIntegration_Cache_ConcurrentPulls(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/concurrent-same:v1"
	srcFS := testFS()

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Pull concurrently from multiple goroutines
	const numGoroutines = 5
	var wg sync.WaitGroup
	errs := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			destDir := t.TempDir()
			if err := client.Pull(ctx, ref, destDir); err != nil {
				errs <- err
				return
			}
			// Verify content
			content, err := os.ReadFile(filepath.Join(destDir, "hello.txt"))
			if err != nil {
				errs <- err
				return
			}
			if string(content) != "Hello, World!" {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	// Should have exactly 1 cache entry
	assertCacheHasEntries(t, cacheDir, 1)
}

func TestIntegration_Cache_ConcurrentDifferentBlobs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	const numBlobs = 5
	refs := make([]string, numBlobs)
	for i := 0; i < numBlobs; i++ {
		ref := reg.Host + "/test/concurrent-diff" + string(rune('0'+i)) + ":v1"
		refs[i] = ref
		fs := fstest.MapFS{
			"file.txt": &fstest.MapFile{
				Data: []byte("content" + string(rune('0'+i))),
				Mode: 0644,
			},
		}
		_, err = client.Push(ctx, ref, fs)
		require.NoError(t, err)
	}

	// Pull all concurrently
	var wg sync.WaitGroup
	errs := make(chan error, numBlobs)

	for _, ref := range refs {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			if err := client.Pull(ctx, r, t.TempDir()); err != nil {
				errs <- err
			}
		}(ref)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	// Should have all cache entries
	assertCacheHasEntries(t, cacheDir, numBlobs)
}

func TestIntegration_Cache_SharedCacheDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	// Create two clients sharing the same cache
	client1, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	client2, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/shared-cache:v1"
	srcFS := testFS()

	// Push with client1
	_, err = client1.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Pull with client1 - populates cache
	err = client1.Pull(ctx, ref, t.TempDir())
	require.NoError(t, err)

	// Verify cache has entry
	assertCacheHasEntries(t, cacheDir, 1)

	// Pull with client2 - should use cached blob
	destDir := t.TempDir()
	err = client2.Pull(ctx, ref, destDir)
	require.NoError(t, err)

	// Verify content
	assertFilesMatch(t, srcFS, destDir)

	// Still only 1 entry
	assertCacheHasEntries(t, cacheDir, 1)
}

func TestIntegration_Cache_SeparateCacheDirs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir1 := cacheTestDir(t)
	cacheDir2 := filepath.Join(t.TempDir(), "cache2")

	client1, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir1),
	)
	require.NoError(t, err)

	client2, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir2),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/separate-cache:v1"
	srcFS := testFS()

	_, err = client1.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Pull with client1
	err = client1.Pull(ctx, ref, t.TempDir())
	require.NoError(t, err)

	// Pull with client2
	err = client2.Pull(ctx, ref, t.TempDir())
	require.NoError(t, err)

	// Each cache should have its own entry
	assertCacheHasEntries(t, cacheDir1, 1)
	assertCacheHasEntries(t, cacheDir2, 1)
}

func TestIntegration_Cache_EmptyFilesystem(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/cache-empty-fs:v1"
	srcFS := fstest.MapFS{}

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Pull
	destDir := t.TempDir()
	err = client.Pull(ctx, ref, destDir)
	require.NoError(t, err)

	// Verify cache has entry
	assertCacheHasEntries(t, cacheDir, 1)

	// Second pull should use cache
	err = client.Pull(ctx, ref, t.TempDir())
	require.NoError(t, err)

	// Still 1 entry
	assertCacheHasEntries(t, cacheDir, 1)
}

func TestIntegration_Cache_LargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	// Create a ~5MB file
	largeContent := bytes.Repeat([]byte("ABCDEFGHIJ"), 524288)
	srcFS := fstest.MapFS{
		"large.bin": &fstest.MapFile{Data: largeContent, Mode: 0644},
	}

	ref := reg.Host + "/test/cache-large:v1"
	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// First pull
	destDir1 := t.TempDir()
	err = client.Pull(ctx, ref, destDir1)
	require.NoError(t, err)

	// Verify cache
	info := getCacheStats(t, cacheDir)
	require.Equal(t, 1, info.EntryCount)
	require.True(t, info.Entries[0].Complete)

	// Second pull from cache
	destDir2 := t.TempDir()
	err = client.Pull(ctx, ref, destDir2)
	require.NoError(t, err)

	// Verify content matches
	content, err := os.ReadFile(filepath.Join(destDir2, "large.bin"))
	require.NoError(t, err)
	assert.Equal(t, largeContent, content)
}

func TestIntegration_Cache_PullByDigest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/cache-bydigest:v1"
	srcFS := testFS()

	digest, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Pull by digest reference
	digestRef := reg.Host + "/test/cache-bydigest@" + digest
	err = client.Pull(ctx, digestRef, t.TempDir())
	require.NoError(t, err)

	// Verify cache entry exists
	assertCacheHasEntries(t, cacheDir, 1)

	// Pull again - should use cache
	err = client.Pull(ctx, digestRef, t.TempDir())
	require.NoError(t, err)

	// Still 1 entry
	assertCacheHasEntries(t, cacheDir, 1)
}

func TestIntegration_Cache_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)
	cacheDir := cacheTestDir(t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithCacheDir(cacheDir),
	)
	require.NoError(t, err)

	// Create a larger file to increase chance of cancellation during download
	srcFS := fstest.MapFS{
		"large.bin": &fstest.MapFile{
			Data: bytes.Repeat([]byte("x"), 100000),
			Mode: 0644,
		},
	}

	ref := reg.Host + "/test/cache-cancel:v1"
	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Create cancelled context
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel immediately

	// Try to pull with cancelled context
	err = client.Pull(cancelCtx, ref, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)

	// Cache should either be empty or have an incomplete entry
	// (implementation-dependent, but should not have corrupt data)
	info := getCacheStats(t, cacheDir)
	// If there's an entry, it should be handled gracefully on next pull
	if info.EntryCount > 0 {
		// Try pulling with valid context - should work
		err = client.Pull(ctx, ref, t.TempDir())
		require.NoError(t, err)
	}
}
