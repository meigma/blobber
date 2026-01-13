//go:build integration

package integration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/v2"
	"github.com/meigma/blobber/v2/internal/testutils"
)

func TestPushPull_Gzip(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()

	client := blobber.NewClient(blobber.WithPlainHTTP(true))

	// Push.
	result, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Reference)
	assert.True(t, strings.HasPrefix(result.Manifest.Digest.String(), "sha256:"))
	assert.Greater(t, result.Manifest.Blob.Size, int64(0))

	// Pull.
	handle, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { handle.Close() })

	// Extract and verify.
	destDir := t.TempDir()
	err = blobber.CopyTo(handle, destDir)
	require.NoError(t, err)

	testutils.AssertFilesMatch(t, srcFS, destDir)
}

func TestPushPull_Zstd(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()

	client := blobber.NewClient(blobber.WithPlainHTTP(true))

	// Push with zstd compression.
	result, err := client.Push(ctx, ref, srcFS, blobber.WithCompression(blobber.ZstdCompression()))
	require.NoError(t, err)
	assert.NotEmpty(t, result.Reference)

	// Pull.
	handle, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { handle.Close() })

	// Extract and verify.
	destDir := t.TempDir()
	err = blobber.CopyTo(handle, destDir)
	require.NoError(t, err)

	testutils.AssertFilesMatch(t, srcFS, destDir)
}

func TestPushPull_LargeFiles(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.LargeTestFS(50, 1024) // 50 files, 1KB each

	client := blobber.NewClient(blobber.WithPlainHTTP(true))

	// Push.
	result, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Reference)

	// Pull.
	handle, err := client.Pull(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { handle.Close() })

	// Extract and verify.
	destDir := t.TempDir()
	err = blobber.CopyTo(handle, destDir)
	require.NoError(t, err)

	testutils.AssertFilesMatch(t, srcFS, destDir)
}

func TestPushPull_ByDigest(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()

	client := blobber.NewClient(blobber.WithPlainHTTP(true))

	// Push.
	result, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Pull by digest reference (result.Reference is already a digest ref).
	handle, err := client.Pull(ctx, result.Reference)
	require.NoError(t, err)
	t.Cleanup(func() { handle.Close() })

	// Extract and verify.
	destDir := t.TempDir()
	err = blobber.CopyTo(handle, destDir)
	require.NoError(t, err)

	testutils.AssertFilesMatch(t, srcFS, destDir)
}

func TestStream_SingleFile(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()

	client := blobber.NewClient(blobber.WithPlainHTTP(true))

	// Push.
	_, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Stream.
	handle, err := client.Stream(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { handle.Close() })

	// Read a single file via Open().
	testutils.AssertFSContains(t, handle, "hello.txt", []byte("Hello, World!"))
}

func TestStream_ReadDir(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()

	client := blobber.NewClient(blobber.WithPlainHTTP(true))

	// Push.
	_, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Stream.
	handle, err := client.Stream(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { handle.Close() })

	// List root directory.
	entries, err := handle.ReadDir(".")
	require.NoError(t, err)
	assert.Len(t, entries, 3) // hello.txt, subdir, binary.bin

	// List subdirectory.
	subEntries, err := handle.ReadDir("subdir")
	require.NoError(t, err)
	assert.Len(t, subEntries, 1)
	assert.Equal(t, "nested.txt", subEntries[0].Name())
}

func TestStream_CopyTo(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()

	client := blobber.NewClient(blobber.WithPlainHTTP(true))

	// Push.
	_, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Stream.
	handle, err := client.Stream(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { handle.Close() })

	// CopyTo extracts all files (uses fetchFull optimization).
	destDir := t.TempDir()
	err = blobber.CopyTo(handle, destDir)
	require.NoError(t, err)

	testutils.AssertFilesMatch(t, srcFS, destDir)
}

func TestStream_Zstd(t *testing.T) {
	ctx := testutils.TestContext(t)
	ref := registry.TestRef(t, "blob")
	srcFS := testutils.TestFS()

	client := blobber.NewClient(blobber.WithPlainHTTP(true))

	// Push with zstd.
	_, err := client.Push(ctx, ref, srcFS, blobber.WithCompression(blobber.ZstdCompression()))
	require.NoError(t, err)

	// Stream.
	handle, err := client.Stream(ctx, ref)
	require.NoError(t, err)
	t.Cleanup(func() { handle.Close() })

	// Read single file.
	testutils.AssertFSContains(t, handle, "hello.txt", []byte("Hello, World!"))

	// CopyTo.
	destDir := t.TempDir()
	err = blobber.CopyTo(handle, destDir)
	require.NoError(t, err)

	testutils.AssertFilesMatch(t, srcFS, destDir)
}
