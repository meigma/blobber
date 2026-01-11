package tar

import (
	"archive/tar"
	"bytes"
	"io"
	"io/fs"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// tarEntry represents a parsed tar entry for testing.
type tarEntry struct {
	name     string
	typeflag byte
	content  string
	linkname string
	mode     int64
}

// readTarEntries reads all entries from a tar archive.
func readTarEntries(t *testing.T, r io.Reader) []tarEntry {
	t.Helper()

	tr := tar.NewReader(r)
	var entries []tarEntry

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		entry := tarEntry{
			name:     header.Name,
			typeflag: header.Typeflag,
			linkname: header.Linkname,
			mode:     header.Mode,
		}

		if header.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			require.NoError(t, err)
			entry.content = string(data)
		}

		entries = append(entries, entry)
	}

	return entries
}

// extractNames returns the names from a slice of tar entries.
func extractNames(entries []tarEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	return names
}

// fileEntry represents a file in test filesystems.
type fileEntry struct {
	data       []byte
	mode       fs.FileMode
	linkTarget string
}

// symlinkFS is a test filesystem that supports symlinks.
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

// noReadLinkFS is a test filesystem that supports Lstat but not ReadLink.
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
		return entries, nil
	}
	return nil, fs.ErrNotExist
}

func (n *noReadLinkFS) Lstat(name string) (fs.FileInfo, error) {
	entry, ok := n.files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &memFileInfo{name: name, entry: entry}, nil
}

// readDirFS is an interface for filesystems that support ReadDir.
type readDirFS interface {
	ReadDir(name string) ([]fs.DirEntry, error)
}

// memDir implements fs.File for directories.
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

// memFile implements fs.File for testing.
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

// memFileInfo implements fs.FileInfo for testing.
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

// memDirEntry implements fs.DirEntry for testing.
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

// symlinkTypeOnlyFS is a filesystem that reports symlinks but doesn't support ReadLink.
type symlinkTypeOnlyFS struct {
	opened bool
}

func (s *symlinkTypeOnlyFS) Open(name string) (fs.File, error) {
	if name == "." {
		return &memDir{name: "."}, nil
	}
	s.opened = true
	return nil, fs.ErrInvalid
}

func (s *symlinkTypeOnlyFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "." {
		return []fs.DirEntry{&symlinkTypeOnlyEntry{name: "link"}}, nil
	}
	return nil, fs.ErrNotExist
}

func (s *symlinkTypeOnlyFS) Stat(name string) (fs.FileInfo, error) {
	if name == "." {
		return &memFileInfo{name: ".", entry: fileEntry{mode: fs.ModeDir | 0o755}}, nil
	}
	if name == "link" {
		return &memFileInfo{name: "link", entry: fileEntry{data: []byte("payload"), mode: 0o644}}, nil
	}
	return nil, fs.ErrNotExist
}

// symlinkTypeOnlyEntry is a DirEntry that reports symlink type.
type symlinkTypeOnlyEntry struct {
	name string
}

func (e *symlinkTypeOnlyEntry) Name() string      { return e.name }
func (e *symlinkTypeOnlyEntry) IsDir() bool       { return false }
func (e *symlinkTypeOnlyEntry) Type() fs.FileMode { return fs.ModeSymlink }
func (e *symlinkTypeOnlyEntry) Info() (fs.FileInfo, error) {
	return &memFileInfo{name: e.name, entry: fileEntry{data: []byte("payload"), mode: 0o644}}, nil
}
