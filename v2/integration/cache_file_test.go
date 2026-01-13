//go:build integration

package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/v2"
	"github.com/meigma/blobber/v2/internal/testutils"
)

func TestFileCache_PullTwice(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithFileCache(cacheDir),
	)

	// Push.
	result, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// First pull - populates file cache.
	handle1, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { handle1.Close() })

	// Verify cache directory was created.
	blobCacheDir := fileCacheDir(cacheDir, result.Manifest.Blob.Digest.String())
	_, err = os.Stat(blobCacheDir)
	require.NoError(t, err, "blob cache directory should exist")

	// Verify cache.json exists (marks complete).
	cacheJSON := filepath.Join(blobCacheDir, "cache.json")
	_, err = os.Stat(cacheJSON)
	require.NoError(t, err, "cache.json should exist")

	// Verify extracted files exist in cache.
	_, err = os.Stat(filepath.Join(blobCacheDir, "hello.txt"))
	require.NoError(t, err, "hello.txt should be cached")

	// Second pull - should serve from cache.
	handle2, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { handle2.Close() })

	// Verify contents are correct.
	testutils.AssertFSContains(t, handle2, "hello.txt", []byte("Hello, World!"))
}

func TestFileCache_PullServesFromCache(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithFileCache(cacheDir),
	)

	// Push.
	result, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// First pull.
	handle1, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	handle1.Close()

	// Modify a cached file to prove second pull reads from cache.
	blobCacheDir := fileCacheDir(cacheDir, result.Manifest.Blob.Digest.String())
	cachedFile := filepath.Join(blobCacheDir, "hello.txt")
	err = os.WriteFile(cachedFile, []byte("Modified!"), 0o644)
	require.NoError(t, err)

	// Second pull - should serve modified content from cache.
	handle2, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { handle2.Close() })

	// Verify we get the modified content (proving cache is used).
	testutils.AssertFSContains(t, handle2, "hello.txt", []byte("Modified!"))
}

func TestFileCache_Stream_OnDemandCaching(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithFileCache(cacheDir),
	)

	// Push.
	result, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Stream and read a single file fully.
	handle, err := client.Stream(ctx, ref)
	require.NoError(t, err)

	// Read hello.txt fully to trigger caching.
	f, err := handle.Open("hello.txt")
	require.NoError(t, err)
	content, err := readAll(f)
	require.NoError(t, err)
	f.Close()
	handle.Close()

	assert.Equal(t, "Hello, World!", string(content))

	// Check if file was cached.
	// Note: On-demand caching only occurs if the file is fully read.
	blobCacheDir := fileCacheDir(cacheDir, result.Manifest.Blob.Digest.String())
	cachedFile := filepath.Join(blobCacheDir, "hello.txt")

	// The file should be cached after full read.
	_, err = os.Stat(cachedFile)
	assert.NoError(t, err, "file should be cached after full read")
}

func TestFileCache_StreamServesFromCache(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithFileCache(cacheDir),
	)

	// Push.
	result, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Stream and read a single file fully to cache it.
	handle1, err := client.Stream(ctx, ref)
	require.NoError(t, err)

	f, err := handle1.Open("hello.txt")
	require.NoError(t, err)
	content, err := readAll(f)
	require.NoError(t, err)
	f.Close()
	handle1.Close()

	assert.Equal(t, "Hello, World!", string(content))

	blobCacheDir := fileCacheDir(cacheDir, result.Manifest.Blob.Digest.String())
	cachedFile := filepath.Join(blobCacheDir, "hello.txt")
	_, err = os.Stat(cachedFile)
	require.NoError(t, err, "file should be cached after full read")

	// Modify cached file to prove second stream reads from cache.
	err = os.WriteFile(cachedFile, []byte("Modified!"), 0o644)
	require.NoError(t, err)

	handle2, err := client.Stream(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { handle2.Close() })

	f2, err := handle2.Open("hello.txt")
	require.NoError(t, err)
	content2, err := readAll(f2)
	require.NoError(t, err)
	f2.Close()

	assert.Equal(t, "Modified!", string(content2))
}

func TestFileCache_PartialEntryPruned(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithFileCache(cacheDir),
	)

	// Push.
	result, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Stream and read a single file fully to create a partial cache entry.
	handle, err := client.Stream(ctx, ref)
	require.NoError(t, err)

	f, err := handle.Open("hello.txt")
	require.NoError(t, err)
	_, err = readAll(f)
	require.NoError(t, err)
	f.Close()
	handle.Close()

	blobCacheDir := fileCacheDir(cacheDir, result.Manifest.Blob.Digest.String())
	cacheJSON := filepath.Join(blobCacheDir, "cache.json")

	entry := readFileCacheEntry(t, cacheJSON)
	entry.LastAccessed = time.Now().Add(-2 * time.Hour)
	complete := false
	entry.Complete = &complete
	writeFileCacheEntry(t, cacheJSON, entry)

	err = blobber.PruneFileCache(ctx, cacheDir, blobber.PruneStrategy{
		MaxAge: 1 * time.Hour,
	})
	require.NoError(t, err)

	_, err = os.Stat(blobCacheDir)
	assert.True(t, os.IsNotExist(err), "partial cache should be removed")
}

