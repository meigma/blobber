// Package core provides the shared types and interfaces for blobber.
//
// This package exists to break import cycles between the root blobber package
// and internal implementation packages. The blobber package re-exports all
// public types from this package, so external users should import blobber
// directly, not blobber/core.
package core

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"time"

	"github.com/containerd/stargz-snapshotter/estargz"
)

// Sentinel errors for common failure conditions.
var (
	// ErrNotFound indicates the requested image or file was not found.
	ErrNotFound = errors.New("blobber: not found")

	// ErrUnauthorized indicates authentication failed.
	ErrUnauthorized = errors.New("blobber: unauthorized")

	// ErrInvalidRef indicates the image reference is malformed.
	ErrInvalidRef = errors.New("blobber: invalid reference")

	// ErrPathTraversal indicates a path traversal attack was detected.
	ErrPathTraversal = errors.New("blobber: path traversal detected")

	// ErrExtractLimits indicates extraction safety limits were exceeded.
	ErrExtractLimits = errors.New("blobber: extraction limits exceeded")

	// ErrInvalidArchive indicates the blob is not a valid eStargz archive.
	ErrInvalidArchive = errors.New("blobber: invalid eStargz archive")

	// ErrClosed indicates an operation was attempted on a closed resource.
	ErrClosed = errors.New("blobber: resource closed")

	// ErrRangeNotSupported indicates the registry does not support range requests.
	ErrRangeNotSupported = errors.New("blobber: range requests not supported")
)

// ExtractLimits defines safety limits for extraction.
type ExtractLimits struct {
	MaxFiles     int   // Maximum number of files (0 = no limit)
	MaxTotalSize int64 // Maximum total extracted size (0 = no limit)
	MaxFileSize  int64 // Maximum single file size (0 = no limit)
}

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
func (e *TOCEntry) ToFileEntry() FileEntry {
	return FileEntry{
		FilePath: e.Name,
		FileSize: e.Size,
		//nolint:gosec // G115: Mode from trusted estargz TOC entry
		FileMode: fs.FileMode(e.Mode),
	}
}

// FileEntry represents a file in a remote image.
// Implements fs.DirEntry for use with Walk.
type FileEntry struct {
	FilePath string
	FileSize int64
	FileMode fs.FileMode
}

// Name returns the base name of the file.
func (f FileEntry) Name() string { return f.FilePath }

// Path returns the full path of the file within the image.
func (f FileEntry) Path() string { return f.FilePath }

// Size returns the size of the file in bytes.
func (f FileEntry) Size() int64 { return f.FileSize }

// Mode returns the file mode.
func (f FileEntry) Mode() fs.FileMode { return f.FileMode }

// IsDir reports whether the entry describes a directory.
func (f FileEntry) IsDir() bool { return f.FileMode.IsDir() }

// Type returns the type bits for the entry.
func (f FileEntry) Type() fs.FileMode { return f.FileMode.Type() }

// Info returns the FileInfo for the file.
func (f FileEntry) Info() (fs.FileInfo, error) {
	return fileInfo{f}, nil
}

// fileInfo implements fs.FileInfo for FileEntry.
// It wraps a FileEntry to satisfy the fs.FileInfo interface returned by
// FileEntry.Info(), enabling FileEntry to fully implement fs.DirEntry.
type fileInfo struct {
	entry FileEntry
}

func (fi fileInfo) Name() string       { return fi.entry.FilePath }
func (fi fileInfo) Size() int64        { return fi.entry.FileSize }
func (fi fileInfo) Mode() fs.FileMode  { return fi.entry.FileMode }
func (fi fileInfo) ModTime() time.Time { return time.Time{} }
func (fi fileInfo) IsDir() bool        { return fi.entry.FileMode.IsDir() }
func (fi fileInfo) Sys() any           { return nil }

// RegistryPushOptions contains metadata for push operations.
type RegistryPushOptions struct {
	MediaType   string
	Annotations map[string]string
	TOCDigest   string

	// DiffID is the digest of the uncompressed layer content (required for OCI config).
	// Per OCI spec, this must be the uncompressed tar digest, not the compressed blob digest.
	DiffID string
	// BlobDigest is the pre-computed digest of the compressed blob (required for streaming push).
	BlobDigest string
	// BlobSize is the pre-computed size of the compressed blob in bytes (required for streaming push).
	BlobSize int64
}

