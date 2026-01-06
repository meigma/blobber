package blobber

import (
	"io/fs"
	"path/filepath"
	"time"
)

// FileEntry represents a file in a remote image.
// Implements fs.DirEntry for use with Walk.
type FileEntry struct {
	path string
	size int64
	mode fs.FileMode
}

// fileInfo implements fs.FileInfo for FileEntry.
type fileInfo struct {
	entry FileEntry
}

// Info returns the FileInfo for the file.
func (f FileEntry) Info() (fs.FileInfo, error) {
	return fileInfo{f}, nil
}

// IsDir reports whether the entry describes a directory.
func (f FileEntry) IsDir() bool {
	return f.mode.IsDir()
}

// Mode returns the file mode.
func (f FileEntry) Mode() fs.FileMode {
	return f.mode
}

// Name returns the base name of the file.
func (f FileEntry) Name() string {
	return filepath.Base(f.path)
}

// Path returns the full path of the file within the image.
func (f FileEntry) Path() string {
	return f.path
}

// Size returns the size of the file in bytes.
func (f FileEntry) Size() int64 {
	return f.size
}

// Type returns the type bits of the file mode.
func (f FileEntry) Type() fs.FileMode {
	return f.mode.Type()
}

func (fi fileInfo) IsDir() bool        { return fi.entry.IsDir() }
func (fi fileInfo) ModTime() time.Time { return time.Time{} } // Not available from TOC
func (fi fileInfo) Mode() fs.FileMode  { return fi.entry.mode }
func (fi fileInfo) Name() string       { return fi.entry.Name() }
func (fi fileInfo) Size() int64        { return fi.entry.size }
func (fi fileInfo) Sys() any           { return nil }
