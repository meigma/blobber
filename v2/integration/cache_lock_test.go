//go:build integration

package integration

import (
	"context"
	"os"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/v2/internal/cache"
)

func TestFileCache_PruneSkipsLockedBlob(t *testing.T) {
	cacheDir := t.TempDir()

	fileCache, err := cache.NewFileCache(cacheDir)
	require.NoError(t, err)

	blobDigest := digest.FromString("blob-lock-test")
	blobDir, err := fileCache.Dir(blobDigest)
	require.NoError(t, err)

	err = fileCache.MarkComplete(blobDigest, 10)
	require.NoError(t, err)

	locker, ok := fileCache.(cache.BlobLocker)
	require.True(t, ok, "file cache should support blob locking")

	unlock, err := locker.LockBlob(blobDigest)
	require.NoError(t, err)
	locked := true
	t.Cleanup(func() {
		if locked {
			require.NoError(t, unlock())
		}
	})

	err = cache.PruneFileCache(context.Background(), cacheDir, cache.PruneStrategy{
		MaxSize: 1,
	})
	require.NoError(t, err)

	_, err = os.Stat(blobDir)
	require.NoError(t, err, "locked blob directory should not be pruned")

	require.NoError(t, unlock())
	locked = false

	err = cache.PruneFileCache(context.Background(), cacheDir, cache.PruneStrategy{
		MaxSize: 1,
	})
	require.NoError(t, err)

	_, err = os.Stat(blobDir)
	require.Error(t, err, "blob directory should be pruned after unlock")
	require.True(t, os.IsNotExist(err))
}
