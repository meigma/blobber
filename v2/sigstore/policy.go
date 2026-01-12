package sigstore

import (
	"context"
	"fmt"

	blobber "github.com/meigma/blobber/v2"
)

// RequireSignature returns a Policy that requires at least one valid signature.
//
// The policy inspects the manifest's referrers for sigstore signatures and
// attempts to verify each one. If at least one signature verifies successfully,
// the policy passes. Otherwise, it returns an error.
//
// Example:
//
//	verifier, _ := sigstore.NewVerifier(
//	    sigstore.WithIdentity("https://accounts.google.com", "user@example.com"),
//	)
//	policy := sigstore.RequireSignature(verifier)
//	handle, err := client.Pull(ctx, ref, blobber.WithPullPolicy(policy))
func RequireSignature(v *Verifier) blobber.Policy {
	return &signaturePolicy{verifier: v}
}

type signaturePolicy struct {
	verifier *Verifier
}

func (p *signaturePolicy) Verify(ctx context.Context, manifest blobber.BlobManifest, fetcher blobber.ReferrerFetcher) error {
	// Find signature referrers.
	var signatures []blobber.Referrer
	for _, r := range manifest.Referrers {
		if r.ArtifactType == ArtifactType {
			signatures = append(signatures, r)
		}
	}

	if len(signatures) == 0 {
		return ErrNoSignature
	}

	// Try to verify at least one signature.
	var lastErr error
	for _, sig := range signatures {
		content, err := fetcher.FetchReferrer(ctx, sig.Digest)
		if err != nil {
			lastErr = err
			continue
		}

		s := &Signature{Data: content, MediaType: sig.ArtifactType}
		if err := p.verifier.Verify(ctx, manifest.Digest, manifest.Raw, s); err != nil {
			lastErr = err
			continue
		}

		return nil // Success - at least one signature verified.
	}

	return fmt.Errorf("%w: %v", ErrSignatureInvalid, lastErr)
}
