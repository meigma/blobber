package archive

import (
	"bytes"
	"crypto/sha256"
	"io"
	"io/fs"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testFileInfo struct {
	name    string
	mode    fs.FileMode
	size    int64
	modTime time.Time
}

func (tfi testFileInfo) Name() string       { return tfi.name }
func (tfi testFileInfo) Size() int64        { return tfi.size }
func (tfi testFileInfo) Mode() fs.FileMode  { return tfi.mode }
func (tfi testFileInfo) ModTime() time.Time { return tfi.modTime }
func (tfi testFileInfo) IsDir() bool        { return tfi.mode.IsDir() }
func (tfi testFileInfo) Sys() any           { return nil }

func TestIndexRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	fileData := []byte("hello world")
	hash := sha256.Sum256(fileData)

	tests := []struct {
		name   string
		layout Layout
	}{
		{name: "layout-a", layout: LayoutA},
		{name: "layout-b", layout: LayoutB},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var dataBuf bytes.Buffer
			builder := NewBuilder(
				WithLayout(tt.layout),
				WithDataWriter(&dataBuf),
			)

			dirInfo := testFileInfo{name: "config", mode: fs.ModeDir | 0o755, modTime: now}
			require.NoError(t, builder.AddDir("config", dirInfo))

			fileInfo := testFileInfo{name: "app.yaml", mode: 0o644, size: int64(len(fileData)), modTime: now}
			require.NoError(t, builder.AddFile("config/app.yaml", fileInfo, bytes.NewReader(fileData)))

			var indexBuf bytes.Buffer
			result, err := builder.Finalize(&indexBuf)
			require.NoError(t, err)
			require.Equal(t, uint32(2), result.EntryCount)

			idx, err := OpenIndex(bytes.NewReader(indexBuf.Bytes()), int64(indexBuf.Len()))
			require.NoError(t, err)

			entry, ok, err := idx.Lookup("config/app.yaml")
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, EntryFile, entry.Type)
			assert.Equal(t, uint32(0o644), entry.Mode)
			assert.Equal(t, now, entry.MTime)
			assert.Equal(t, uint64(len(fileData)), entry.RawSize)
			assert.Equal(t, hash, entry.Hash)

			content := dataBuf.Bytes()[entry.DataOffset : entry.DataOffset+entry.CompSize]
			assert.Equal(t, fileData, content)

			dirEntry, ok, err := idx.Lookup("config")
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, EntryDir, dirEntry.Type)
			assert.Equal(t, uint32(0o755), dirEntry.Mode)
			assert.Equal(t, now, dirEntry.MTime)
		})
	}
}

func TestIndexLookup_NotFound(t *testing.T) {
	t.Parallel()

	var dataBuf bytes.Buffer
	builder := NewBuilder(WithDataWriter(&dataBuf))
	fileInfo := testFileInfo{name: "file.txt", mode: 0o644, size: 3, modTime: time.Unix(1_700_000_000, 0).UTC()}
	require.NoError(t, builder.AddFile("file.txt", fileInfo, bytes.NewReader([]byte("abc"))))

	var indexBuf bytes.Buffer
	_, err := builder.Finalize(&indexBuf)
	require.NoError(t, err)

	idx, err := OpenIndex(bytes.NewReader(indexBuf.Bytes()), int64(indexBuf.Len()))
	require.NoError(t, err)

	_, ok, err := idx.Lookup("missing.txt")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestIndexLookup_InvalidPath(t *testing.T) {
	t.Parallel()

	idx := &Index{}
	_, _, err := idx.Lookup("../escape")
	require.ErrorIs(t, err, ErrInvalidPath)
}

func TestIndexLookupCache(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	fileData := []byte("cache me")

	var dataBuf bytes.Buffer
	builder := NewBuilder(WithDataWriter(&dataBuf))
	fileInfo := testFileInfo{name: "file.txt", mode: 0o644, size: int64(len(fileData)), modTime: now}
	require.NoError(t, builder.AddFile("file.txt", fileInfo, bytes.NewReader(fileData)))

	var indexBuf bytes.Buffer
	_, err := builder.Finalize(&indexBuf)
	require.NoError(t, err)

	idx, err := OpenIndex(bytes.NewReader(indexBuf.Bytes()), int64(indexBuf.Len()))
	require.NoError(t, err)

	require.NoError(t, idx.BuildLookupCache())

	entry, ok, err := idx.Lookup("file.txt")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, EntryFile, entry.Type)

	entries := idx.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "file.txt", entries[0].Path)
}

