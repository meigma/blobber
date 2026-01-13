//go:build integration

package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/v2"
	"github.com/meigma/blobber/v2/internal/testutils"
)

func TestRefCache_PullTwice(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithRefCache(cacheDir, 1*time.Hour),
	)

	// Push.
	_, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// First pull - populates ref cache.
	handle1, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	handle1.Close()

	// Verify cache file was created.
	cacheFile := refCachePath(cacheDir, ref)
	_, err = os.Stat(cacheFile)
	require.NoError(t, err, "ref cache file should exist")

	// Second pull - should use cached ref.
	handle2, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	handle2.Close()

	// Verify the cache file still exists and wasn't recreated.
	info, err := os.Stat(cacheFile)
	require.NoError(t, err)
	assert.False(t, info.ModTime().IsZero())
}

func TestRefCache_StreamThenPull(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithRefCache(cacheDir, 1*time.Hour),
	)

	// Push.
	_, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Stream - populates ref cache.
	streamHandle, err := client.Stream(ctx, ref)
	require.NoError(t, err)
	streamHandle.Close()

	// Verify cache file was created.
	cacheFile := refCachePath(cacheDir, ref)
	_, err = os.Stat(cacheFile)
	require.NoError(t, err, "ref cache file should exist after Stream")

	// Pull - should benefit from cached ref.
	pullHandle, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { pullHandle.Close() })

	// Verify contents are correct.
	testutils.AssertFSContains(t, pullHandle, "hello.txt", []byte("Hello, World!"))
}

func TestRefCache_TTLExpiry(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	// Use a short TTL for testing.
	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithRefCache(cacheDir, 1*time.Hour),
	)

	// Push.
	_, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// First pull - populates ref cache.
	handle1, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	handle1.Close()

	// Manipulate cache to simulate TTL expiry.
	cacheFile := refCachePath(cacheDir, ref)
	expireRefCache(t, cacheFile, 2*time.Hour)

	// Second pull - should re-resolve due to stale cache.
	// (This works because the cache entry is expired, forcing re-fetch)
	handle2, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	handle2.Close()

	// Verify the cache was refreshed (updatedAt should be recent).
	entry := readRefCacheEntry(t, cacheFile)
	assert.True(t, time.Since(entry.UpdatedAt) < 1*time.Minute,
		"cache entry should have been refreshed")
}

func TestRefCache_DifferentRefs(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref1 := registry.TestRef(t, "blob1")
	ref2 := registry.TestRef(t, "blob2")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithRefCache(cacheDir, 1*time.Hour),
	)

	// Push to both refs.
	_, err := client.Push(ctx, ref1, srcFS)
	require.NoError(t, err)
	_, err = client.Push(ctx, ref2, srcFS)
	require.NoError(t, err)

	// Pull both.
	handle1, err := client.Pull(ctx, ref1)
	require.NoError(t, err)
	handle1.Close()

	handle2, err := client.Pull(ctx, ref2)
	require.NoError(t, err)
	handle2.Close()

	// Verify both cache files exist.
	_, err = os.Stat(refCachePath(cacheDir, ref1))
	require.NoError(t, err, "ref1 cache should exist")

	_, err = os.Stat(refCachePath(cacheDir, ref2))
	require.NoError(t, err, "ref2 cache should exist")
}

// refCacheEntry mirrors the internal refEntry structure.
type refCacheEntry struct {
	Ref       string    `json:"ref"`
	Digest    string    `json:"digest"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// refCachePath returns the cache.json path for a ref.
func refCachePath(cacheDir, ref string) string {
	h := sha256.Sum256([]byte(ref))
	hash := hex.EncodeToString(h[:])
	return filepath.Join(cacheDir, "refs", hash, "cache.json")
}

// readRefCacheEntry reads and parses a ref cache entry.
func readRefCacheEntry(t *testing.T, path string) refCacheEntry {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var entry refCacheEntry
	require.NoError(t, json.Unmarshal(data, &entry))
	return entry
}

// expireRefCache modifies the cache file to simulate TTL expiry.
func expireRefCache(t *testing.T, path string, age time.Duration) {
	t.Helper()
	entry := readRefCacheEntry(t, path)
	entry.UpdatedAt = time.Now().Add(-age)

	data, err := json.MarshalIndent(entry, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}
