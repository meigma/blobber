// Package domain defines shared types for the Blobber domain model.
//
// These types represent the core abstractions that flow between packages.
// This package has no dependencies on other internal packages to avoid cycles.
package domain

import "github.com/opencontainers/go-digest"

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
	// This may be empty if referrers were not fetched.
	Referrers []Referrer

	// Raw contains the original manifest JSON bytes.
	// This is used for signing and verification.
	Raw []byte
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
