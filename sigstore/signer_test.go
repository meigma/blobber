package sigstore

import (
	"context"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSigner_NoKeypair(t *testing.T) {
	t.Parallel()

	_, err := NewSigner()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no keypair configured")
}

func TestNewSigner_WithEphemeralKey(t *testing.T) {
	t.Parallel()

	signer, err := NewSigner(WithEphemeralKey())
	require.NoError(t, err)
	assert.NotNil(t, signer)
	assert.NotNil(t, signer.keypair)
}

func TestSigner_SignRequiresKeypair(t *testing.T) {
	t.Parallel()

	// Create signer with ephemeral key
	signer, err := NewSigner(WithEphemeralKey())
	require.NoError(t, err)

	// Sign should work (though it may fail without Fulcio in test env)
	ctx := context.Background()
	d := digest.FromString("test")
	payload := []byte(`{"test":"data"}`)

	// Without Fulcio/Rekor configured, signing will produce a bundle
	// but it won't have a certificate or transparency log entry.
	// This is still a valid test of the basic signing flow.
	sig, err := signer.Sign(ctx, d, payload)

	// Signing without Fulcio should still produce a valid signature
	// (it just won't be keyless/verifiable via public Sigstore)
	require.NoError(t, err)
	assert.NotEmpty(t, sig.Data)
	assert.Equal(t, "application/vnd.dev.sigstore.bundle.v0.3+json", sig.MediaType)
}

func TestWithFulcio(t *testing.T) {
	t.Parallel()

	signer, err := NewSigner(
		WithEphemeralKey(),
		WithFulcio("https://fulcio.sigstore.dev"),
	)
	require.NoError(t, err)
	assert.NotNil(t, signer.opts.CertificateProvider)
}

func TestWithRekor(t *testing.T) {
	t.Parallel()

	signer, err := NewSigner(
		WithEphemeralKey(),
		WithRekor("https://rekor.sigstore.dev"),
	)
	require.NoError(t, err)
	assert.Len(t, signer.opts.TransparencyLogs, 1)
}

func TestWithMultipleRekorInstances(t *testing.T) {
	t.Parallel()

	signer, err := NewSigner(
		WithEphemeralKey(),
		WithRekor("https://rekor1.example.com"),
		WithRekor("https://rekor2.example.com"),
	)
	require.NoError(t, err)
	assert.Len(t, signer.opts.TransparencyLogs, 2)
}
