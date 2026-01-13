package estargz

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sizedReader wraps a bytes.Reader to implement SizedReaderAt.
type sizedReader struct {
	*bytes.Reader
	size int64
}

func newSizedReader(data []byte) *sizedReader {
	return &sizedReader{
		Reader: bytes.NewReader(data),
		size:   int64(len(data)),
	}
}

func (sr *sizedReader) Size() int64 { return sr.size }

type countingReader struct {
	*sizedReader
	mu    sync.Mutex
	reads int
}

func (cr *countingReader) ReadAt(p []byte, off int64) (int, error) {
	cr.mu.Lock()
	cr.reads++
	cr.mu.Unlock()
	return cr.sizedReader.ReadAt(p, off)
}

func (cr *countingReader) ReadCount() int {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	return cr.reads
}

func TestReader_Open(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt":       &fstest.MapFile{Data: []byte("hello world"), Mode: 0o644},
		"dir":            &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		"dir/nested.txt": &fstest.MapFile{Data: []byte("nested content"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)
	reader, err := NewReader(context.Background(), newSizedReader(blob))
	require.NoError(t, err)
	t.Cleanup(func() { reader.Close() })

	t.Run("open regular file", func(t *testing.T) {
		t.Parallel()

		f, err := reader.Open("file.txt")
		require.NoError(t, err)
		defer f.Close()

		content, err := io.ReadAll(f)
		require.NoError(t, err)
		assert.Equal(t, "hello world", string(content))
	})

	t.Run("open nested file", func(t *testing.T) {
		t.Parallel()

		f, err := reader.Open("dir/nested.txt")
		require.NoError(t, err)
		defer f.Close()

		content, err := io.ReadAll(f)
		require.NoError(t, err)
		assert.Equal(t, "nested content", string(content))
	})

	t.Run("open directory", func(t *testing.T) {
		t.Parallel()

		f, err := reader.Open("dir")
		require.NoError(t, err)
		defer f.Close()

		info, err := f.Stat()
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("open nonexistent file", func(t *testing.T) {
		t.Parallel()

		_, err := reader.Open("nonexistent.txt")
		require.Error(t, err)
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})
}

func TestLazyReader_DefersTOC(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("lazy content"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)
	counter := &countingReader{sizedReader: newSizedReader(blob)}

	reader := NewLazyReader(context.Background(), counter)

	assert.Equal(t, 0, counter.ReadCount(), "lazy reader should not read before access")

	info, err := reader.Stat("file.txt")
	require.NoError(t, err)
	assert.Equal(t, "file.txt", info.Name())
	assert.Greater(t, counter.ReadCount(), 0, "lazy reader should read on first access")
}

func TestReader_Stat(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
		"dir":      &fstest.MapFile{Mode: fs.ModeDir | 0o755},
	}

	blob := buildTestBlob(t, src)
	reader, err := NewReader(context.Background(), newSizedReader(blob))
	require.NoError(t, err)
	t.Cleanup(func() { reader.Close() })

	t.Run("stat regular file", func(t *testing.T) {
		t.Parallel()

		info, err := reader.Stat("file.txt")
		require.NoError(t, err)
		assert.Equal(t, "file.txt", info.Name())
		assert.Equal(t, int64(7), info.Size())
		assert.False(t, info.IsDir())
	})

	t.Run("stat directory", func(t *testing.T) {
		t.Parallel()

		info, err := reader.Stat("dir")
		require.NoError(t, err)
		assert.Equal(t, "dir", info.Name())
		assert.True(t, info.IsDir())
	})

	t.Run("stat nonexistent", func(t *testing.T) {
		t.Parallel()

		_, err := reader.Stat("nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})
}

func TestReader_ReadDir(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"a.txt":        &fstest.MapFile{Data: []byte("a"), Mode: 0o644},
		"b.txt":        &fstest.MapFile{Data: []byte("b"), Mode: 0o644},
		"subdir":       &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		"subdir/c.txt": &fstest.MapFile{Data: []byte("c"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)
	reader, err := NewReader(context.Background(), newSizedReader(blob))
	require.NoError(t, err)
	t.Cleanup(func() { reader.Close() })

	t.Run("read root directory", func(t *testing.T) {
		t.Parallel()

		entries, err := reader.ReadDir(".")
		require.NoError(t, err)

		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}

		assert.Contains(t, names, "a.txt")
		assert.Contains(t, names, "b.txt")
		assert.Contains(t, names, "subdir")
	})

	t.Run("read subdirectory", func(t *testing.T) {
		t.Parallel()

		entries, err := reader.ReadDir("subdir")
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, "c.txt", entries[0].Name())
	})

	t.Run("read nonexistent directory", func(t *testing.T) {
		t.Parallel()

		_, err := reader.ReadDir("nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("read file as directory", func(t *testing.T) {
		t.Parallel()

		_, err := reader.ReadDir("a.txt")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})
}

func TestReader_FileSeek(t *testing.T) {
	t.Parallel()

	content := "hello world, this is a test file"
	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte(content), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)
	reader, err := NewReader(context.Background(), newSizedReader(blob))
	require.NoError(t, err)
	defer reader.Close()

	f, err := reader.Open("file.txt")
	require.NoError(t, err)
	defer f.Close()

	seeker, ok := f.(io.Seeker)
	require.True(t, ok, "file should implement io.Seeker")

	// Read first 5 bytes.
	buf := make([]byte, 5)
	n, err := f.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(buf))

	// Seek to position 6.
	pos, err := seeker.Seek(6, io.SeekStart)
	require.NoError(t, err)
	assert.Equal(t, int64(6), pos)

	// Read from new position.
	n, err = f.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "world", string(buf))

	// Seek relative to current.
	pos, err = seeker.Seek(2, io.SeekCurrent)
	require.NoError(t, err)
	assert.Equal(t, int64(13), pos)

	// Seek from end.
	pos, err = seeker.Seek(-4, io.SeekEnd)
	require.NoError(t, err)
	assert.Equal(t, int64(len(content)-4), pos)
}

func TestReader_DirReadDir(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"dir":       &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		"dir/a.txt": &fstest.MapFile{Data: []byte("a"), Mode: 0o644},
		"dir/b.txt": &fstest.MapFile{Data: []byte("b"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)
	reader, err := NewReader(context.Background(), newSizedReader(blob))
	require.NoError(t, err)
	defer reader.Close()

	f, err := reader.Open("dir")
	require.NoError(t, err)
	defer f.Close()

	dirReader, ok := f.(fs.ReadDirFile)
	require.True(t, ok, "directory should implement fs.ReadDirFile")

	// Read one entry at a time.
	entries1, err := dirReader.ReadDir(1)
	require.NoError(t, err)
	assert.Len(t, entries1, 1)

	entries2, err := dirReader.ReadDir(1)
	require.NoError(t, err)
	assert.Len(t, entries2, 1)

	// Should return EOF when no more entries.
	_, err = dirReader.ReadDir(1)
	assert.ErrorIs(t, err, io.EOF)
}

func TestReader_RootDirReadDir(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"a.txt": &fstest.MapFile{Data: []byte("a"), Mode: 0o644},
		"b.txt": &fstest.MapFile{Data: []byte("b"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)
	reader, err := NewReader(context.Background(), newSizedReader(blob))
	require.NoError(t, err)
	defer reader.Close()

	// Open root directory with "."
	f, err := reader.Open(".")
	require.NoError(t, err)
	defer f.Close()

	dirReader, ok := f.(fs.ReadDirFile)
	require.True(t, ok, "root directory should implement fs.ReadDirFile")

	// ReadDir on root should work.
	entries, err := dirReader.ReadDir(-1)
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	// Verify Stat returns "." as the name.
	info, err := f.Stat()
	require.NoError(t, err)
	assert.Equal(t, ".", info.Name())
}

func TestReader_ContextCancellation(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)

	ctx, cancel := context.WithCancel(context.Background())
	reader, err := NewReader(ctx, newSizedReader(blob))
	require.NoError(t, err)
	defer reader.Close()

	// Cancel the context.
	cancel()

	// Operations should fail with context error.
	_, err = reader.Open("file.txt")
	assert.ErrorIs(t, err, context.Canceled)

	_, err = reader.Stat("file.txt")
	assert.ErrorIs(t, err, context.Canceled)

	_, err = reader.ReadDir(".")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestReader_EmptyArchive(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{}

	blob := buildTestBlob(t, src)
	reader, err := NewReader(context.Background(), newSizedReader(blob))
	require.NoError(t, err)
	defer reader.Close()

	// ReadDir on empty archive should return empty list, not error.
	entries, err := reader.ReadDir(".")
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestReader_InvalidPaths(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)
	reader, err := NewReader(context.Background(), newSizedReader(blob))
	require.NoError(t, err)
	t.Cleanup(func() { reader.Close() })

	invalidPaths := []string{
		"/absolute",  // starts with /
		"../parent",  // contains ..
		"foo/../bar", // contains ..
		"./dot",      // contains .
		"trailing/",  // ends with /
		"",           // empty string
	}

	for _, path := range invalidPaths {
		t.Run("Open_"+path, func(t *testing.T) {
			t.Parallel()
			_, err := reader.Open(path)
			require.Error(t, err)
			assert.ErrorIs(t, err, fs.ErrInvalid)
		})

		t.Run("Stat_"+path, func(t *testing.T) {
			t.Parallel()
			_, err := reader.Stat(path)
			require.Error(t, err)
			assert.ErrorIs(t, err, fs.ErrInvalid)
		})

		t.Run("ReadDir_"+path, func(t *testing.T) {
			t.Parallel()
			_, err := reader.ReadDir(path)
			require.Error(t, err)
			assert.ErrorIs(t, err, fs.ErrInvalid)
		})
	}
}

func TestReader_ReadDirSorted(t *testing.T) {
	t.Parallel()

	// Create files that would be in random order if not sorted.
	src := fstest.MapFS{
		"zebra.txt":  &fstest.MapFile{Data: []byte("z"), Mode: 0o644},
		"apple.txt":  &fstest.MapFile{Data: []byte("a"), Mode: 0o644},
		"mango.txt":  &fstest.MapFile{Data: []byte("m"), Mode: 0o644},
		"banana.txt": &fstest.MapFile{Data: []byte("b"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)
	reader, err := NewReader(context.Background(), newSizedReader(blob))
	require.NoError(t, err)
	defer reader.Close()

	entries, err := reader.ReadDir(".")
	require.NoError(t, err)
	require.Len(t, entries, 4)

	// Verify entries are sorted alphabetically.
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	assert.Equal(t, []string{"apple.txt", "banana.txt", "mango.txt", "zebra.txt"}, names)
}

func TestReader_DirReadDirContextCancellation(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"dir":       &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		"dir/a.txt": &fstest.MapFile{Data: []byte("a"), Mode: 0o644},
		"dir/b.txt": &fstest.MapFile{Data: []byte("b"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)

	ctx, cancel := context.WithCancel(context.Background())
	reader, err := NewReader(ctx, newSizedReader(blob))
	require.NoError(t, err)
	defer reader.Close()

	f, err := reader.Open("dir")
	require.NoError(t, err)
	defer f.Close()

	dirReader, ok := f.(fs.ReadDirFile)
	require.True(t, ok)

	// First call should succeed (loads entries).
	entries1, err := dirReader.ReadDir(1)
	require.NoError(t, err)
	assert.Len(t, entries1, 1)

	// Cancel the context.
	cancel()

	// Second call should fail even though entries are cached.
	_, err = dirReader.ReadDir(1)
	assert.ErrorIs(t, err, context.Canceled)
}
