package archive

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilmanlab/blobber"
)

func TestBuild(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fs          fstest.MapFS
		compression blobber.Compression
		wantErr     bool
	}{
		{
			name: "simple file with gzip",
			fs: fstest.MapFS{
				"hello.txt": &fstest.MapFile{
					Data: []byte("hello world"),
					Mode: 0o644,
				},
			},
			compression: blobber.GzipCompression(),
			wantErr:     false,
		},
		{
			name: "directory with files",
			fs: fstest.MapFS{
				"dir":          &fstest.MapFile{Mode: 0o755 | fs.ModeDir},
				"dir/file.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
			},
			compression: blobber.GzipCompression(),
			wantErr:     false,
		},
		{
			name:        "empty filesystem",
			fs:          fstest.MapFS{},
			compression: blobber.GzipCompression(),
			wantErr:     false,
		},
		{
			name: "multiple files with zstd",
			fs: fstest.MapFS{
				"a.txt": &fstest.MapFile{Data: []byte("aaa"), Mode: 0o644},
				"b.txt": &fstest.MapFile{Data: []byte("bbb"), Mode: 0o644},
				"c.txt": &fstest.MapFile{Data: []byte("ccc"), Mode: 0o644},
			},
			compression: blobber.ZstdCompression(),
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			builder := NewBuilder(nil)
			blob, tocDigest, err := builder.Build(context.Background(), tt.fs, tt.compression)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, blob, "Build() returned nil blob")
			defer blob.Close()

			assert.NotEmpty(t, tocDigest, "Build() returned empty TOC digest")

			data, err := io.ReadAll(blob)
			require.NoError(t, err, "failed to read blob")
			assert.NotEmpty(t, data, "Build() returned empty blob")
		})
	}
}

func TestBuild_ContextCancellation(t *testing.T) {
	t.Parallel()

	testFS := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	builder := NewBuilder(nil)
	_, _, err := builder.Build(ctx, testFS, blobber.GzipCompression())

	assert.Error(t, err, "Build() should return error when context is canceled")
}

func TestBuild_RoundTrip(t *testing.T) {
	t.Parallel()

	// Create a filesystem with test files
	testFS := fstest.MapFS{
		"config.yaml": &fstest.MapFile{Data: []byte("key: value\n"), Mode: 0o644},
		"data.txt":    &fstest.MapFile{Data: []byte("some data"), Mode: 0o644},
	}

	builder := NewBuilder(nil)
	blob, _, err := builder.Build(context.Background(), testFS, blobber.GzipCompression())
	require.NoError(t, err)
	defer blob.Close()

	data, err := io.ReadAll(blob)
	require.NoError(t, err, "failed to read blob")

	reader := NewReader()
	ra := bytes.NewReader(data)
	toc, err := reader.ReadTOC(ra, int64(len(data)))
	require.NoError(t, err)

	assert.NotEmpty(t, toc.Entries, "expected entries in TOC")

	// Find and verify config.yaml
	var foundConfig bool
	for _, entry := range toc.Entries {
		if entry.Name == "config.yaml" {
			foundConfig = true
			assert.Equal(t, "reg", entry.Type, "config.yaml type mismatch") //nolint:goconst // tar type string
		}
	}
	assert.True(t, foundConfig, "config.yaml not found in TOC")
}

func TestBuild_TempFileCleanup(t *testing.T) {
	// NOT parallel - t.Setenv mutates process-wide state

	// Isolate temp files to test-specific directory
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir) // Unix
	t.Setenv("TEMP", tmpDir)   // Windows
	t.Setenv("TMP", tmpDir)    // Windows fallback

	testFS := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
	}

	builder := NewBuilder(nil)
	blob, _, err := builder.Build(context.Background(), testFS, blobber.GzipCompression())
	require.NoError(t, err)

	// Drain blob before closing - estargz uses io.Pipe internally
	// and will block/leak goroutines if not drained
	_, copyErr := io.Copy(io.Discard, blob)
	require.NoError(t, copyErr, "failed to drain blob")
	blob.Close()

	// Verify temp directory is empty (temp file was cleaned up)
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "temp directory not empty")
}

func TestBuild_WithSymlinks(t *testing.T) {
	t.Parallel()

	// Create a temp directory with a file and a symlink
	tmpDir := t.TempDir()

	// Create target file
	targetPath := filepath.Join(tmpDir, "target.txt")
	err := os.WriteFile(targetPath, []byte("target content"), 0o644)
	require.NoError(t, err, "failed to create target file")

	// Create symlink pointing to target
	linkPath := filepath.Join(tmpDir, "link.txt")
	if symlinkErr := os.Symlink("target.txt", linkPath); symlinkErr != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("skipping symlink test on windows: %v", symlinkErr)
		}
		require.NoError(t, symlinkErr, "failed to create symlink")
	}

	// Build using OSFS (which supports symlinks)
	builder := NewBuilder(nil)
	blob, _, err := builder.Build(context.Background(), OSFS(tmpDir), blobber.GzipCompression())
	require.NoError(t, err)
	defer blob.Close()

	// Read the blob and verify TOC
	data, err := io.ReadAll(blob)
	require.NoError(t, err, "failed to read blob")

	reader := NewReader()
	toc, err := reader.ReadTOC(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	// Find the symlink entry
	var foundSymlink bool
	for _, entry := range toc.Entries {
		if entry.Name == "link.txt" {
			foundSymlink = true
			assert.Equal(t, "symlink", entry.Type, "link.txt type mismatch")
			assert.Equal(t, "target.txt", entry.LinkName, "link.txt linkname mismatch")
		}
	}
	assert.True(t, foundSymlink, "symlink entry not found in TOC")
}

