package archive

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"testing/fstest"

	"github.com/klauspost/compress/zstd"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/core"
)

func TestDigestingWriter(t *testing.T) {
	t.Parallel()

	t.Run("computes correct digest and size", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		dw := newDigestingWriter(&buf)

		data := []byte("hello world")
		n, err := dw.Write(data)
		require.NoError(t, err)
		assert.Equal(t, len(data), n)

		// Verify size
		assert.Equal(t, int64(len(data)), dw.Size())

		// Verify digest matches expected SHA256
		expected := digest.FromBytes(data)
		assert.Equal(t, expected, dw.Digest())

		// Verify data was written to underlying buffer
		assert.Equal(t, data, buf.Bytes())
	})

	t.Run("accumulates size across multiple writes", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		dw := newDigestingWriter(&buf)

		chunk1 := []byte("hello ")
		chunk2 := []byte("world")

		_, err := dw.Write(chunk1)
		require.NoError(t, err)

		_, err = dw.Write(chunk2)
		require.NoError(t, err)

		// Total size should be sum of chunks
		assert.Equal(t, int64(len(chunk1)+len(chunk2)), dw.Size())

		// Digest should be of combined data
		combined := make([]byte, 0, len(chunk1)+len(chunk2))
		combined = append(combined, chunk1...)
		combined = append(combined, chunk2...)
		expected := digest.FromBytes(combined)
		assert.Equal(t, expected, dw.Digest())
	})

	t.Run("handles empty write", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		dw := newDigestingWriter(&buf)

		n, err := dw.Write([]byte{})
		require.NoError(t, err)
		assert.Equal(t, 0, n)
		assert.Equal(t, int64(0), dw.Size())
	})
}

func TestBuild(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fs          fstest.MapFS
		compression core.Compression
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
			compression: core.GzipCompression(),
			wantErr:     false,
		},
		{
			name: "directory with files",
			fs: fstest.MapFS{
				"dir":          &fstest.MapFile{Mode: 0o755 | fs.ModeDir},
				"dir/file.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
			},
			compression: core.GzipCompression(),
			wantErr:     false,
		},
		{
			name:        "empty filesystem",
			fs:          fstest.MapFS{},
			compression: core.GzipCompression(),
			wantErr:     false,
		},
		{
			name: "multiple files with zstd",
			fs: fstest.MapFS{
				"a.txt": &fstest.MapFile{Data: []byte("aaa"), Mode: 0o644},
				"b.txt": &fstest.MapFile{Data: []byte("bbb"), Mode: 0o644},
				"c.txt": &fstest.MapFile{Data: []byte("ccc"), Mode: 0o644},
			},
			compression: core.ZstdCompression(),
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			builder := NewBuilder(nil)
			result, err := builder.Build(context.Background(), tt.fs, tt.compression)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result, "Build() returned nil result")
			require.NotNil(t, result.Blob, "Build() returned nil blob")
			defer result.Blob.Close()

			assert.NotEmpty(t, result.TOCDigest, "Build() returned empty TOC digest")
			assert.NotEmpty(t, result.DiffID, "Build() returned empty DiffID")
			assert.NotEmpty(t, result.BlobDigest, "Build() returned empty blob digest")
			assert.Greater(t, result.BlobSize, int64(0), "Build() returned zero blob size")

			// DiffID (uncompressed) must differ from BlobDigest (compressed)
			assert.NotEqual(t, result.DiffID, result.BlobDigest,
				"DiffID should differ from BlobDigest (uncompressed vs compressed)")

			data, err := io.ReadAll(result.Blob)
			require.NoError(t, err, "failed to read blob")
			assert.NotEmpty(t, data, "Build() returned empty blob")
			assert.Equal(t, result.BlobSize, int64(len(data)), "BlobSize mismatch")

			// Verify BlobDigest matches actual content
			actualDigest := digest.FromBytes(data)
			assert.Equal(t, actualDigest.String(), result.BlobDigest, "BlobDigest mismatch")
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
	_, err := builder.Build(ctx, testFS, core.GzipCompression())

	assert.Error(t, err, "Build() should return error when context is canceled")
}