// Registry handles OCI registry operations.
// This interface is implemented by internal/registry.
type Registry interface {
	// Push uploads a blob and creates a manifest.
	// Returns the manifest digest.
	Push(ctx context.Context, ref string, layer io.Reader, opts *RegistryPushOptions) (string, error)

	// Pull returns a reader for the image's layer blob and its size.
	Pull(ctx context.Context, ref string) (io.ReadCloser, int64, error)

	// PullRange fetches a byte range from the layer blob.
	// Used for selective file retrieval from eStargz.
	PullRange(ctx context.Context, ref string, offset, length int64) (io.ReadCloser, error)

	// ResolveLayer resolves a reference to its layer descriptor.
	// Returns the layer metadata without fetching the blob content.
	// Used by the cache to check for hits before downloading.
	ResolveLayer(ctx context.Context, ref string) (LayerDescriptor, error)

	// FetchBlob fetches a blob by its descriptor.
	// Unlike Pull, this uses a known digest rather than resolving a ref.
	FetchBlob(ctx context.Context, ref string, desc LayerDescriptor) (io.ReadCloser, error)

	// FetchBlobRange fetches a byte range from a blob by its descriptor.
	// Used for resuming partial downloads and selective file access.
	// Returns ErrRangeNotSupported if the registry doesn't support range requests.
	FetchBlobRange(ctx context.Context, ref string, desc LayerDescriptor, offset, length int64) (io.ReadCloser, error)
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

// Extractor extracts eStargz archives to the filesystem.
// This interface is implemented by internal/archive.
type Extractor interface {
	// Extract extracts an eStargz blob to the destination directory.
	Extract(ctx context.Context, r io.Reader, destDir string, validator PathValidator, limits ExtractLimits) error
}

// PathValidator validates paths for security concerns.
// This interface is implemented by internal/safepath.
type PathValidator interface {
	// ValidatePath checks if a path is safe (no traversal, valid characters).
	// Returns ErrPathTraversal if the path is unsafe.
	ValidatePath(path string) error

	// ValidateExtraction checks if extracting the given entries to destDir is safe.
	// Returns ErrPathTraversal for unsafe paths, ErrExtractLimits if limits exceeded.
	ValidateExtraction(destDir string, entries []TOCEntry, limits ExtractLimits) error

	// ValidateSymlink checks if a symlink target is safe (stays within destDir).
	// Returns ErrPathTraversal if the symlink would escape.
	ValidateSymlink(destDir, linkPath, target string) error
}

// Compression provides compression/decompression for eStargz blobs.
// This is a type alias for estargz.Compression, allowing custom implementations.
//
// Use GzipCompression() or ZstdCompression() for built-in implementations.
type Compression = estargz.Compression

// LayerDescriptor captures the resolved layer metadata plus platform context.
// Used as the cache key and for blob retrieval operations.
type LayerDescriptor struct {
	// Digest is the SHA256 digest of the compressed blob (sha256:...).
	// This is the primary cache key.
	Digest string
	// Size is the total blob size in bytes.
	Size int64
	// MediaType is the OCI media type of the layer.
	MediaType string
	// ManifestDigest is the digest of the manifest that contained this layer.
	// Used for tag drift detection during partial caching.
	ManifestDigest string
	// Platform is the target platform in os/arch[/variant] format.
	Platform string
}

// BlobHandle provides random access to a cached or remote blob.
// Implements io.ReaderAt for estargz compatibility.
type BlobHandle interface {
	io.ReaderAt
	io.Closer
	// Size returns the total blob size in bytes.
	Size() int64
	// Complete reports whether the entire blob is available locally.
	Complete() bool
}

// BlobSource provides access to blobs, either from cache or network.
// The cache implementation wraps a fallback source (typically the registry).
type BlobSource interface {
	// Open returns a BlobHandle for random access to the blob.
	// The ref parameter is used for fetching missing ranges during resume.
	// The handle must be closed when done.
	Open(ctx context.Context, ref string, desc LayerDescriptor) (BlobHandle, error)
	// OpenStream returns a streaming reader for the blob.
	// The ref parameter is used for fetching missing ranges during resume.
	// More efficient for sequential reads like extraction.
	OpenStream(ctx context.Context, ref string, desc LayerDescriptor) (io.ReadCloser, error)
}
