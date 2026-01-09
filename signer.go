// Package blobber provides signing and verification interfaces for OCI artifacts.
package blobber

import (
	"context"

	"github.com/opencontainers/go-digest"
)

// SignatureArtifactType is the OCI artifact type for sigstore bundles.
const SignatureArtifactType = "application/vnd.dev.sigstore.bundle.v0.3+json"

// Signature holds a cryptographic signature and its format metadata.
type Signature struct {
	// Data contains the signature bytes (format is implementation-specific).
	Data []byte

	// MediaType indicates the signature format.
	// Example: "application/vnd.dev.sigstore.bundle.v0.3+json"
	MediaType string
}

// Signer creates cryptographic signatures for pushed artifacts.
type Signer interface {
	// Sign creates a signature for the artifact identified by manifestDigest.
	// The payload is the raw manifest JSON that was pushed.
	Sign(ctx context.Context, manifestDigest digest.Digest, payload []byte) (*Signature, error)
}

// Verifier validates cryptographic signatures on artifacts.
type Verifier interface {
	// Verify checks that sig is a valid signature for the given manifest.
	// Returns nil if valid, error otherwise.
	Verify(ctx context.Context, manifestDigest digest.Digest, payload []byte, sig *Signature) error
}
