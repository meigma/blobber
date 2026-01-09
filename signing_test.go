//go:build integration

package blobber_test

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber"
)

// mockSigner is a test signer that creates deterministic signatures.
type mockSigner struct {
	signatures map[string][]byte // digest -> signature
}

func newMockSigner() *mockSigner {
	return &mockSigner{
		signatures: make(map[string][]byte),
	}
}

func (m *mockSigner) Sign(_ context.Context, manifestDigest digest.Digest, _ []byte) (*blobber.Signature, error) {
	// Create a deterministic "signature" based on the digest
	sig := []byte(`{"mock":"signature","digest":"` + manifestDigest.String() + `"}`)
	m.signatures[manifestDigest.String()] = sig
	return &blobber.Signature{
		Data:      sig,
		MediaType: blobber.SignatureArtifactType,
	}, nil
}

// mockVerifier is a test verifier that validates against mockSigner signatures.
type mockVerifier struct {
	expectedSignatures map[string][]byte // digest -> expected signature
}

func newMockVerifier(signer *mockSigner) *mockVerifier {
	return &mockVerifier{
		expectedSignatures: signer.signatures,
	}
}

func (m *mockVerifier) Verify(_ context.Context, manifestDigest digest.Digest, _ []byte, sig *blobber.Signature) error {
	expected, ok := m.expectedSignatures[manifestDigest.String()]
	if !ok {
		return blobber.ErrSignatureInvalid
	}
	if string(sig.Data) != string(expected) {
		return blobber.ErrSignatureInvalid
	}
	return nil
}

// strictMockVerifier always requires a signature and fails if none found.
type strictMockVerifier struct{}

func (m *strictMockVerifier) Verify(_ context.Context, _ digest.Digest, _ []byte, sig *blobber.Signature) error {
	if sig == nil || len(sig.Data) == 0 {
		return blobber.ErrNoSignature
	}
	// Accept any non-empty signature for testing
	return nil
}

func TestIntegration_PushWithSigning(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	signer := newMockSigner()

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithSigner(signer),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/signed:v1"
	srcFS := fstest.MapFS{
		"data.txt": &fstest.MapFile{
			Data:    []byte("signed content"),
			Mode:    0644,
			ModTime: time.Now(),
		},
	}

	// Push with automatic signing
	manifestDigest, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)
	assert.NotEmpty(t, manifestDigest)

	// Verify signature was created
	assert.Contains(t, signer.signatures, manifestDigest, "signature should be created for manifest")
}

func TestIntegration_OpenImageWithVerification(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	signer := newMockSigner()

	// Push with signing
	pushClient, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithSigner(signer),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/verified:v1"
	srcFS := fstest.MapFS{
		"data.txt": &fstest.MapFile{
			Data:    []byte("verified content"),
			Mode:    0644,
			ModTime: time.Now(),
		},
	}

	_, err = pushClient.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Open with verification using matching verifier
	verifier := newMockVerifier(signer)
	verifyClient, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithVerifier(verifier),
	)
	require.NoError(t, err)

	img, err := verifyClient.OpenImage(ctx, ref)
	require.NoError(t, err)
	defer img.Close()

	// Should be able to list files
	entries, err := img.List()
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestIntegration_OpenImageWithVerification_NoSignature(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	// Push WITHOUT signing
	pushClient, err := blobber.NewClient(
		blobber.WithInsecure(true),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/unsigned:v1"
	srcFS := fstest.MapFS{
		"data.txt": &fstest.MapFile{
			Data:    []byte("unsigned content"),
			Mode:    0644,
			ModTime: time.Now(),
		},
	}

	_, err = pushClient.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Try to open with verification - should fail (no signature)
	verifyClient, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithVerifier(&strictMockVerifier{}),
	)
	require.NoError(t, err)

	_, err = verifyClient.OpenImage(ctx, ref)
	require.Error(t, err)
	assert.ErrorIs(t, err, blobber.ErrNoSignature)
}

func TestIntegration_PushPullWithSigningRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	signer := newMockSigner()

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithSigner(signer),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/signedpull:v1"
	srcFS := fstest.MapFS{
		"hello.txt": &fstest.MapFile{
			Data:    []byte("Hello, signed world!"),
			Mode:    0644,
			ModTime: time.Now(),
		},
	}

	// Push with signing
	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Pull should still work (Pull doesn't verify, only OpenImage does)
	destDir := t.TempDir()
	err = client.Pull(ctx, ref, destDir)
	require.NoError(t, err)

	// Verify file content
	assertFilesMatch(t, srcFS, destDir)
}
