// Package core provides shared data types and errors for blobber.
// Interfaces that define internal contracts live in internal/contracts to avoid
// exposing them as part of the public API.
package core

import (
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

	// ErrSignatureInvalid indicates signature verification failed.
	ErrSignatureInvalid = errors.New("blobber: signature verification failed")

	// ErrNoSignature indicates no signature was found when verification was required.
	ErrNoSignature = errors.New("blobber: no signature found")
)

// Compression provides compression/decompression for eStargz blobs.
// This is a type alias for estargz.Compression, allowing custom implementations.
//
// Use GzipCompression() or ZstdCompression() for built-in implementations.
type Compression = estargz.Compression

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

// ExtractLimits defines safety limits for extraction.
type ExtractLimits struct {
	MaxFiles     int   // Maximum number of files (0 = no limit)
	MaxTotalSize int64 // Maximum total extracted size (0 = no limit)
	MaxFileSize  int64 // Maximum single file size (0 = no limit)
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

// Referrer represents an OCI referrer artifact (e.g., signature, attestation).
type Referrer struct {
	// Digest is the referrer manifest digest.
	Digest string
	// ArtifactType identifies the referrer type (e.g., "application/vnd.dev.sigstore.bundle.v0.3+json").
	ArtifactType string
	// Annotations are optional key-value metadata.
	Annotations map[string]string
}

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

// ReferrerPushOptions configures how a referrer artifact is pushed.
type ReferrerPushOptions struct {
	// ArtifactType is the OCI artifact type (required).
	ArtifactType string
	// Annotations are optional manifest annotations.
	Annotations map[string]string
}
