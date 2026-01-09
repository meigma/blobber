package sigstore

import (
	"context"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber"
)

func TestVerifier_InvalidBundle(t *testing.T) {
	t.Parallel()

	// Create verifier (will use public Sigstore root)
	// Skip if network is unavailable
	verifier, err := NewVerifier()
	if err != nil {
		t.Skipf("skipping test: cannot create verifier (network required): %v", err)
	}

	ctx := context.Background()
	d := digest.FromString("test")
	payload := []byte(`{"test":"data"}`)

	// Try to verify invalid bundle data
	sig := &blobber.Signature{
		Data:      []byte("not a valid bundle"),
		MediaType: blobber.SignatureArtifactType,
	}

	err = verifier.Verify(ctx, d, payload, sig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse bundle")
}

func TestVerifier_MalformedJSON(t *testing.T) {
	t.Parallel()

	verifier, err := NewVerifier()
	if err != nil {
		t.Skipf("skipping test: cannot create verifier (network required): %v", err)
	}

	ctx := context.Background()
	d := digest.FromString("test")
	payload := []byte(`{"test":"data"}`)

	// Try to verify malformed JSON
	sig := &blobber.Signature{
		Data:      []byte("{invalid json"),
		MediaType: blobber.SignatureArtifactType,
	}

	err = verifier.Verify(ctx, d, payload, sig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse bundle")
}

func TestVerifier_EmptyBundle(t *testing.T) {
	t.Parallel()

	verifier, err := NewVerifier()
	if err != nil {
		t.Skipf("skipping test: cannot create verifier (network required): %v", err)
	}

	ctx := context.Background()
	d := digest.FromString("test")
	payload := []byte(`{"test":"data"}`)

	// Try to verify empty JSON object (valid JSON but not a valid bundle)
	sig := &blobber.Signature{
		Data:      []byte("{}"),
		MediaType: blobber.SignatureArtifactType,
	}

	err = verifier.Verify(ctx, d, payload, sig)
	require.Error(t, err)
	// Empty bundle fails at parse time due to missing bundle version/media type
	assert.Contains(t, err.Error(), "parse bundle")
}

func TestWithIdentity(t *testing.T) {
	t.Parallel()

	verifier, err := NewVerifier(
		WithIdentity("https://accounts.google.com", "user@example.com"),
	)
	if err != nil {
		t.Skipf("skipping test: cannot create verifier (network required): %v", err)
	}

	assert.NotNil(t, verifier.identity)
}

func TestWithIdentity_InvalidIssuer(t *testing.T) {
	t.Parallel()

	// NewShortCertificateIdentity validates the issuer format
	_, err := NewVerifier(
		WithIdentity("", "user@example.com"),
	)
	// Empty issuer should fail
	require.Error(t, err)
}

func TestWithTrustedRootFile_NotFound(t *testing.T) {
	t.Parallel()

	_, err := NewVerifier(
		WithTrustedRootFile("/nonexistent/path/trusted_root.json"),
	)
	require.Error(t, err)
}
