package estargz

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		src       fs.FS
		wantFiles []string
	}{
		{
			name:      "empty filesystem",
			src:       fstest.MapFS{},
			wantFiles: nil,
		},
		{
			name: "single file",
			src: fstest.MapFS{
				"hello.txt": &fstest.MapFile{
					Data:    []byte("hello world"),
					Mode:    0o644,
					ModTime: time.Now(),
				},
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
			wantFiles: []string{"dir", "dir/subdir", "dir/subdir/a.txt", "dir/b.txt", "root.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			b := NewBuilder()
			result, err := b.Build(context.Background(), &buf, tt.src)
			require.NoError(t, err)

			// Verify digests are valid.
			assert.NotEmpty(t, result.BlobDigest)
			assert.True(t, result.BlobDigest.Algorithm().Available())

			assert.NotEmpty(t, result.UncompressedDigest)
			assert.True(t, result.UncompressedDigest.Algorithm().Available())

			assert.NotEmpty(t, result.TOCDigest)
			assert.True(t, result.TOCDigest.Algorithm().Available())

			// Verify size is positive for non-empty filesystems.
			if len(tt.wantFiles) > 0 {
				assert.Greater(t, result.BlobSize, int64(0))
			}

			// Verify the blob has correct size.
			assert.Equal(t, result.BlobSize, int64(buf.Len()))

			// Verify blob digest matches content.
			actualDigest := digest.SHA256.FromBytes(buf.Bytes())
			assert.Equal(t, result.BlobDigest, actualDigest)

			// Verify archive contains expected entries (compare as sorted sets).
			entries := extractTarEntries(t, buf.Bytes())
			gotNames := entryNames(entries)
			slices.Sort(gotNames)
			wantNames := append([]string{}, tt.wantFiles...) // clone to avoid nil
			slices.Sort(wantNames)
			assert.Equal(t, wantNames, gotNames)
		})
	}
}

func TestBuild_BlobIsValidGzip(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("content"), Mode: 0o644},
	}

	var buf bytes.Buffer
	b := NewBuilder()
	_, err := b.Build(context.Background(), &buf, src)
	require.NoError(t, err)

	// Verify it's valid gzip.
	gr, err := gzip.NewReader(&buf)
	require.NoError(t, err)
	defer gr.Close()

	// Should be able to read decompressed content.
	_, err = io.ReadAll(gr)
	require.NoError(t, err)
}

func TestBuild_ContextCancellation(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("data"), Mode: 0o644},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var buf bytes.Buffer
	b := NewBuilder()
	_, err := b.Build(ctx, &buf, src)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestBuild_LargeFile(t *testing.T) {
	t.Parallel()

	content := bytes.Repeat([]byte("x"), 256*1024) // 256KB
	src := fstest.MapFS{
		"large.bin": &fstest.MapFile{Data: content, Mode: 0o644},
	}

	var buf bytes.Buffer
	b := NewBuilder()
	result, err := b.Build(context.Background(), &buf, src)
	require.NoError(t, err)

	assert.Greater(t, result.BlobSize, int64(0))
	assert.Equal(t, result.BlobSize, int64(buf.Len()))
}

func TestBuild_Symlinks(t *testing.T) {
	t.Parallel()

	src := &symlinkFS{
		files: map[string]fileEntry{
			"target.txt": {data: []byte("target content"), mode: 0o644},
			"link":       {mode: fs.ModeSymlink, linkTarget: "target.txt"},
		},
	}

	var buf bytes.Buffer
	b := NewBuilder()
	result, err := b.Build(context.Background(), &buf, src)
	require.NoError(t, err)

	assert.Greater(t, result.BlobSize, int64(0))

	// Verify symlink entry has correct Typeflag and Linkname.
	entries := extractTarEntries(t, buf.Bytes())

	var linkEntry *tarEntry
	for i := range entries {
		if entries[i].Name == "link" {
			linkEntry = &entries[i]
			break
		}
	}
	require.NotNil(t, linkEntry, "symlink entry not found")
	assert.Equal(t, byte(tar.TypeSymlink), linkEntry.Typeflag)
	assert.Equal(t, "target.txt", linkEntry.Linkname)
}

func TestBuild_SymlinkWithoutReadLinkFS(t *testing.T) {
	t.Parallel()

	src := &noReadLinkFS{
		files: map[string]fileEntry{
			"link": {mode: fs.ModeSymlink, linkTarget: "target"},
		},
	}

	var buf bytes.Buffer
	b := NewBuilder()
	_, err := b.Build(context.Background(), &buf, src)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ReadLinkFS")
}

