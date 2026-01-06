package archive

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilmanlab/blobber"
)

func TestReader_ReadTOC(t *testing.T) {
	t.Parallel()

	// Create a test eStargz blob
	testFS := fstest.MapFS{
		"file1.txt":        &fstest.MapFile{Data: []byte("content1"), Mode: 0o644},
		"file2.txt":        &fstest.MapFile{Data: []byte("content2"), Mode: 0o644},
		"subdir":           &fstest.MapFile{Mode: 0o755 | fs.ModeDir},
		"subdir/nested.go": &fstest.MapFile{Data: []byte("package main"), Mode: 0o644},
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

	// Verify entries
	entryMap := make(map[string]blobber.TOCEntry)
	for _, e := range toc.Entries {
		entryMap[e.Name] = e
	}

	// Check file1.txt exists
	assert.Contains(t, entryMap, "file1.txt", "file1.txt not found in TOC")

	// Check subdir exists and is a directory
	if e, ok := entryMap["subdir"]; assert.True(t, ok, "subdir not found in TOC") {
		assert.Equal(t, "dir", e.Type, "subdir type mismatch")
	}

	// Check nested file exists
	assert.Contains(t, entryMap, "subdir/nested.go", "subdir/nested.go not found in TOC")

	// Verify root entry is omitted
	assert.NotContains(t, entryMap, "", "root entry should be omitted from TOC")
}

func TestReader_ReadTOC_Zstd(t *testing.T) {
	t.Parallel()

	testFS := fstest.MapFS{
		"test.txt": &fstest.MapFile{Data: []byte("zstd content"), Mode: 0o644},
	}

	builder := NewBuilder(nil)
	blob, _, err := builder.Build(context.Background(), testFS, blobber.ZstdCompression())
	require.NoError(t, err)
	defer blob.Close()

	data, err := io.ReadAll(blob)
	require.NoError(t, err, "failed to read blob")

	reader := NewReader()
	toc, err := reader.ReadTOC(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	assert.NotEmpty(t, toc.Entries, "expected entries in TOC for zstd archive")
}

func TestReader_ReadTOC_InvalidArchive(t *testing.T) {
	t.Parallel()

	reader := NewReader()

	// Test with invalid data
	invalidData := []byte("this is not a valid eStargz archive")
	_, err := reader.ReadTOC(bytes.NewReader(invalidData), int64(len(invalidData)))

	assert.ErrorIs(t, err, blobber.ErrInvalidArchive)
}

func TestReader_OpenFile(t *testing.T) {
	t.Parallel()

	expectedContent := "hello from test file"
	testFS := fstest.MapFS{
		"test.txt": &fstest.MapFile{Data: []byte(expectedContent), Mode: 0o644},
	}

	builder := NewBuilder(nil)
	blob, _, err := builder.Build(context.Background(), testFS, blobber.GzipCompression())
	require.NoError(t, err)
	defer blob.Close()

	data, err := io.ReadAll(blob)
	require.NoError(t, err, "failed to read blob")

	reader := NewReader()
	ra := bytes.NewReader(data)
	size := int64(len(data))

	// Read TOC first to get entry
	toc, err := reader.ReadTOC(ra, size)
	require.NoError(t, err)

	// Find test.txt entry
	var entry blobber.TOCEntry
	for _, e := range toc.Entries {
		if e.Name == "test.txt" {
			entry = e
			break
		}
	}
	require.NotEmpty(t, entry.Name, "test.txt not found in TOC")

	// Open file
	fileReader, err := reader.OpenFile(ra, size, entry)
	require.NoError(t, err)

	// Read content
	content, err := io.ReadAll(fileReader)
	require.NoError(t, err, "failed to read file")

	assert.Equal(t, expectedContent, string(content))
}

func TestReader_OpenFile_Zstd(t *testing.T) {
	t.Parallel()

	expectedContent := "zstd file content"
	testFS := fstest.MapFS{
		"zstd.txt": &fstest.MapFile{Data: []byte(expectedContent), Mode: 0o644},
	}

	builder := NewBuilder(nil)
	blob, _, err := builder.Build(context.Background(), testFS, blobber.ZstdCompression())
	require.NoError(t, err)
	defer blob.Close()

	data, err := io.ReadAll(blob)
	require.NoError(t, err, "failed to read blob")

	reader := NewReader()
	ra := bytes.NewReader(data)
	size := int64(len(data))

	toc, err := reader.ReadTOC(ra, size)
	require.NoError(t, err)

	var entry blobber.TOCEntry
	for _, e := range toc.Entries {
		if e.Name == "zstd.txt" {
			entry = e
			break
		}
	}

	fileReader, err := reader.OpenFile(ra, size, entry)
	require.NoError(t, err)

	content, err := io.ReadAll(fileReader)
	require.NoError(t, err, "failed to read file")

	assert.Equal(t, expectedContent, string(content))
}

func TestReader_OpenFile_Missing(t *testing.T) {
	t.Parallel()

	testFS := fstest.MapFS{
		"exists.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
	}

	builder := NewBuilder(nil)
	blob, _, err := builder.Build(context.Background(), testFS, blobber.GzipCompression())
	require.NoError(t, err)
	defer blob.Close()

	data, err := io.ReadAll(blob)
	require.NoError(t, err, "failed to read blob")

	reader := NewReader()
	ra := bytes.NewReader(data)
	size := int64(len(data))

	// Try to open a non-existent file
	missingEntry := blobber.TOCEntry{
		Name: "does-not-exist.txt",
		Type: "reg",
		Size: 100,
	}

	_, err = reader.OpenFile(ra, size, missingEntry)
	assert.Error(t, err, "OpenFile() should return error for missing file")
}
