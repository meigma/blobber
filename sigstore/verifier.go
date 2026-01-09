package sigstore

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"

	"github.com/opencontainers/go-digest"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"

	"github.com/meigma/blobber"
)

// Verifier implements blobber.Verifier using sigstore-go.
type Verifier struct {
	trustedRoot root.TrustedMaterial
	identity    *verify.CertificateIdentity
	logger      *slog.Logger
}

// NewVerifier creates a sigstore-based verifier.
func NewVerifier(opts ...VerifierOption) (*Verifier, error) {
	v := &Verifier{
		logger: slog.New(slog.DiscardHandler),
	}
	for _, opt := range opts {
		if err := opt(v); err != nil {
			return nil, err
		}
	}

	// Default to public Sigstore instance if no trusted root provided
	if v.trustedRoot == nil {
		tr, err := root.FetchTrustedRoot()
		if err != nil {
			return nil, fmt.Errorf("sigstore fetch trusted root: %w", err)
		}
		v.trustedRoot = tr
	}

	// Warn if no identity is configured
	if v.identity == nil {
		v.logger.Warn("sigstore verifier created without identity requirement; " +
			"any valid signature will be accepted regardless of signer")
	}

	return v, nil
}

// Verify implements blobber.Verifier.
func (v *Verifier) Verify(ctx context.Context, manifestDigest digest.Digest, payload []byte, sig *blobber.Signature) error {
	var b bundle.Bundle
	if err := b.UnmarshalJSON(sig.Data); err != nil {
		return fmt.Errorf("sigstore parse bundle: %w", err)
	}

	// Build verifier with transparency log and timestamp requirements
	verifier, err := verify.NewVerifier(
		v.trustedRoot,
		verify.WithObserverTimestamps(1),
		verify.WithTransparencyLog(1),
	)
	if err != nil {
		return fmt.Errorf("sigstore create verifier: %w", err)
	}

	// Build verification policy
	var policyOpts []verify.PolicyOption
	if v.identity != nil {
		policyOpts = append(policyOpts, verify.WithCertificateIdentity(*v.identity))
	} else {
		policyOpts = append(policyOpts, verify.WithoutIdentitiesUnsafe())
	}

	policy := verify.NewPolicy(
		verify.WithArtifact(bytes.NewReader(payload)),
		policyOpts...,
	)

	_, err = verifier.Verify(&b, policy)
	if err != nil {
		return fmt.Errorf("%w: %w", blobber.ErrSignatureInvalid, err)
	}

	return nil
}

// Ensure Verifier implements blobber.Verifier.
var _ blobber.Verifier = (*Verifier)(nil)
