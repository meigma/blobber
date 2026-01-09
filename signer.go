// Package blobber provides signing and verification interfaces for OCI artifacts.
package blobber

import (
	"context"

	"github.com/opencontainers/go-digest"
)

// SignatureArtifactType is the OCI artifact type for sigstore bundles.
const SignatureArtifactType = "application/vnd.dev.sigstore.bundle.v0.3+json"

// knownNonSignatureTypes lists artifact types that are known to NOT be signatures.
// These are explicitly excluded during verification to avoid treating SBOMs,
// attestations, and other artifacts as failed signature attempts.
// Unknown types are passed through to the verifier (supporting custom signers).
//
// References for common artifact types:
// - OCI Artifacts: https://github.com/opencontainers/image-spec/blob/main/artifacts-guidance.md
// - SPDX: https://spdx.dev/specifications/
// - CycloneDX: https://cyclonedx.org/specification/overview/
// - in-toto: https://in-toto.io/
// - Sigstore: https://github.com/sigstore/protobuf-specs
var knownNonSignatureTypes = map[string]bool{
	// SBOM formats
	"application/spdx+json":                 true,
	"application/vnd.cyclonedx+json":        true,
	"application/vnd.syft+json":             true,
	"application/vnd.cyclonedx+xml":         true,
	"application/spdx":                      true,
	"application/vnd.spdx+json":             true,
	"application/vnd.spdx.spdx+json":        true,
	"application/vnd.cyclonedx":             true,
	"text/spdx":                             true,
	"text/spdx+xml":                         true,
	"text/spdx+json":                        true,
	"text/vnd.cyclonedx+xml":                true,
	"text/vnd.cyclonedx+json":               true,
	"text/vnd.in-toto+json":                 true,
	"text/vnd.syft+json":                    true,
	"application/vnd.in-toto+json":          true,
	"application/vnd.dsse.envelope.v1+json": true,

	// Attestation formats (in-toto, SLSA)
	"application/vnd.in-toto.bundle+json":                  true,
	"application/vnd.dev.cosign.attestation.v1+json":       true,
	"application/vnd.dev.cosign.sbom.v1+json":              true,
	"application/vnd.oci.image.manifest.v1+json":           true,
	"application/vnd.docker.distribution.manifest.v2+json": true,
}

// IsNonSignatureArtifactType reports whether the artifact type is known to NOT be a signature.
// Returns true for known non-signature formats (SBOMs, attestations).
// Returns false for signature formats and unknown types (allowing custom signers).
func IsNonSignatureArtifactType(artifactType string) bool {
	return knownNonSignatureTypes[artifactType]
}

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
