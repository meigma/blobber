package blobber_test

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"log/slog"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilmanlab/blobber"
	"github.com/gilmanlab/blobber/internal/archive"
	"github.com/gilmanlab/blobber/internal/safepath"
)

// buildTestBlob creates an eStargz blob from the given filesystem for testing.
func buildTestBlob(t *testing.T, fsys fstest.MapFS) (data []byte, size int64) {
	t.Helper()

	builder := archive.NewBuilder(nil)
	result, err := builder.Build(context.Background(), fsys, blobber.GzipCompression())
	require.NoError(t, err, "Build() failed")
	defer result.Blob.Close()

	data, err = io.ReadAll(result.Blob)
	require.NoError(t, err, "failed to read blob")

	size = int64(len(data))
	return data, size
}

func TestImageList(t *testing.T) {
	t.Parallel()

	testFS := fstest.MapFS{
		"config.yaml":    &fstest.MapFile{Data: []byte("key: value"), Mode: 0o644},
		"data/file1.txt": &fstest.MapFile{Data: []byte("file1"), Mode: 0o644},
		"data/file2.txt": &fstest.MapFile{Data: []byte("file2"), Mode: 0o644},
		"data":           &fstest.MapFile{Mode: fs.ModeDir | 0o755},
	}

	data, size := buildTestBlob(t, testFS)

	img, err := blobber.NewImageFromBlob("test:latest", bytes.NewReader(data), size, safepath.NewValidator(), slog.New(slog.DiscardHandler))
	require.NoError(t, err, "NewImageFromBlob() failed")
	defer img.Close()

	entries, err := img.List()
	require.NoError(t, err, "List() failed")
	assert.NotEmpty(t, entries, "List() returned no entries")

	// Check for expected files
	found := make(map[string]bool)
	for _, e := range entries {
		found[e.Path()] = true
	}

	expectedFiles := []string{"config.yaml", "data/file1.txt", "data/file2.txt"}
	for _, f := range expectedFiles {
		assert.True(t, found[f], "expected file %q not found in List() results", f)
	}
}

func TestImageOpen(t *testing.T) {
	t.Parallel()

	expectedContent := "hello from test file"
	testFS := fstest.MapFS{
		"test.txt": &fstest.MapFile{Data: []byte(expectedContent), Mode: 0o644},
	}

	data, size := buildTestBlob(t, testFS)

	img, err := blobber.NewImageFromBlob("test:latest", bytes.NewReader(data), size, safepath.NewValidator(), slog.New(slog.DiscardHandler))
	require.NoError(t, err, "NewImageFromBlob() failed")
	defer img.Close()

	rc, err := img.Open("test.txt")
	require.NoError(t, err, "Open() failed")
	defer rc.Close()

	content, err := io.ReadAll(rc)
	require.NoError(t, err, "failed to read file")

	assert.Equal(t, expectedContent, string(content))
}

func TestImageOpenNotFound(t *testing.T) {
	t.Parallel()

	testFS := fstest.MapFS{
		"exists.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
	}

	data, size := buildTestBlob(t, testFS)

	img, err := blobber.NewImageFromBlob("test:latest", bytes.NewReader(data), size, safepath.NewValidator(), slog.New(slog.DiscardHandler))
	require.NoError(t, err, "NewImageFromBlob() failed")
	defer img.Close()

	_, err = img.Open("does-not-exist.txt")
	require.Error(t, err, "Open() should return error for non-existent file")
	assert.ErrorIs(t, err, blobber.ErrNotFound)
}

