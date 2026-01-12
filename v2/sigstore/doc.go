// Package sigstore provides signing and verification using sigstore-go.
//
// This package provides Sigstore-based signing and verification that integrates
// with the blobber v2 client through the Policy interface and Sign helper function.
//
// # Separate Module
//
// This package is a separate Go module (github.com/meigma/blobber/v2/sigstore) to isolate
// the sigstore-go dependency. This design allows users who don't need signing/verification
// to import github.com/meigma/blobber/v2 without pulling in sigstore-go and its transitive
// dependencies (protobuf, gRPC, OIDC, etc.).
//
// # Signing
//
// The Sign function creates a Sigstore bundle and attaches it as an OCI referrer:
//
//	signer, err := sigstore.NewSigner(
//	    sigstore.WithEphemeralKey(),
//	    sigstore.WithFulcio("https://fulcio.sigstore.dev"),
//	    sigstore.WithRekor("https://rekor.sigstore.dev"),
//	)
//	result, _ := client.Push(ctx, ref, myFS)
//	_, err = sigstore.Sign(ctx, client, result.Reference, signer)
//
// # Verification
//
// Use RequireSignature to create a Policy for Pull or Stream:
//
//	verifier, err := sigstore.NewVerifier(
//	    sigstore.WithIdentity("https://accounts.google.com", "user@example.com"),
//	)
//	policy := sigstore.RequireSignature(verifier)
//	handle, err := client.Pull(ctx, ref, blobber.WithPullPolicy(policy))
package sigstore
