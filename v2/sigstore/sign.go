package sigstore

import (
	"context"
	"fmt"

	"github.com/opencontainers/go-digest"

	blobber "github.com/meigma/blobber/v2"
)

// Sign signs a blob's manifest and attaches the signature as a referrer.
//
// This function fetches the manifest for the given reference, signs it using
// the provided signer, and attaches the signature as an OCI referrer artifact.
//
// Returns the digest of the signature referrer manifest.
//
// Example:
//
//	result, _ := client.Push(ctx, "ghcr.io/org/repo:v1", myFS)
//	sigDigest, err := sigstore.Sign(ctx, client, result.Reference, signer)
func Sign(ctx context.Context, client *blobber.Client, ref string, signer *Signer) (digest.Digest, error) {
	// Fetch the manifest to get the raw bytes for signing.
	manifest, err := client.FetchManifest(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("fetch manifest: %w", err)
	}

	// Sign the manifest bytes.
	sig, err := signer.Sign(ctx, manifest.Digest, manifest.Raw)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}

	// Attach the signature as a referrer.
	referrerDigest, err := client.AttachArtifact(ctx, ref, ArtifactType, sig.Data, nil)
	if err != nil {
		return "", fmt.Errorf("attach signature: %w", err)
	}

	return referrerDigest, nil
}
