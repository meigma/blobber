package estargz

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

	"github.com/meigma/blobber/v2/internal/tar"
	"github.com/meigma/blobber/v2/internal/tar/mocks"
)

func TestExtract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		src       fs.FS
		wantFiles []string
		wantDirs  []string
	}{
		{
			name:      "empty filesystem",
			src:       fstest.MapFS{},
			wantFiles: nil,
			wantDirs:  nil,
		},
		{
			name: "single file",
			src: fstest.MapFS{
				"hello.txt": &fstest.MapFile{Data: []byte("hello world"), Mode: 0o644},
			},
			wantFiles: []string{"hello.txt"},
		},
		{
			name: "nested structure",
			src: fstest.MapFS{
				"dir":              &fstest.MapFile{Mode: fs.ModeDir | 0o755},
				"dir/subdir":       &fstest.MapFile{Mode: fs.ModeDir | 0o755},
				"dir/subdir/a.txt": &fstest.MapFile{Data: []byte("a"), Mode: 0o644},
				"dir/b.txt":        &fstest.MapFile{Data: []byte("b"), Mode: 0o644},
				"root.txt":         &fstest.MapFile{Data: []byte("root"), Mode: 0o644},
			},
			wantFiles: []string{"dir/subdir/a.txt", "dir/b.txt", "root.txt"},
			wantDirs:  []string{"dir", "dir/subdir"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Build an estargz archive.
			blob := buildTestBlob(t, tt.src)

			// Extract to temp directory.
			destDir := t.TempDir()
			e := NewExtractor()
			err := e.Extract(context.Background(), bytes.NewReader(blob), destDir)
			require.NoError(t, err)

			// Verify expected files exist with correct content.
			for _, wantFile := range tt.wantFiles {
				path := filepath.Join(destDir, wantFile)
				info, err := os.Stat(path)
				require.NoError(t, err, "expected file %s to exist", wantFile)
				assert.True(t, info.Mode().IsRegular(), "expected %s to be a regular file", wantFile)
			}

			// Verify expected directories exist.
			for _, wantDir := range tt.wantDirs {
				path := filepath.Join(destDir, wantDir)
				info, err := os.Stat(path)
				require.NoError(t, err, "expected directory %s to exist", wantDir)
				assert.True(t, info.IsDir(), "expected %s to be a directory", wantDir)
			}
		})
	}
}

func TestExtract_TOCExcluded(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)

	// Extract and verify TOC is not extracted.
	// The estargz format embeds a TOC (stargz.index.json) which our extractor
	// should filter out during extraction.
	destDir := t.TempDir()
	e := NewExtractor()
	err := e.Extract(context.Background(), bytes.NewReader(blob), destDir)
	require.NoError(t, err)

	// The TOC file should NOT exist in the destination.
	tocPath := filepath.Join(destDir, tocFilename)
	_, err = os.Stat(tocPath)
	assert.True(t, os.IsNotExist(err), "TOC file should not be extracted")

	// But the regular file should exist.
	filePath := filepath.Join(destDir, "file.txt")
	_, err = os.Stat(filePath)
	assert.NoError(t, err, "regular file should be extracted")
}

func TestIsTOCEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{"stargz.index.json", true},
		{"./stargz.index.json", true},
		{"file.txt", false},
		{"./file.txt", false},
		{"stargz.index.json.bak", false},
		{"dir/stargz.index.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			got := isTOCEntry(tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtract_FileContent(t *testing.T) {
	t.Parallel()

	content := []byte("hello world content")
	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: content, Mode: 0o644},
	}

	blob := buildTestBlob(t, src)

	destDir := t.TempDir()
	e := NewExtractor()
	err := e.Extract(context.Background(), bytes.NewReader(blob), destDir)
	require.NoError(t, err)

	// Verify file content matches.
	got, err := os.ReadFile(filepath.Join(destDir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestExtract_ContextCancellation(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("data"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	destDir := t.TempDir()
	e := NewExtractor()
	err := e.Extract(ctx, bytes.NewReader(blob), destDir)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestExtract_UnknownCompression(t *testing.T) {
	t.Parallel()

	// Create data with unrecognized magic bytes (but long enough to detect).
	invalidBlob := []byte{0x00, 0x00, 0x00, 0x00, 0x00}

	destDir := t.TempDir()
	e := NewExtractor()
	err := e.Extract(context.Background(), bytes.NewReader(invalidBlob), destDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown compression format")
}

func TestExtract_TruncatedInput(t *testing.T) {
	t.Parallel()

	// Create data that's too short to detect compression format.
	truncatedBlob := []byte{0x1f} // Only 1 byte, gzip needs at least 2

	destDir := t.TempDir()
	e := NewExtractor()
	err := e.Extract(context.Background(), bytes.NewReader(truncatedBlob), destDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "input too short")
}

func TestExtract_WithTarExtractor(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
	}

	blob := buildTestBlob(t, src)

	// Track that our custom extractor was called.
	var extractCalled bool
	mockExtractor := &mocks.ExtractorMock{
		ExtractFunc: func(ctx context.Context, r io.Reader, destDir string, filter tar.EntryFilter) error {
			extractCalled = true
			// Consume the reader to avoid blocking.
			_, _ = io.Copy(io.Discard, r)
			return nil
		},
	}

	destDir := t.TempDir()
	e := NewExtractor(WithTarExtractor(mockExtractor))
	err := e.Extract(context.Background(), bytes.NewReader(blob), destDir)
	require.NoError(t, err)
	assert.True(t, extractCalled, "custom tar extractor should be called")
}

// buildTestBlob creates an estargz blob from the given filesystem.
func buildTestBlob(t *testing.T, src fs.FS) []byte {
	t.Helper()

	var buf bytes.Buffer
	b := NewBuilder()
	_, err := b.Build(context.Background(), &buf, src)
	require.NoError(t, err)

	return buf.Bytes()
}
