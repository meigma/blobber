package blobber

import (
	"context"

	"github.com/opencontainers/go-digest"
)

// Policy verifies a blob manifest meets requirements before accessing contents.
//
// Policies enable verification-before-pull workflows. A Policy inspects the
// manifest and its referrers (signatures, SBOMs, SLSA attestations) to decide
// whether to allow access to the blob contents.
//
// If Verify returns nil, the Pull or Stream operation proceeds.
// If Verify returns an error, the operation is aborted.
type Policy interface {
	Verify(ctx context.Context, manifest BlobManifest, fetcher ReferrerFetcher) error
}

// ReferrerFetcher allows policies to fetch referrer content when needed.
//
// The fetcher is scoped to a single Pull or Stream operation. Policies can
// inspect manifest.Referrers for metadata (artifact types, digests) and then
// call FetchReferrer to retrieve the actual content when needed for verification.
type ReferrerFetcher interface {
	FetchReferrer(ctx context.Context, referrerDigest digest.Digest) ([]byte, error)
}