func TestImageWalk(t *testing.T) {
	t.Parallel()

	testFS := fstest.MapFS{
		"a.txt":     &fstest.MapFile{Data: []byte("a"), Mode: 0o644},
		"dir":       &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		"dir/b.txt": &fstest.MapFile{Data: []byte("b"), Mode: 0o644},
		"dir/c.txt": &fstest.MapFile{Data: []byte("c"), Mode: 0o644},
	}

	data, size := buildTestBlob(t, testFS)

	img, err := blobber.NewImageFromBlob("test:latest", bytes.NewReader(data), size, safepath.NewValidator(), slog.New(slog.DiscardHandler))
	require.NoError(t, err, "NewImageFromBlob() failed")
	defer img.Close()

	var paths []string
	err = img.Walk(func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	require.NoError(t, err, "Walk() failed")
	assert.NotEmpty(t, paths, "Walk() visited no paths")

	// Verify some expected paths exist
	pathSet := make(map[string]bool)
	for _, p := range paths {
		pathSet[p] = true
	}

	for _, expected := range []string{"a.txt", "dir/b.txt"} {
		assert.True(t, pathSet[expected], "Walk() did not visit expected path %q", expected)
	}
}

func TestImageClose(t *testing.T) {
	t.Parallel()

	testFS := fstest.MapFS{
		"test.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
	}

	data, size := buildTestBlob(t, testFS)

	img, err := blobber.NewImageFromBlob("test:latest", bytes.NewReader(data), size, safepath.NewValidator(), slog.New(slog.DiscardHandler))
	require.NoError(t, err, "NewImageFromBlob() failed")

	// Close the image
	require.NoError(t, img.Close(), "Close() failed")

	// Operations should fail after close
	_, err = img.List()
	assert.Error(t, err, "List() should return error after Close()")

	_, err = img.Open("test.txt")
	assert.Error(t, err, "Open() should return error after Close()")

	err = img.Walk(func(path string, d fs.DirEntry, err error) error { return nil })
	assert.Error(t, err, "Walk() should return error after Close()")

	// Double close should be safe
	assert.NoError(t, img.Close(), "second Close() returned error")
}

func TestImageConcurrentAccess(t *testing.T) {
	t.Parallel()

	testFS := fstest.MapFS{
		"file1.txt": &fstest.MapFile{Data: []byte("content1"), Mode: 0o644},
		"file2.txt": &fstest.MapFile{Data: []byte("content2"), Mode: 0o644},
		"file3.txt": &fstest.MapFile{Data: []byte("content3"), Mode: 0o644},
	}

	data, size := buildTestBlob(t, testFS)

	img, err := blobber.NewImageFromBlob("test:latest", bytes.NewReader(data), size, safepath.NewValidator(), slog.New(slog.DiscardHandler))
	require.NoError(t, err, "NewImageFromBlob() failed")
	defer img.Close()

	// Run concurrent operations
	var wg sync.WaitGroup
	errCh := make(chan error, 30)

	// 10 concurrent List calls
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := img.List()
			if err != nil {
				errCh <- err
			}
		}()
	}

	// 10 concurrent Open calls
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rc, err := img.Open("file1.txt")
			if err != nil {
				errCh <- err
				return
			}
			rc.Close()
		}()
	}

	// 10 concurrent Walk calls
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := img.Walk(func(path string, d fs.DirEntry, err error) error {
				return nil
			})
			if err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		assert.NoError(t, err, "concurrent operation failed")
	}
}

func TestImageMultipleOpens(t *testing.T) {
	t.Parallel()

	testFS := fstest.MapFS{
		"a.txt": &fstest.MapFile{Data: []byte("aaa"), Mode: 0o644},
		"b.txt": &fstest.MapFile{Data: []byte("bbb"), Mode: 0o644},
	}

	data, size := buildTestBlob(t, testFS)

	img, err := blobber.NewImageFromBlob("test:latest", bytes.NewReader(data), size, safepath.NewValidator(), slog.New(slog.DiscardHandler))
	require.NoError(t, err, "NewImageFromBlob() failed")
	defer img.Close()

	// Open multiple files - should all work
	rc1, err := img.Open("a.txt")
	require.NoError(t, err, "Open(a.txt) failed")
	defer rc1.Close()

	rc2, err := img.Open("b.txt")
	require.NoError(t, err, "Open(b.txt) failed")
	defer rc2.Close()

	// Read from both
	content1, err := io.ReadAll(rc1)
	require.NoError(t, err, "failed to read a.txt")

	content2, err := io.ReadAll(rc2)
	require.NoError(t, err, "failed to read b.txt")

	assert.Equal(t, "aaa", string(content1), "a.txt content mismatch")
	assert.Equal(t, "bbb", string(content2), "b.txt content mismatch")
}
