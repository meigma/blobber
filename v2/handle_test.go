package blobber

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/v2/internal/estargz"
)

// buildTestBlob creates an estargz archive from the given filesystem.
func buildTestBlob(t *testing.T, src fs.FS) []byte {
	t.Helper()

	var buf bytes.Buffer
	b := estargz.NewBuilder()
	_, err := b.Build(context.Background(), &buf, src)
	require.NoError(t, err)

	return buf.Bytes()
}

// writeTestBlob writes an estargz archive to a temp file.
func writeTestBlob(t *testing.T, data []byte) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "blob-*.estargz")
	require.NoError(t, err)

	_, err = f.Write(data)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	return f.Name()
}

func TestLocalHandle_Open(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt":       &fstest.MapFile{Data: []byte("hello world"), Mode: 0o644},
		"dir":            &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		"dir/nested.txt": &fstest.MapFile{Data: []byte("nested content"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)
	tempPath := writeTestBlob(t, blob)

	handle, err := newLocalHandle(context.Background(), tempPath)
	require.NoError(t, err)
	t.Cleanup(func() { handle.Close() })

	t.Run("open regular file", func(t *testing.T) {
		t.Parallel()

		f, err := handle.Open("file.txt")
		require.NoError(t, err)
		defer f.Close()

		content, err := io.ReadAll(f)
		require.NoError(t, err)
		assert.Equal(t, "hello world", string(content))
	})

	t.Run("open nested file", func(t *testing.T) {
		t.Parallel()

		f, err := handle.Open("dir/nested.txt")
		require.NoError(t, err)
		defer f.Close()

		content, err := io.ReadAll(f)
		require.NoError(t, err)
		assert.Equal(t, "nested content", string(content))
	})

	t.Run("open directory", func(t *testing.T) {
		t.Parallel()

		f, err := handle.Open("dir")
		require.NoError(t, err)
		defer f.Close()

		info, err := f.Stat()
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("open nonexistent file", func(t *testing.T) {
		t.Parallel()

		_, err := handle.Open("nonexistent.txt")
		require.Error(t, err)
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})
}

func TestLocalHandle_Stat(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
		"dir":      &fstest.MapFile{Mode: fs.ModeDir | 0o755},
	}

	blob := buildTestBlob(t, src)
	tempPath := writeTestBlob(t, blob)

	handle, err := newLocalHandle(context.Background(), tempPath)
	require.NoError(t, err)
	t.Cleanup(func() { handle.Close() })

	t.Run("stat file", func(t *testing.T) {
		t.Parallel()

		info, err := handle.Stat("file.txt")
		require.NoError(t, err)
		assert.Equal(t, "file.txt", info.Name())
		assert.False(t, info.IsDir())
	})

	t.Run("stat directory", func(t *testing.T) {
		t.Parallel()

		info, err := handle.Stat("dir")
		require.NoError(t, err)
		assert.Equal(t, "dir", info.Name())
		assert.True(t, info.IsDir())
	})
}

func TestLocalHandle_ReadDir(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"a.txt": &fstest.MapFile{Data: []byte("a"), Mode: 0o644},
		"b.txt": &fstest.MapFile{Data: []byte("b"), Mode: 0o644},
		"dir":   &fstest.MapFile{Mode: fs.ModeDir | 0o755},
	}

	blob := buildTestBlob(t, src)
	tempPath := writeTestBlob(t, blob)

	handle, err := newLocalHandle(context.Background(), tempPath)
	require.NoError(t, err)
	t.Cleanup(func() { handle.Close() })

	entries, err := handle.ReadDir(".")
	require.NoError(t, err)
	require.Len(t, entries, 3)

	// Entries should be sorted.
	assert.Equal(t, "a.txt", entries[0].Name())
	assert.Equal(t, "b.txt", entries[1].Name())
	assert.Equal(t, "dir", entries[2].Name())
}

func TestLocalHandle_Close(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)
	tempPath := writeTestBlob(t, blob)

	handle, err := newLocalHandle(context.Background(), tempPath)
	require.NoError(t, err)

	// Temp file exists before close.
	_, err = os.Stat(tempPath)
	require.NoError(t, err)

	// Close should delete the temp file.
	require.NoError(t, handle.Close())

	// Temp file should be gone.
	_, err = os.Stat(tempPath)
	require.True(t, os.IsNotExist(err))
}

func TestNetworkHandle_Open(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("hello network"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)

	handle, err := newNetworkHandle(
		context.Background(),
		newMockBlobReader(blob),
		func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(blob)), nil
		},
	)
	require.NoError(t, err)
	t.Cleanup(func() { handle.Close() })

	f, err := handle.Open("file.txt")
	require.NoError(t, err)
	defer f.Close()

	content, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, "hello network", string(content))
}

func TestCopyTo_LocalHandle(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt":       &fstest.MapFile{Data: []byte("file content"), Mode: 0o644},
		"dir":            &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		"dir/nested.txt": &fstest.MapFile{Data: []byte("nested"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)
	tempPath := writeTestBlob(t, blob)

	handle, err := newLocalHandle(context.Background(), tempPath)
	require.NoError(t, err)
	defer handle.Close()

	dest := t.TempDir()
	require.NoError(t, CopyTo(handle, dest))

	// Verify extracted files.
	content, err := os.ReadFile(filepath.Join(dest, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "file content", string(content))

	content, err = os.ReadFile(filepath.Join(dest, "dir", "nested.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested", string(content))
}

func TestCopyTo_NetworkHandle(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"config.yaml": &fstest.MapFile{Data: []byte("key: value"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)

	handle, err := newNetworkHandle(
		context.Background(),
		newMockBlobReader(blob),
		func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(blob)), nil
		},
	)
	require.NoError(t, err)
	defer handle.Close()

	dest := t.TempDir()
	require.NoError(t, CopyTo(handle, dest))

	content, err := os.ReadFile(filepath.Join(dest, "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "key: value", string(content))
}

// mockBlobReader implements the blobSource interface for testing.
type mockBlobReader struct {
	*bytes.Reader
	size   int64
	closed bool
}

func newMockBlobReader(data []byte) *mockBlobReader {
	return &mockBlobReader{
		Reader: bytes.NewReader(data),
		size:   int64(len(data)),
	}
}

func (m *mockBlobReader) Size() int64 {
	return m.size
}

func (m *mockBlobReader) Close() error {
	m.closed = true
	return nil
}
