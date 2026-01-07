package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilmanlab/blobber/core"
	"github.com/gilmanlab/blobber/internal/safepath"
)

func TestExtract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fs          fstest.MapFS
		compression core.Compression
		wantFiles   []string
	}{
		{
			name: "simple file gzip",
			fs: fstest.MapFS{
				"hello.txt": &fstest.MapFile{Data: []byte("hello world"), Mode: 0o644},
			},
			compression: core.GzipCompression(),
			wantFiles:   []string{"hello.txt"},
		},
		{
			name: "directory with files",
			fs: fstest.MapFS{
				"subdir":          &fstest.MapFile{Mode: 0o755 | os.ModeDir},
				"subdir/file.txt": &fstest.MapFile{Data: []byte("nested content"), Mode: 0o644},
			},
			compression: core.GzipCompression(),
			wantFiles:   []string{"subdir/file.txt"},
		},
		{
			name: "multiple files zstd",
			fs: fstest.MapFS{
				"a.txt": &fstest.MapFile{Data: []byte("aaa"), Mode: 0o644},
				"b.txt": &fstest.MapFile{Data: []byte("bbb"), Mode: 0o644},
			},
			compression: core.ZstdCompression(),
			wantFiles:   []string{"a.txt", "b.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Build archive
			builder := NewBuilder(nil)
			result, err := builder.Build(context.Background(), tt.fs, tt.compression)
			require.NoError(t, err)
			defer result.Blob.Close()

			data, err := io.ReadAll(result.Blob)
			require.NoError(t, err, "failed to read blob")

			// Create temp directory for extraction
			destDir := t.TempDir()

			// Extract
			validator := safepath.NewValidator()
			limits := core.ExtractLimits{}
			err = Extract(context.Background(), bytes.NewReader(data), destDir, validator, limits)
			require.NoError(t, err)

			// Verify files exist
			for _, wantFile := range tt.wantFiles {
				path := filepath.Join(destDir, wantFile)
				_, err := os.Stat(path)
				assert.NoError(t, err, "expected file %s to exist", wantFile)
			}
		})
	}
}

func TestExtract_FileContent(t *testing.T) {
	t.Parallel()

	expectedContent := "test file content 12345"
	testFS := fstest.MapFS{
		"test.txt": &fstest.MapFile{Data: []byte(expectedContent), Mode: 0o644},
	}

	builder := NewBuilder(nil)
	result, err := builder.Build(context.Background(), testFS, core.GzipCompression())
	require.NoError(t, err)
	defer result.Blob.Close()

	data, err := io.ReadAll(result.Blob)
	require.NoError(t, err, "failed to read blob")

	destDir := t.TempDir()
	validator := safepath.NewValidator()
	limits := core.ExtractLimits{}

	err = Extract(context.Background(), bytes.NewReader(data), destDir, validator, limits)
	require.NoError(t, err)

	//nolint:gosec // G304: Test file path is constructed from t.TempDir()
	content, err := os.ReadFile(filepath.Join(destDir, "test.txt"))
	require.NoError(t, err, "failed to read extracted file")

	assert.Equal(t, expectedContent, string(content))
}

func TestExtract_Symlink(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	targetPath := filepath.Join(tmpDir, "target.txt")
	err := os.WriteFile(targetPath, []byte("target content"), 0o644)
	require.NoError(t, err, "failed to create target file")

	linkPath := filepath.Join(tmpDir, "link.txt")
	if symlinkErr := os.Symlink("target.txt", linkPath); symlinkErr != nil {
		if runtime.GOOS == osWindows {
			t.Skipf("skipping symlink test on windows: %v", symlinkErr)
		}
		require.NoError(t, symlinkErr, "failed to create symlink")
	}

	builder := NewBuilder(nil)
	result, err := builder.Build(context.Background(), OSFS(tmpDir), core.GzipCompression())
	require.NoError(t, err)
	defer result.Blob.Close()

	data, err := io.ReadAll(result.Blob)
	require.NoError(t, err, "failed to read blob")

	destDir := t.TempDir()
	validator := safepath.NewValidator()
	limits := core.ExtractLimits{}

	err = Extract(context.Background(), bytes.NewReader(data), destDir, validator, limits)
	require.NoError(t, err)

	linkDest := filepath.Join(destDir, "link.txt")
	info, err := os.Lstat(linkDest)
	require.NoError(t, err)
	require.True(t, info.Mode()&os.ModeSymlink != 0, "expected symlink at %s", linkDest)

	target, err := os.Readlink(linkDest)
	require.NoError(t, err)
	assert.Equal(t, "target.txt", target)

	_, err = os.Stat(filepath.Join(destDir, "target.txt"))
	assert.NoError(t, err, "target file missing after extract")
}