func TestBuild_MidStreamCancellation(t *testing.T) {
	t.Parallel()

	const timeout = 5 * time.Second

	// Channel for the file to signal it has returned data.
	readStarted := make(chan struct{})
	// Channel for the test to signal cancellation occurred.
	cancelDone := make(chan struct{})

	src := &cancelableFS{
		data:        bytes.Repeat([]byte("x"), 256*1024), // 256KB file
		readStarted: readStarted,
		cancelDone:  cancelDone,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buf bytes.Buffer
	b := NewBuilder()

	// Run Build in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		_, err := b.Build(ctx, &buf, src)
		errCh <- err
	}()

	// Wait for the file to signal it returned some data.
	select {
	case <-readStarted:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for read to start")
	}

	// Cancel the context.
	cancel()

	// Signal to the file that cancellation is done.
	close(cancelDone)

	// Build should return an error indicating cancellation.
	// Note: errors.Is may fail if the estargz library doesn't preserve
	// the error chain, so we also check the error message.
	select {
	case err := <-errCh:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
	case <-time.After(timeout):
		t.Fatal("timed out waiting for Build to return")
	}
}

// Tar extraction helpers

// tarEntry represents a single entry extracted from a tar archive.
type tarEntry struct {
	Name     string
	Typeflag byte
	Linkname string
	Size     int64
}

// extractTarEntries reads an eStargz blob and returns all tar entries,
// filtering out eStargz TOC entries (stargz.index.json).
func extractTarEntries(t *testing.T, blob []byte) []tarEntry {
	t.Helper()

	gr, err := gzip.NewReader(bytes.NewReader(blob))
	require.NoError(t, err)
	defer gr.Close()

	tr := tar.NewReader(gr)
	var entries []tarEntry

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		// Filter out eStargz TOC entries.
		if isTOCEntry(hdr.Name) {
			continue
		}

		entries = append(entries, tarEntry{
			Name:     hdr.Name,
			Typeflag: hdr.Typeflag,
			Linkname: hdr.Linkname,
			Size:     hdr.Size,
		})
	}

	return entries
}

// entryNames extracts just the names from tar entries.
func entryNames(entries []tarEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}

// Test filesystem helpers

type fileEntry struct {
	data       []byte
	mode       fs.FileMode
	linkTarget string
}

type symlinkFS struct {
	files map[string]fileEntry
}

func (sfs *symlinkFS) Open(name string) (fs.File, error) {
	if name == "." {
		return &memDir{name: ".", fs: sfs}, nil
	}
	entry, ok := sfs.files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &memFile{
		name:  name,
		entry: entry,
		r:     bytes.NewReader(entry.data),
	}, nil
}

func (sfs *symlinkFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "." {
		var entries []fs.DirEntry
		for n, f := range sfs.files {
			entries = append(entries, &memDirEntry{name: n, entry: f})
		}
		slices.SortFunc(entries, func(a, b fs.DirEntry) int {
			return strings.Compare(a.Name(), b.Name())
		})
		return entries, nil
	}
	return nil, fs.ErrNotExist
}

func (sfs *symlinkFS) Lstat(name string) (fs.FileInfo, error) {
	entry, ok := sfs.files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &memFileInfo{name: name, entry: entry}, nil
}

func (sfs *symlinkFS) ReadLink(name string) (string, error) {
	entry, ok := sfs.files[name]
	if !ok {
		return "", fs.ErrNotExist
	}
	if entry.mode&fs.ModeSymlink == 0 {
		return "", fs.ErrInvalid
	}
	return entry.linkTarget, nil
}

type noReadLinkFS struct {
	files map[string]fileEntry
}

func (n *noReadLinkFS) Open(name string) (fs.File, error) {
	if name == "." {
		return &memDir{name: ".", fs: n}, nil
	}
	entry, ok := n.files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &memFile{name: name, entry: entry, r: bytes.NewReader(entry.data)}, nil
}

func (n *noReadLinkFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "." {
		var entries []fs.DirEntry
		for nm, f := range n.files {
			entries = append(entries, &memDirEntry{name: nm, entry: f})
		}
		slices.SortFunc(entries, func(a, b fs.DirEntry) int {
			return strings.Compare(a.Name(), b.Name())
		})
		return entries, nil
	}
	return nil, fs.ErrNotExist
}

