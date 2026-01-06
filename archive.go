package blobber

import (
	"context"
	"io"
	"io/fs"
)

// BuildResult contains the output of building an eStargz blob.
type BuildResult struct {
	// Blob is the eStargz blob reader. Caller must close when done.
	Blob io.ReadCloser
	// TOCDigest is the digest of the table of contents (for eStargz index).
	TOCDigest string
	// DiffID is the digest of the uncompressed layer content (for OCI config rootfs).
	// Per OCI spec, DiffIDs must be the digest of the uncompressed tar, not the compressed blob.
	DiffID string
	// BlobDigest is the digest of the entire compressed blob (for OCI descriptor).
	BlobDigest string
	// BlobSize is the size of the compressed blob in bytes.
	BlobSize int64
}

// ArchiveBuilder creates eStargz blobs from files.
// This interface is implemented by internal/archive.
type ArchiveBuilder interface {
	// Build creates an eStargz blob from the given filesystem.
	// Returns a BuildResult containing the blob, TOC digest, blob digest, and size.
	Build(ctx context.Context, src fs.FS, compression Compression) (*BuildResult, error)
}

// ArchiveReader reads eStargz blobs.
// This interface is implemented by internal/archive.
//
// Note: The default implementation re-parses the eStargz archive on each call
// to ReadTOC or OpenFile. For efficient repeated access to the same archive,
// use [Client.OpenImage] which caches the parsed archive in an [Image].
type ArchiveReader interface {
	// ReadTOC extracts the TOC from an eStargz blob.
	// The size parameter is the total blob size (needed for footer location).
	ReadTOC(r io.ReaderAt, size int64) (*TOC, error)

	// OpenFile returns a reader for a specific file within an eStargz blob.
	// The size parameter is the total blob size (needed for estargz.Open).
	// The caller obtains size from the registry pull operation.
	//
	// Note: Each call re-parses the archive. For multiple file access,
	// prefer [Client.OpenImage] which caches the parsed state.
	OpenFile(r io.ReaderAt, size int64, entry TOCEntry) (io.Reader, error)
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
