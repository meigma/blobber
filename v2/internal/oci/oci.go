// Package oci provides OCI registry operations.
//
// This package wraps ORAS to provide a clean interface for interacting with
// OCI registries. It handles authentication, error mapping, and provides
// types for random-access blob reading.
//
// The package focuses on pure OCI operations without any Blobber-specific
// logic. Higher-level abstractions (like single-layer manifest enforcement)
// belong in the registry package.
package oci

//go:generate go run github.com/matryer/moq@latest -out mocks/client.go -pkg mocks . Client

import (
	"context"
	"io"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/meigma/blobber/v2/internal/estargz"
)

// Client provides OCI registry operations.
type Client interface {
	// PushBlob uploads a blob to the registry.
	// The descriptor must contain the correct digest and size.
	PushBlob(ctx context.Context, ref string, desc ocispec.Descriptor, content io.Reader) error

	// FetchBlob downloads a blob from the registry.
	// The caller is responsible for closing the returned reader.
	FetchBlob(ctx context.Context, ref string, desc ocispec.Descriptor) (io.ReadCloser, error)

	// FetchBlobRange downloads a byte range from a blob.
	// Returns ErrRangeNotSupported if the registry doesn't support range requests.
	// The caller is responsible for closing the returned reader.
	FetchBlobRange(ctx context.Context, ref string, desc ocispec.Descriptor, offset, length int64) (io.ReadCloser, error)

	// PushManifest uploads a manifest to the registry.
	// The descriptor must contain the correct digest, size, and media type.
	PushManifest(ctx context.Context, ref string, desc ocispec.Descriptor, content []byte) error

	// FetchManifest downloads a manifest from the registry.
	// Returns the raw manifest bytes and its descriptor.
	// The caller is responsible for determining the manifest type and decoding it.
	FetchManifest(ctx context.Context, ref string) ([]byte, ocispec.Descriptor, error)

	// ResolveManifest resolves a reference to its manifest descriptor without fetching content.
	ResolveManifest(ctx context.Context, ref string) (ocispec.Descriptor, error)

	// Tag associates a tag with a manifest.
	Tag(ctx context.Context, ref string, desc ocispec.Descriptor, tag string) error

	// PushReferrer uploads a referrer artifact that references a subject.
	// The artifact descriptor describes the content being pushed.
	// The subject descriptor identifies what this artifact refers to.
	// Returns the referrer manifest digest for optional follow-up operations (e.g., signing).
	PushReferrer(ctx context.Context, ref string, subject ocispec.Descriptor, artifact ocispec.Descriptor, content []byte) (digest.Digest, error)

	// ListReferrers returns all referrers for a subject digest.
	// If artifactType is non-empty, only referrers of that type are returned.
	ListReferrers(ctx context.Context, ref string, subjectDigest string, artifactType string) ([]ocispec.Descriptor, error)

	// BlobReader creates a BlobSource for random access to a remote blob.
	// The returned reader issues HTTP range requests for each ReadAt call.
	// The caller is responsible for closing the returned reader.
	BlobReader(ctx context.Context, ref string, desc ocispec.Descriptor) (estargz.BlobSource, error)
}
