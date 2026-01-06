package blobber

import (
	"context"
	"io"
	"io/fs"
)

// ArchiveBuilder creates eStargz blobs from files.
// This interface is implemented by internal/archive.
type ArchiveBuilder interface {
	// Build creates an eStargz blob from the given filesystem.
	// Returns the blob reader, TOC digest, and diff ID.
	Build(ctx context.Context, src fs.FS, compression Compression) (io.ReadCloser, string, error)
}

// ArchiveReader reads eStargz blobs.
// This interface is implemented by internal/archive.
type ArchiveReader interface {
	// ReadTOC extracts the TOC from an eStargz blob.
	// The size parameter is the total blob size (needed for footer location).
	ReadTOC(r io.ReaderAt, size int64) (*TOC, error)

	// OpenFile returns a reader for a specific file within an eStargz blob.
	// The offset and length are obtained from the TOC entry.
	OpenFile(r io.ReaderAt, entry TOCEntry) (io.Reader, error)
}

// TOC represents the table of contents of an eStargz blob.
type TOC struct {
	Entries []TOCEntry
}

// TOCEntry represents a file in the TOC.
type TOCEntry struct {
	Name       string
	Type       string // "reg", "dir", "symlink", etc.
	Size       int64
	Mode       int64
	Offset     int64  // Byte offset in the blob
	LinkName   string // Target for symlinks
	ChunkSize  int64
	ChunkCount int
}

// ToFileEntry converts a TOCEntry to a FileEntry.
func (e TOCEntry) ToFileEntry() FileEntry {
	return FileEntry{
		path: e.Name,
		size: e.Size,
		mode: fs.FileMode(e.Mode),
	}
}
