package sigstore

import "errors"

// ArtifactType is the OCI artifact type for sigstore bundles.
const ArtifactType = "application/vnd.dev.sigstore.bundle.v0.3+json"

// Signature holds signature data for signing and verification.
type Signature struct {
	// Data contains the sigstore bundle JSON.
	Data []byte

	// MediaType is the artifact type (always ArtifactType for sigstore).
	MediaType string
}

// Errors returned by signing and verification operations.
var (
	// ErrNoSignature indicates no signature was found when verification was required.
	ErrNoSignature = errors.New("sigstore: no signature found")

	// ErrSignatureInvalid indicates signature verification failed.
	ErrSignatureInvalid = errors.New("sigstore: signature verification failed")
)