func TestIndexLookupMap(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	fileData := []byte("cache map")

	var dataBuf bytes.Buffer
	builder := NewBuilder(WithDataWriter(&dataBuf))
	fileInfo := testFileInfo{name: "file.txt", mode: 0o644, size: int64(len(fileData)), modTime: now}
	require.NoError(t, builder.AddFile("file.txt", fileInfo, bytes.NewReader(fileData)))

	var indexBuf bytes.Buffer
	_, err := builder.Finalize(&indexBuf)
	require.NoError(t, err)

	idx, err := OpenIndex(bytes.NewReader(indexBuf.Bytes()), int64(indexBuf.Len()))
	require.NoError(t, err)

	require.NoError(t, idx.BuildLookupMap())

	entry, ok, err := idx.LookupNormalized("file.txt")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, EntryFile, entry.Type)
}

func TestIndexLookupIndex(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	fileData := []byte("index lookup")

	var dataBuf bytes.Buffer
	builder := NewBuilder(WithDataWriter(&dataBuf))
	fileInfo := testFileInfo{name: "file.txt", mode: 0o644, size: int64(len(fileData)), modTime: now}
	require.NoError(t, builder.AddFile("file.txt", fileInfo, bytes.NewReader(fileData)))

	var indexBuf bytes.Buffer
	_, err := builder.Finalize(&indexBuf)
	require.NoError(t, err)

	idx, err := OpenIndex(bytes.NewReader(indexBuf.Bytes()), int64(indexBuf.Len()))
	require.NoError(t, err)
	require.NoError(t, idx.BuildLookupMap())

	entryIndex, ok, err := idx.LookupIndex("file.txt")
	require.NoError(t, err)
	require.True(t, ok)

	entry, ok := idx.EntryAt(entryIndex)
	require.True(t, ok)
	assert.Equal(t, EntryFile, entry.Type)
}

func TestNormalizePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{name: "clean path", input: "dir/file.txt", want: "dir/file.txt"},
		{name: "dot segment", input: "dir/./file.txt", want: "dir/file.txt"},
		{name: "double slash", input: "dir//file.txt", wantErr: ErrInvalidPath},
		{name: "absolute", input: "/etc/passwd", wantErr: ErrInvalidPath},
		{name: "parent", input: "../escape", wantErr: ErrInvalidPath},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NormalizePath(tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func BenchmarkIndexReadFile(b *testing.B) {
	data := []byte("benchmark data")
	var dataBuf bytes.Buffer
	builder := NewBuilder(WithDataWriter(&dataBuf))
	info := testFileInfo{name: "file.txt", mode: 0o644, size: int64(len(data)), modTime: time.Unix(1_700_000_000, 0).UTC()}
	require.NoError(b, builder.AddFile("file.txt", info, bytes.NewReader(data)))

	var indexBuf bytes.Buffer
	_, err := builder.Finalize(&indexBuf)
	require.NoError(b, err)

	idx, err := OpenIndex(bytes.NewReader(indexBuf.Bytes()), int64(indexBuf.Len()))
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry, ok, err := idx.Lookup("file.txt")
		require.NoError(b, err)
		if !ok {
			b.Fatal("entry not found")
		}
		_, _ = io.Copy(io.Discard, bytes.NewReader(dataBuf.Bytes()[entry.DataOffset:entry.DataOffset+entry.CompSize]))
	}
}
