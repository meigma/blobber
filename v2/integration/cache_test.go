//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/v2"
	"github.com/meigma/blobber/v2/internal/testutils"
)

func TestCache_FullWorkflow(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithRefCache(cacheDir, 1*time.Hour),
		blobber.WithFileCache(cacheDir),
	)

	// Push.
	result, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// First pull - populates both caches.
	handle1, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	handle1.Close()

	// Verify ref cache exists.
	refCacheFile := refCachePath(cacheDir, ref)
	_, err = os.Stat(refCacheFile)
	require.NoError(t, err, "ref cache should exist")

	// Verify file cache exists.
	blobCacheDir := fileCacheDir(cacheDir, result.Manifest.Blob.Digest.String())
	_, err = os.Stat(filepath.Join(blobCacheDir, "cache.json"))
	require.NoError(t, err, "file cache should exist")

	// Second pull - full cache hit (both ref and files).
	handle2, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { handle2.Close() })

	// Verify contents.
	testutils.AssertFSContains(t, handle2, "hello.txt", []byte("Hello, World!"))
}

func TestCache_StreamPopulatesRefCache_PullUsesFileCache(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithRefCache(cacheDir, 1*time.Hour),
		blobber.WithFileCache(cacheDir),
	)

	// Push.
	_, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Stream - populates ref cache but not file cache (unless files are read).
	streamHandle, err := client.Stream(ctx, ref)
	require.NoError(t, err)
	streamHandle.Close()

	// Verify ref cache exists.
	refCacheFile := refCachePath(cacheDir, ref)
	_, err = os.Stat(refCacheFile)
	require.NoError(t, err, "ref cache should exist after Stream")

	// Pull - uses ref cache, populates file cache.
	pullHandle, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { pullHandle.Close() })

	// Verify contents.
	testutils.AssertFSContains(t, pullHandle, "hello.txt", []byte("Hello, World!"))
}

func TestCache_RefCacheExpiry_FileCacheStillValid(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithRefCache(cacheDir, 1*time.Hour),
		blobber.WithFileCache(cacheDir),
	)

	// Push.
	result, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// First pull - populates both caches.
	handle1, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	handle1.Close()

	// Expire the ref cache.
	refCacheFile := refCachePath(cacheDir, ref)
	expireRefCache(t, refCacheFile, 2*time.Hour)

	// Modify file cache to prove it's still used.
	blobCacheDir := fileCacheDir(cacheDir, result.Manifest.Blob.Digest.String())
	cachedFile := filepath.Join(blobCacheDir, "hello.txt")
	err = os.WriteFile(cachedFile, []byte("From file cache!"), 0o644)
	require.NoError(t, err)

	// Second pull - ref cache is stale (re-resolves), but file cache is valid.
	handle2, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { handle2.Close() })

	// Should get content from file cache (even though ref was re-resolved).
	testutils.AssertFSContains(t, handle2, "hello.txt", []byte("From file cache!"))
}

func TestCache_SeparateCacheDirectories(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	refCacheDir := t.TempDir()
	fileCacheDir := t.TempDir()

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithRefCache(refCacheDir, 1*time.Hour),
		blobber.WithFileCache(fileCacheDir),
	)

	// Push and pull.
	_, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	handle, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	handle.Close()

	// Verify ref cache is in refCacheDir.
	refCacheFile := refCachePath(refCacheDir, ref)
	_, err = os.Stat(refCacheFile)
	require.NoError(t, err, "ref cache should be in refCacheDir")

	// Verify ref cache is NOT in fileCacheDir.
	wrongRefPath := refCachePath(fileCacheDir, ref)
	_, err = os.Stat(wrongRefPath)
	require.True(t, os.IsNotExist(err), "ref cache should not be in fileCacheDir")

	// Verify file cache is in fileCacheDir (blobs subdir exists).
	entries, err := os.ReadDir(filepath.Join(fileCacheDir, "blobs"))
	require.NoError(t, err)
	require.NotEmpty(t, entries, "file cache should have entries")
}
