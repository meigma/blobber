// Package registry provides Blob storage operations on OCI registries.
//
// This package enforces Blobber's single-layer manifest constraint and works
// with domain types rather than raw OCI structures. It sits between the
// Blobber client and the lower-level OCI client.
package registry

//go:generate go run github.com/matryer/moq@latest -out mocks/registry.go -pkg mocks . Registry

import (
	"context"
	"io"

	"github.com/opencontainers/go-digest"

	"github.com/meigma/blobber/v2/internal/oci"
)

// Registry provides Blob storage operations on OCI registries.
type Registry interface {
	// Push uploads a blob and creates a single-layer manifest.
	// The ref must include a tag (e.g., "ghcr.io/org/repo:v1").
	// Returns the push result containing the digest reference and manifest metadata.
	Push(ctx context.Context, ref string, blob io.Reader, metadata PushMetadata) (*PushResult, error)

	// FetchManifest retrieves a blob's manifest and its referrers.
	// The ref can be a tag or digest reference.
	// By default, referrer metadata is eagerly fetched.
	// Use WithoutReferrers to skip referrer fetching for performance-critical paths.
	FetchManifest(ctx context.Context, ref string, opts ...FetchOption) (*BlobManifest, error)

	// FetchBlob downloads the entire blob content.
	// The ref can be a tag or digest reference.
	// The caller is responsible for closing the returned reader.
	FetchBlob(ctx context.Context, ref string) (io.ReadCloser, error)

	// OpenBlob returns a reader for random access to the blob.
	// This enables streaming via TOC-based byte range requests.
	// The caller is responsible for closing the returned reader.
	OpenBlob(ctx context.Context, ref string) (*oci.BlobReader, error)

	// AttachArtifact attaches content to a manifest as a referrer.
	// The ref identifies the target manifest (tag or digest reference).
	// Returns the referrer manifest's digest for optional follow-up operations.
	AttachArtifact(ctx context.Context, ref string, artifactType string, content []byte, annotations map[string]string) (digest.Digest, error)
}

// PushMetadata contains the metadata needed to create a blob manifest.
type PushMetadata struct {
	// BlobDigest is the SHA-256 digest of the compressed blob.
	BlobDigest digest.Digest

	// BlobSize is the size of the compressed blob in bytes.
	BlobSize int64

	// UncompressedDigest is the SHA-256 digest of the uncompressed tar content (DiffID).
	UncompressedDigest digest.Digest

	// TOCDigest is the SHA-256 digest of the Table of Contents.
	TOCDigest digest.Digest

	// MediaType is the blob's media type (e.g., "application/vnd.oci.image.layer.v1.tar+gzip").
	MediaType string
}

// PushResult contains the result of a Push operation.
type PushResult struct {
	// Reference is the digest reference (e.g., "ghcr.io/org/repo@sha256:abc...").
	Reference string

	// Manifest is the pushed blob's manifest metadata.
	Manifest BlobManifest
}

// BlobManifest represents metadata about a pushed blob's manifest.
type BlobManifest struct {
	// Digest is the manifest's SHA-256 digest.
	Digest digest.Digest

	// Size is the manifest size in bytes.
	Size int64

	// Blob contains the blob's descriptor information.
	Blob BlobDescriptor

	// Referrers contains attached artifacts (signatures, SBOMs, etc.).
	// This is empty if fetched with WithoutReferrers.
	Referrers []Referrer
}

// BlobDescriptor contains metadata about the blob itself.
type BlobDescriptor struct {
	// Digest is the blob's SHA-256 digest.
	Digest digest.Digest

	// Size is the blob size in bytes.
	Size int64

	// MediaType is the blob's media type.
	MediaType string
}

// Referrer represents an artifact attached to a manifest.
type Referrer struct {
	// Digest is the referrer manifest's digest.
	Digest digest.Digest

	// ArtifactType identifies the kind of artifact (e.g., "application/spdx+json").
	ArtifactType string

	// Size is the referrer manifest size in bytes.
	Size int64

	// Annotations contains optional metadata.
	Annotations map[string]string
}

// FetchOption configures FetchManifest behavior.
type FetchOption func(*fetchOptions)

type fetchOptions struct {
	skipReferrers bool
}

// WithoutReferrers skips fetching referrer metadata.
// Use this for performance-critical paths where referrers aren't needed.
func WithoutReferrers() FetchOption {
	return func(o *fetchOptions) {
		o.skipReferrers = true
	}
}
