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

	"github.com/meigma/blobber/v2/internal/domain"
	"github.com/meigma/blobber/v2/internal/estargz"
)

// Re-export domain types for convenience.
type (
	PushMetadata   = domain.PushMetadata
	PushResult     = domain.PushResult
	BlobManifest   = domain.BlobManifest
	BlobDescriptor = domain.BlobDescriptor
	Referrer       = domain.Referrer
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

	// ResolveRef resolves a ref to its blob digest without fetching the blob.
	// This is useful for cache lookups where only the digest is needed.
	// The ref can be a tag or digest reference.
	ResolveRef(ctx context.Context, ref string) (digest.Digest, error)

	// FetchBlob downloads the entire blob content.
	// The ref can be a tag or digest reference.
	// The caller is responsible for closing the returned reader.
	FetchBlob(ctx context.Context, ref string) (io.ReadCloser, error)

	// FetchBlobByDigest downloads the blob using a known digest.
	// The ref identifies the repository; the digest specifies which blob to fetch.
	// Use this when the digest is already known (e.g., from cache) to avoid
	// a redundant manifest fetch.
	// The caller is responsible for closing the returned reader.
	FetchBlobByDigest(ctx context.Context, ref string, d digest.Digest, size int64) (io.ReadCloser, error)

	// OpenBlob returns a reader for random access to the blob.
	// This enables streaming via TOC-based byte range requests.
	// The caller is responsible for closing the returned reader.
	OpenBlob(ctx context.Context, ref string) (estargz.BlobSource, error)

	// OpenBlobByDigest returns a reader for random access using a known digest.
	// The ref identifies the repository; the digest specifies which blob to open.
	// Use this when the digest is already known (e.g., from cache) to avoid
	// a redundant manifest fetch.
	// The caller is responsible for closing the returned reader.
	OpenBlobByDigest(ctx context.Context, ref string, d digest.Digest, size int64) (estargz.BlobSource, error)

	// AttachArtifact attaches content to a manifest as a referrer.
	// The ref identifies the target manifest (tag or digest reference).
	// Returns the referrer manifest's digest for optional follow-up operations.
	AttachArtifact(ctx context.Context, ref string, artifactType string, content []byte, annotations map[string]string) (digest.Digest, error)

	// FetchReferrerContent downloads the content of a referrer artifact.
	// The ref identifies the repository and referrerDigest is the digest of the
	// referrer manifest (as returned by AttachArtifact or found in Referrer.Digest).
	// This method fetches the referrer manifest, validates it has a single layer,
	// and returns the content of that layer.
	// This is used by Policy implementations to fetch signature or attestation content.
	FetchReferrerContent(ctx context.Context, ref string, referrerDigest digest.Digest) ([]byte, error)
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