func TestBuild_SymlinkWithoutLstatFS(t *testing.T) {
	t.Parallel()

	// Create a filesystem that reports symlinks but doesn't implement Lstat/ReadLink
	testFS := &noSymlinkSupportFS{
		inner: fstest.MapFS{
			"link": &fstest.MapFile{
				Mode: fs.ModeSymlink | 0o777,
			},
		},
	}

	builder := NewBuilder(nil)
	blob, _, err := builder.Build(context.Background(), testFS, blobber.GzipCompression())

	if err == nil {
		// Drain blob to avoid goroutine leak
		if blob != nil {
			_, _ = io.Copy(io.Discard, blob)
			blob.Close()
		}
	}
	assert.Error(t, err, "Build() should return error for symlink without Lstat support")
}

// noSymlinkSupportFS wraps an fs.FS but doesn't expose Lstat/ReadLink.
type noSymlinkSupportFS struct {
	inner fs.FS
}

func (f *noSymlinkSupportFS) Open(name string) (fs.File, error) {
	return f.inner.Open(name)
}

func (f *noSymlinkSupportFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(f.inner, name)
}

func TestBuild_EmptyFile(t *testing.T) {
	t.Parallel()

	testFS := fstest.MapFS{
		"empty.txt": &fstest.MapFile{Data: []byte{}, Mode: 0o644},
	}

	builder := NewBuilder(nil)
	blob, _, err := builder.Build(context.Background(), testFS, blobber.GzipCompression())
	require.NoError(t, err)
	defer blob.Close()

	data, err := io.ReadAll(blob)
	require.NoError(t, err, "failed to read blob")

	reader := NewReader()
	toc, err := reader.ReadTOC(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	var found bool
	for _, entry := range toc.Entries {
		if entry.Name == "empty.txt" {
			found = true
			assert.Equal(t, int64(0), entry.Size, "empty.txt size mismatch")
			assert.Equal(t, "reg", entry.Type, "empty.txt type mismatch")
		}
	}
	assert.True(t, found, "empty.txt not found in TOC")
}

func TestBuild_SpecialCharacters(t *testing.T) {
	t.Parallel()

	testFS := fstest.MapFS{
		"file with spaces.txt":      &fstest.MapFile{Data: []byte("spaces"), Mode: 0o644},
		"file-with-dashes.txt":      &fstest.MapFile{Data: []byte("dashes"), Mode: 0o644},
		"file_with_underscores.txt": &fstest.MapFile{Data: []byte("underscores"), Mode: 0o644},
		"file.multiple.dots.txt":    &fstest.MapFile{Data: []byte("dots"), Mode: 0o644},
	}

	builder := NewBuilder(nil)
	blob, _, err := builder.Build(context.Background(), testFS, blobber.GzipCompression())
	require.NoError(t, err)
	defer blob.Close()

	data, err := io.ReadAll(blob)
	require.NoError(t, err, "failed to read blob")

	reader := NewReader()
	toc, err := reader.ReadTOC(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	expectedFiles := map[string]bool{
		"file with spaces.txt":      false,
		"file-with-dashes.txt":      false,
		"file_with_underscores.txt": false,
		"file.multiple.dots.txt":    false,
	}

	for _, entry := range toc.Entries {
		if _, ok := expectedFiles[entry.Name]; ok {
			expectedFiles[entry.Name] = true
		}
	}

	for name, found := range expectedFiles {
		assert.True(t, found, "file %q not found in TOC", name)
	}
}

func TestBuild_DeeplyNested(t *testing.T) {
	t.Parallel()

	// Create a deeply nested directory structure
	testFS := fstest.MapFS{
		"a":                        &fstest.MapFile{Mode: 0o755 | fs.ModeDir},
		"a/b":                      &fstest.MapFile{Mode: 0o755 | fs.ModeDir},
		"a/b/c":                    &fstest.MapFile{Mode: 0o755 | fs.ModeDir},
		"a/b/c/d":                  &fstest.MapFile{Mode: 0o755 | fs.ModeDir},
		"a/b/c/d/e":                &fstest.MapFile{Mode: 0o755 | fs.ModeDir},
		"a/b/c/d/e/f":              &fstest.MapFile{Mode: 0o755 | fs.ModeDir},
		"a/b/c/d/e/f/g":            &fstest.MapFile{Mode: 0o755 | fs.ModeDir},
		"a/b/c/d/e/f/g/h":          &fstest.MapFile{Mode: 0o755 | fs.ModeDir},
		"a/b/c/d/e/f/g/h/deep.txt": &fstest.MapFile{Data: []byte("deep content"), Mode: 0o644},
	}

	builder := NewBuilder(nil)
	blob, _, err := builder.Build(context.Background(), testFS, blobber.GzipCompression())
	require.NoError(t, err)
	defer blob.Close()

	data, err := io.ReadAll(blob)
	require.NoError(t, err, "failed to read blob")

	reader := NewReader()
	toc, err := reader.ReadTOC(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	var found bool
	for _, entry := range toc.Entries {
		if entry.Name == "a/b/c/d/e/f/g/h/deep.txt" {
			found = true
			assert.Equal(t, "reg", entry.Type, "deep.txt type mismatch")
		}
	}
	assert.True(t, found, "a/b/c/d/e/f/g/h/deep.txt not found in TOC")
}
