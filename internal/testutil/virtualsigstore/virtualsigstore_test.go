package virtualsigstore

import (
	"bytes"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVirtualSigstore_Sign(t *testing.T) {
	t.Parallel()

	vs, err := New()
	require.NoError(t, err)

	artifact := []byte("hello world")
	signed, err := vs.Sign("test@example.com", "https://issuer.example.com", artifact)
	require.NoError(t, err)

	assert.Equal(t, artifact, signed.Artifact)
	assert.NotEmpty(t, signed.BundleJSON)
	assert.Equal(t, "test@example.com", signed.Identity)
	assert.Equal(t, "https://issuer.example.com", signed.Issuer)

	// Verify the bundle JSON is valid
	var b bundle.Bundle
	err = b.UnmarshalJSON(signed.BundleJSON)
	require.NoError(t, err)

	t.Logf("Bundle JSON:\n%s", signed.BundleJSON)
}

func TestVirtualSigstore_TrustedRootJSON(t *testing.T) {
	t.Parallel()

	vs, err := New()
	require.NoError(t, err)

	trJSON, err := vs.TrustedRootJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, trJSON)

	// Verify the trusted root JSON is valid
	tr, err := root.NewTrustedRootFromJSON(trJSON)
	require.NoError(t, err)
	assert.NotNil(t, tr)

	t.Logf("Trusted Root JSON:\n%s", trJSON)
}

func TestVirtualSigstore_SignAndVerify(t *testing.T) {
	t.Parallel()

	vs, err := New()
	require.NoError(t, err)

	// Sign an artifact
	artifact := []byte("test artifact content")
	signed, err := vs.Sign("signer@test.com", "https://accounts.google.com", artifact)
	require.NoError(t, err)

	// Load the bundle
	var b bundle.Bundle
	err = b.UnmarshalJSON(signed.BundleJSON)
	require.NoError(t, err)

	// Create verifier using VirtualSigstore as trusted material
	verifier, err := verify.NewVerifier(
		vs.TrustedMaterial(),
		verify.WithTransparencyLog(1),
		verify.WithSignedTimestamps(1),
	)
	require.NoError(t, err)

	// Build policy
	certID, err := verify.NewShortCertificateIdentity(
		signed.Issuer, "",
		signed.Identity, "",
	)
	require.NoError(t, err)

	policy := verify.NewPolicy(
		verify.WithArtifact(bytes.NewReader(artifact)),
		verify.WithCertificateIdentity(certID),
	)

	// Verify
	result, err := verifier.Verify(&b, policy)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Signature)
	assert.Equal(t, signed.Identity, result.VerifiedIdentity.SubjectAlternativeName.SubjectAlternativeName)
}

func TestVirtualSigstore_VerifyWithTrustedRootJSON(t *testing.T) {
	t.Parallel()

	vs, err := New()
	require.NoError(t, err)

	// Sign an artifact
	artifact := []byte("another test artifact")
	signed, err := vs.Sign("user@company.com", "https://login.company.com", artifact)
	require.NoError(t, err)

	// Get trusted root JSON (simulates saving to file)
	trJSON, err := vs.TrustedRootJSON()
	require.NoError(t, err)

	// Load trusted root from JSON (simulates loading from file)
	trustedRoot, err := root.NewTrustedRootFromJSON(trJSON)
	require.NoError(t, err)

	// Load the bundle
	var b bundle.Bundle
	err = b.UnmarshalJSON(signed.BundleJSON)
	require.NoError(t, err)

	// Create verifier using the loaded trusted root
	verifier, err := verify.NewVerifier(
		trustedRoot,
		verify.WithTransparencyLog(1),
		verify.WithSignedTimestamps(1),
	)
	require.NoError(t, err)

	// Build policy
	certID, err := verify.NewShortCertificateIdentity(
		signed.Issuer, "",
		signed.Identity, "",
	)
	require.NoError(t, err)

	policy := verify.NewPolicy(
		verify.WithArtifact(bytes.NewReader(artifact)),
		verify.WithCertificateIdentity(certID),
	)

	// Verify
	result, err := verifier.Verify(&b, policy)
	require.NoError(t, err)
	assert.NotNil(t, result)
	t.Logf("Verified identity: %s", result.VerifiedIdentity.SubjectAlternativeName.SubjectAlternativeName)
}
