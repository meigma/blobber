// Package contracts defines internal interfaces shared across blobber components.
// These interfaces are intentionally internal to avoid exposing implementation
// contracts as part of the public API.
package contracts

import (
	"context"
	"io"
	"io/fs"

	"github.com/meigma/blobber/core"
)

// ArchiveBuilder creates eStargz blobs from files.
type ArchiveBuilder interface {
	// Build creates an eStargz blob from the given filesystem.
	// Returns a BuildResult containing the blob, TOC digest, blob digest, and size.
	Build(ctx context.Context, src fs.FS, compression core.Compression) (*core.BuildResult, error)
}

// ArchiveReader reads eStargz blobs.
//
// Note: The default implementation re-parses the eStargz archive on each call
// to ReadTOC or OpenFile. For efficient repeated access to the same archive,
// use Client.OpenImage which caches the parsed archive in an Image.
type ArchiveReader interface {
	// ReadTOC extracts the TOC from an eStargz blob.
	// The size parameter is the total blob size (needed for footer location).
	ReadTOC(r io.ReaderAt, size int64) (*core.TOC, error)

	// OpenFile returns a reader for a specific file within an eStargz blob.
	// The size parameter is the total blob size (needed for estargz.Open).
	// The caller obtains size from the registry pull operation.
	//
	// Note: Each call re-parses the archive. For multiple file access,
	// prefer Client.OpenImage which caches the parsed state.
	OpenFile(r io.ReaderAt, size int64, entry core.TOCEntry) (io.Reader, error)
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
	Open(ctx context.Context, ref string, desc core.LayerDescriptor) (BlobHandle, error)
	// OpenStream returns a streaming reader for the blob.
	// The ref parameter is used for fetching missing ranges during resume.
	// More efficient for sequential reads like extraction.
	OpenStream(ctx context.Context, ref string, desc core.LayerDescriptor) (io.ReadCloser, error)
}

// Extractor extracts eStargz archives to the filesystem.
type Extractor interface {
	// Extract extracts an eStargz blob to the destination directory.
	Extract(ctx context.Context, r io.Reader, destDir string, validator PathValidator, limits core.ExtractLimits) error
}

// PathValidator validates paths for security concerns.
type PathValidator interface {
	// ValidatePath checks if a path is safe (no traversal, valid characters).
	// Returns ErrPathTraversal if the path is unsafe.
	ValidatePath(path string) error

	// ValidateExtraction checks if extracting the given entries to destDir is safe.
	// Returns ErrPathTraversal for unsafe paths, ErrExtractLimits if limits exceeded.
	ValidateExtraction(destDir string, entries []core.TOCEntry, limits core.ExtractLimits) error

	// ValidateSymlink checks if a symlink target is safe (stays within destDir).
	// Returns ErrPathTraversal if the symlink would escape.
	ValidateSymlink(destDir, linkPath, target string) error
}

// Registry handles OCI registry operations.
type Registry interface {
	// Push uploads a blob and creates a manifest.
	// Returns the manifest digest.
	Push(ctx context.Context, ref string, layer io.Reader, opts *core.RegistryPushOptions) (string, error)

	// Pull returns a reader for the image's layer blob and its size.
	Pull(ctx context.Context, ref string) (io.ReadCloser, int64, error)

	// PullRange fetches a byte range from the layer blob.
	// Used for selective file retrieval from eStargz.
	PullRange(ctx context.Context, ref string, offset, length int64) (io.ReadCloser, error)

	// ResolveLayer resolves a reference to its layer descriptor.
	// Returns the layer metadata without fetching the blob content.
	// Used by the cache to check for hits before downloading.
	ResolveLayer(ctx context.Context, ref string) (core.LayerDescriptor, error)

	// FetchBlob fetches a blob by its descriptor.
	// Unlike Pull, this uses a known digest rather than resolving a ref.
	FetchBlob(ctx context.Context, ref string, desc core.LayerDescriptor) (io.ReadCloser, error)

	// FetchBlobRange fetches a byte range from a blob by its descriptor.
	// Used for resuming partial downloads and selective file access.
	// Returns ErrRangeNotSupported if the registry doesn't support range requests.
	FetchBlobRange(ctx context.Context, ref string, desc core.LayerDescriptor, offset, length int64) (io.ReadCloser, error)

	// PushReferrer pushes a referrer artifact that references the subject digest.
	// The data is stored as a single layer in an OCI manifest with the subject field set.
	// Returns the referrer manifest digest.
	PushReferrer(ctx context.Context, ref string, subjectDigest string, data []byte, opts *core.ReferrerPushOptions) (string, error)

	// FetchReferrers returns all referrers for a subject digest, optionally filtered by artifact type.
	// Uses the OCI 1.1 referrers API (GET /v2/<name>/referrers/<digest>?artifactType=...).
	// Pass empty artifactType to fetch all referrers.
	FetchReferrers(ctx context.Context, ref string, subjectDigest string, artifactType string) ([]core.Referrer, error)

	// FetchReferrer fetches the content of a specific referrer by its digest.
	// Returns the first layer's content (the signature/attestation data).
	FetchReferrer(ctx context.Context, ref string, referrerDigest string) ([]byte, error)

	// FetchManifest fetches the raw manifest bytes for a reference.
	// Returns the manifest JSON and its digest.
	FetchManifest(ctx context.Context, ref string) ([]byte, string, error)

	// ListTags returns all tags for a repository.
	// The repository should be in the format "registry/namespace/repo" (e.g., "ghcr.io/org/repo").
	ListTags(ctx context.Context, repository string) ([]string, error)
}