func TestBuild_DiffIDMatchesDecompressedBlob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		compression core.Compression
		decompress  func([]byte) ([]byte, error)
	}{
		{
			name:        "gzip",
			compression: core.GzipCompression(),
			decompress: func(data []byte) ([]byte, error) {
				// estargz uses multiple concatenated gzip streams for lazy loading.
				// We need to read all streams to get the complete tar.
				var result bytes.Buffer
				r := bytes.NewReader(data)
				for {
					gr, err := gzip.NewReader(r)
					if err == io.EOF {
						break
					}
					if err != nil {
						return nil, err
					}
					if _, err := io.Copy(&result, gr); err != nil {
						gr.Close()
						return nil, err
					}
					gr.Close()
				}
				return result.Bytes(), nil
			},
		},
		{
			name:        "zstd",
			compression: core.ZstdCompression(),
			decompress: func(data []byte) ([]byte, error) {
				zr, err := zstd.NewReader(bytes.NewReader(data))
				if err != nil {
					return nil, err
				}
				defer zr.Close()
				return io.ReadAll(zr)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testFS := fstest.MapFS{
				"hello.txt": &fstest.MapFile{Data: []byte("hello world"), Mode: 0o644},
				"data.bin":  &fstest.MapFile{Data: []byte{0x00, 0x01, 0x02, 0x03}, Mode: 0o644},
			}

			builder := NewBuilder(nil)
			result, err := builder.Build(context.Background(), testFS, tt.compression)
			require.NoError(t, err)
			defer result.Blob.Close()

			// Read the compressed blob
			compressedData, err := io.ReadAll(result.Blob)
			require.NoError(t, err)

			// Decompress the blob
			uncompressedData, err := tt.decompress(compressedData)
			require.NoError(t, err, "failed to decompress blob")

			// Hash the uncompressed layer bytes.
			actualDiffID := digest.FromBytes(uncompressedData)

			// Verify DiffID matches the hash of the original tar content
			assert.Equal(t, actualDiffID.String(), result.DiffID,
				"DiffID should match the digest of the original tar content")
		})
	}
}

func TestBuild_RoundTrip(t *testing.T) {
	t.Parallel()

	// Create a filesystem with test files
	testFS := fstest.MapFS{
		"config.yaml": &fstest.MapFile{Data: []byte("key: value\n"), Mode: 0o644},
		"data.txt":    &fstest.MapFile{Data: []byte("some data"), Mode: 0o644},
	}

	builder := NewBuilder(nil)
	result, err := builder.Build(context.Background(), testFS, core.GzipCompression())
	require.NoError(t, err)
	defer result.Blob.Close()

	data, err := io.ReadAll(result.Blob)
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
	result, err := builder.Build(context.Background(), testFS, core.GzipCompression())
	require.NoError(t, err)

	// Drain blob before closing - estargz uses io.Pipe internally
	// and will block/leak goroutines if not drained
	_, copyErr := io.Copy(io.Discard, result.Blob)
	require.NoError(t, copyErr, "failed to drain blob")
	result.Blob.Close()

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
		if runtime.GOOS == osWindows {
			t.Skipf("skipping symlink test on windows: %v", symlinkErr)
		}
		require.NoError(t, symlinkErr, "failed to create symlink")
	}

	// Build using OSFS (which supports symlinks)
	builder := NewBuilder(nil)
	result, err := builder.Build(context.Background(), OSFS(tmpDir), core.GzipCompression())
	require.NoError(t, err)
	defer result.Blob.Close()

	// Read the blob and verify TOC
	data, err := io.ReadAll(result.Blob)
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
	result, err := builder.Build(context.Background(), testFS, core.GzipCompression())

	if err == nil {
		// Drain blob to avoid goroutine leak
		if result != nil && result.Blob != nil {
			_, _ = io.Copy(io.Discard, result.Blob)
			result.Blob.Close()
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
	result, err := builder.Build(context.Background(), testFS, core.GzipCompression())
	require.NoError(t, err)
	defer result.Blob.Close()

	data, err := io.ReadAll(result.Blob)
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
	result, err := builder.Build(context.Background(), testFS, core.GzipCompression())
	require.NoError(t, err)
	defer result.Blob.Close()

	data, err := io.ReadAll(result.Blob)
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
	result, err := builder.Build(context.Background(), testFS, core.GzipCompression())
	require.NoError(t, err)
	defer result.Blob.Close()

	data, err := io.ReadAll(result.Blob)
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
