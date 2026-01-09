// Package sigstore provides signing and verification implementations using sigstore-go.
//
// This package implements blobber.Signer and blobber.Verifier using the sigstore-go
// library for cryptographic signing operations.
//
// # Separate Module
//
// This package is a separate Go module (github.com/meigma/blobber/sigstore) to isolate
// the sigstore-go dependency. This design allows users who don't need signing/verification
// to import github.com/meigma/blobber without pulling in sigstore-go and its transitive
// dependencies (protobuf, gRPC, OIDC, etc.).
//
// # Signing
//
// The Signer creates Sigstore bundles that can be stored as OCI referrer artifacts.
// Multiple signing modes are supported:
//
//   - Ephemeral keys with Fulcio CA (keyless signing via OIDC)
//   - Local key files (private keys in PEM format)
//
// Example:
//
//	signer, err := sigstore.NewSigner(
//	    sigstore.WithEphemeralKey(),
//	    sigstore.WithFulcio("https://fulcio.sigstore.dev"),
//	    sigstore.WithRekor("https://rekor.sigstore.dev"),
//	)
//
// # Verification
//
// The Verifier validates Sigstore bundles against a trusted root.
// By default, it uses the public Sigstore trust root.
//
// Example:
//
//	verifier, err := sigstore.NewVerifier(
//	    sigstore.WithIdentity("https://accounts.google.com", "user@example.com"),
//	)
package sigstore