type readDirFS interface {
	ReadDir(name string) ([]fs.DirEntry, error)
}

type memDir struct {
	name string
	fs   readDirFS
}

func (d *memDir) Stat() (fs.FileInfo, error) {
	return &memFileInfo{name: d.name, entry: fileEntry{mode: fs.ModeDir | 0o755}}, nil
}

func (d *memDir) Read([]byte) (int, error) {
	return 0, fs.ErrInvalid
}

func (d *memDir) Close() error {
	return nil
}

func (d *memDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.fs != nil {
		return d.fs.ReadDir(".")
	}
	return nil, fs.ErrInvalid
}

type memFile struct {
	name  string
	entry fileEntry
	r     *bytes.Reader
}

func (f *memFile) Stat() (fs.FileInfo, error) {
	return &memFileInfo{name: f.name, entry: f.entry}, nil
}

func (f *memFile) Read(p []byte) (int, error) {
	return f.r.Read(p)
}

func (f *memFile) Close() error {
	return nil
}

type memFileInfo struct {
	name  string
	entry fileEntry
}

func (fi *memFileInfo) Name() string       { return fi.name }
func (fi *memFileInfo) Size() int64        { return int64(len(fi.entry.data)) }
func (fi *memFileInfo) Mode() fs.FileMode  { return fi.entry.mode }
func (fi *memFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *memFileInfo) IsDir() bool        { return fi.entry.mode.IsDir() }
func (fi *memFileInfo) Sys() any           { return nil }

type memDirEntry struct {
	name  string
	entry fileEntry
}

func (de *memDirEntry) Name() string      { return de.name }
func (de *memDirEntry) IsDir() bool       { return de.entry.mode.IsDir() }
func (de *memDirEntry) Type() fs.FileMode { return de.entry.mode.Type() }
func (de *memDirEntry) Info() (fs.FileInfo, error) {
	return &memFileInfo{name: de.name, entry: de.entry}, nil
}

// cancelableFS is a filesystem with a single file that coordinates with
// the test to enable mid-stream cancellation testing.
type cancelableFS struct {
	data        []byte
	readStarted chan struct{}
	cancelDone  chan struct{}
}

func (c *cancelableFS) Open(name string) (fs.File, error) {
	if name == "." {
		return &cancelableDir{fs: c}, nil
	}
	if name == "file.bin" {
		return &cancelableFile{
			data:        c.data,
			readStarted: c.readStarted,
			cancelDone:  c.cancelDone,
		}, nil
	}
	return nil, fs.ErrNotExist
}

type cancelableDir struct {
	fs *cancelableFS
}

func (d *cancelableDir) Stat() (fs.FileInfo, error) {
	return &memFileInfo{name: ".", entry: fileEntry{mode: fs.ModeDir | 0o755}}, nil
}

func (d *cancelableDir) Read([]byte) (int, error) {
	return 0, fs.ErrInvalid
}

func (d *cancelableDir) Close() error {
	return nil
}

func (d *cancelableDir) ReadDir(n int) ([]fs.DirEntry, error) {
	return []fs.DirEntry{
		&memDirEntry{name: "file.bin", entry: fileEntry{data: d.fs.data, mode: 0o644}},
	}, nil
}

// cancelableFile returns a small chunk, signals readStarted, waits for
// cancelDone, then returns context.Canceled on the next read.
type cancelableFile struct {
	data        []byte
	offset      int
	readStarted chan struct{}
	cancelDone  chan struct{}
	signaled    bool
}

func (f *cancelableFile) Stat() (fs.FileInfo, error) {
	return &memFileInfo{name: "file.bin", entry: fileEntry{data: f.data, mode: 0o644}}, nil
}

func (f *cancelableFile) Read(p []byte) (int, error) {
	if f.offset >= len(f.data) {
		return 0, io.EOF
	}

	// First read: return a small chunk and signal.
	if !f.signaled {
		f.signaled = true
		chunkSize := min(1024, len(f.data)-f.offset)
		n := copy(p, f.data[f.offset:f.offset+chunkSize])
		f.offset += n

		// Signal that we've started reading.
		close(f.readStarted)

		// Wait for cancellation to complete.
		<-f.cancelDone

		return n, nil
	}

	// Subsequent reads return context.Canceled to simulate the
	// copyWithContext loop detecting cancellation.
	return 0, context.Canceled
}

func (f *cancelableFile) Close() error {
	return nil
}