func TestFileCache_StreamTOCCacheSkipsRangeFetch(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	var ranges []byteRange

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithFileCache(cacheDir),
		blobber.WithRangeHook(func(offset, length int64) {
			ranges = append(ranges, byteRange{offset: offset, length: length})
		}),
	)

	// Push.
	result, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// First stream forces TOC fetch and caches TOC bytes.
	handle1, err := client.Stream(ctx, ref)
	require.NoError(t, err)
	_, err = handle1.ReadDir(".")
	require.NoError(t, err)
	handle1.Close()

	// Compute TOC range from cached metadata.
	blobCacheDir := fileCacheDir(cacheDir, result.Manifest.Blob.Digest.String())
	tocEntry := readTOCCacheEntry(t, filepath.Join(blobCacheDir, "toc.json"))
	footerBytes := readFooterCacheBytes(t, filepath.Join(blobCacheDir, "footer.bin"))
	footerOffset := result.Manifest.Blob.Size - int64(len(footerBytes))

	ranges = nil

	// Second stream should reuse cached TOC.
	handle2, err := client.Stream(ctx, ref)
	require.NoError(t, err)
	_, err = handle2.ReadDir(".")
	require.NoError(t, err)
	handle2.Close()

	assert.False(t, hasRange(ranges, tocEntry.Offset, tocEntry.Size), "toc range should not be fetched")
	assert.False(t, hasRange(ranges, footerOffset, int64(len(footerBytes))), "footer range should not be fetched")
}

func TestFileCache_Stream_CopyTo(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithFileCache(cacheDir),
	)

	// Push.
	_, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Stream.
	handle, err := client.Stream(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { handle.Close() })

	// CopyTo extracts all files.
	destDir := t.TempDir()
	err = blobber.CopyTo(handle, destDir)
	require.NoError(t, err)

	// Verify extracted files match source.
	testutils.AssertFilesMatch(t, srcFS, destDir)
}

func TestFileCache_PreservesDirectoryStructure(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()
	cacheDir := t.TempDir()

	client := blobber.NewClient(
		blobber.WithPlainHTTP(true),
		blobber.WithFileCache(cacheDir),
	)

	// Push.
	result, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Pull to populate cache.
	handle, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	handle.Close()

	// Verify directory structure in cache.
	blobCacheDir := fileCacheDir(cacheDir, result.Manifest.Blob.Digest.String())

	// Check nested file exists.
	nestedFile := filepath.Join(blobCacheDir, "subdir", "nested.txt")
	_, err = os.Stat(nestedFile)
	require.NoError(t, err, "nested file should be cached")

	// Verify content.
	content, err := os.ReadFile(nestedFile)
	require.NoError(t, err)
	assert.Equal(t, "Nested content", string(content))
}

// fileCacheDir returns the cache directory for a digest.
func fileCacheDir(cacheDir, digestStr string) string {
	h := sha256.Sum256([]byte(digestStr))
	hash := hex.EncodeToString(h[:])
	return filepath.Join(cacheDir, "blobs", hash)
}

// readAll reads all content from an fs.File.
func readAll(f fs.File) ([]byte, error) {
	var content []byte
	buf := make([]byte, 1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			content = append(content, buf[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
	}
	return content, nil
}

type byteRange struct {
	offset int64
	length int64
}

func hasRange(ranges []byteRange, offset, length int64) bool {
	for _, r := range ranges {
		if r.offset == offset && r.length == length {
			return true
		}
	}
	return false
}

type fileCacheEntry struct {
	Digest       string    `json:"digest"`
	Size         int64     `json:"size"`
	LastAccessed time.Time `json:"lastAccessed"`
	Complete     *bool     `json:"complete,omitempty"`
}

type tocCacheEntry struct {
	Digest string `json:"digest"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
}

func readFileCacheEntry(t *testing.T, path string) fileCacheEntry {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var entry fileCacheEntry
	require.NoError(t, json.Unmarshal(data, &entry))
	return entry
}

func writeFileCacheEntry(t *testing.T, path string, entry fileCacheEntry) {
	t.Helper()

	data, err := json.MarshalIndent(entry, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func readTOCCacheEntry(t *testing.T, path string) tocCacheEntry {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var entry tocCacheEntry
	require.NoError(t, json.Unmarshal(data, &entry))
	return entry
}

func readFooterCacheBytes(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	return data
}