func TestExtract_Limits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fs      fstest.MapFS
		limits  core.ExtractLimits
		wantErr error
	}{
		{
			name: "max files exceeded",
			fs: fstest.MapFS{
				"a.txt": &fstest.MapFile{Data: []byte("a"), Mode: 0o644},
				"b.txt": &fstest.MapFile{Data: []byte("b"), Mode: 0o644},
				"c.txt": &fstest.MapFile{Data: []byte("c"), Mode: 0o644},
			},
			limits:  core.ExtractLimits{MaxFiles: 2},
			wantErr: core.ErrExtractLimits,
		},
		{
			name: "max file size exceeded",
			fs: fstest.MapFS{
				"large.txt": &fstest.MapFile{Data: make([]byte, 1000), Mode: 0o644},
			},
			limits:  core.ExtractLimits{MaxFileSize: 500},
			wantErr: core.ErrExtractLimits,
		},
		{
			name: "max total size exceeded",
			fs: fstest.MapFS{
				"a.txt": &fstest.MapFile{Data: make([]byte, 400), Mode: 0o644},
				"b.txt": &fstest.MapFile{Data: make([]byte, 400), Mode: 0o644},
			},
			limits:  core.ExtractLimits{MaxTotalSize: 500},
			wantErr: core.ErrExtractLimits,
		},
		{
			name: "within limits",
			fs: fstest.MapFS{
				"a.txt": &fstest.MapFile{Data: []byte("a"), Mode: 0o644},
				"b.txt": &fstest.MapFile{Data: []byte("b"), Mode: 0o644},
			},
			limits:  core.ExtractLimits{MaxFiles: 10, MaxFileSize: 1000, MaxTotalSize: 10000},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			builder := NewBuilder(nil)
			result, err := builder.Build(context.Background(), tt.fs, core.GzipCompression())
			require.NoError(t, err)
			defer result.Blob.Close()

			data, err := io.ReadAll(result.Blob)
			require.NoError(t, err, "failed to read blob")

			destDir := t.TempDir()
			validator := safepath.NewValidator()

			err = Extract(context.Background(), bytes.NewReader(data), destDir, validator, tt.limits)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExtract_ContextCancellation(t *testing.T) {
	t.Parallel()

	testFS := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
	}

	builder := NewBuilder(nil)
	result, err := builder.Build(context.Background(), testFS, core.GzipCompression())
	require.NoError(t, err)
	defer result.Blob.Close()

	data, err := io.ReadAll(result.Blob)
	require.NoError(t, err, "failed to read blob")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	destDir := t.TempDir()
	validator := safepath.NewValidator()
	limits := core.ExtractLimits{}

	err = Extract(ctx, bytes.NewReader(data), destDir, validator, limits)
	assert.Error(t, err, "Extract() should return error when context is canceled")
}

func TestExtract_InvalidArchive(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	validator := safepath.NewValidator()
	limits := core.ExtractLimits{}

	// Invalid data (not gzip or zstd)
	invalidData := []byte("this is not a valid archive")

	err := Extract(context.Background(), bytes.NewReader(invalidData), destDir, validator, limits)
	assert.ErrorIs(t, err, core.ErrInvalidArchive)
}

func Test_detectAndDecompress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		compression core.Compression
	}{
		{
			name:        "gzip detection",
			compression: core.GzipCompression(),
		},
		{
			name:        "zstd detection",
			compression: core.ZstdCompression(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testFS := fstest.MapFS{
				"test.txt": &fstest.MapFile{Data: []byte("test content"), Mode: 0o644},
			}

			builder := NewBuilder(nil)
			result, err := builder.Build(context.Background(), testFS, tt.compression)
			require.NoError(t, err)
			defer result.Blob.Close()

			data, err := io.ReadAll(result.Blob)
			require.NoError(t, err, "failed to read blob")

			reader, err := detectAndDecompress(bytes.NewReader(data))
			require.NoError(t, err)
			defer reader.Close()

			// Should be able to read as tar
			decompressed, err := io.ReadAll(reader)
			require.NoError(t, err, "failed to read decompressed data")

			assert.NotEmpty(t, decompressed, "decompressed data is empty")
		})
	}
}

func TestExtract_HardlinkRejected(t *testing.T) {
	t.Parallel()

	// Create a tar archive with a hardlink entry directly
	// since fstest.MapFS doesn't support hardlinks
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Write a regular file first
	err := tw.WriteHeader(&tar.Header{
		Name: "original.txt",
		Mode: 0o644,
		Size: 7,
	})
	require.NoError(t, err, "failed to write header")

	_, err = tw.Write([]byte("content"))
	require.NoError(t, err, "failed to write content")

	// Write a hardlink to the file
	err = tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeLink,
		Name:     "hardlink.txt",
		Linkname: "original.txt",
		Mode:     0o644,
	})
	require.NoError(t, err, "failed to write hardlink header")

	err = tw.Close()
	require.NoError(t, err, "failed to close tar writer")

	// Compress with gzip
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, err = gw.Write(buf.Bytes())
	require.NoError(t, err, "failed to gzip")

	err = gw.Close()
	require.NoError(t, err, "failed to close gzip")

	destDir := t.TempDir()
	validator := safepath.NewValidator()
	limits := core.ExtractLimits{}

	err = Extract(context.Background(), &gzBuf, destDir, validator, limits)
	assert.ErrorIs(t, err, core.ErrInvalidArchive)
}
