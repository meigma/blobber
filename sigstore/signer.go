package sigstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/opencontainers/go-digest"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/meigma/blobber"
)

// Signer implements blobber.Signer using sigstore-go.
type Signer struct {
	keypair sign.Keypair
	opts    sign.BundleOptions
}

// NewSigner creates a sigstore-based signer.
func NewSigner(opts ...SignerOption) (*Signer, error) {
	s := &Signer{}
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}

	// Require a keypair
	if s.keypair == nil {
		return nil, errors.New("sigstore: no keypair configured (use WithEphemeralKey or WithKeyFile)")
	}

	return s, nil
}

// Sign implements blobber.Signer.
func (s *Signer) Sign(ctx context.Context, manifestDigest digest.Digest, payload []byte) (*blobber.Signature, error) {
	content := &sign.PlainData{Data: payload}

	opts := s.opts
	opts.Context = ctx

	bundle, err := sign.Bundle(content, s.keypair, opts)
	if err != nil {
		return nil, fmt.Errorf("sigstore sign: %w", err)
	}

	bundleJSON, err := protojson.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("sigstore marshal bundle: %w", err)
	}

	return &blobber.Signature{
		Data:      bundleJSON,
		MediaType: blobber.SignatureArtifactType,
	}, nil
}

// Ensure Signer implements blobber.Signer.
var _ blobber.Signer = (*Signer)(nil)
