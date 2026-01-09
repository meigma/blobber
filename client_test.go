package blobber

import (
	"context"
	"io"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/core"
)

func TestDigestReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ref            string
		manifestDigest string
		want           string
	}{
		{
			name:           "tag reference",
			ref:            "ghcr.io/org/repo:v1.0.0",
			manifestDigest: "sha256:abc123",
			want:           "ghcr.io/org/repo@sha256:abc123",
		},
		{
			name:           "digest reference",
			ref:            "ghcr.io/org/repo@sha256:old",
			manifestDigest: "sha256:new",
			want:           "ghcr.io/org/repo@sha256:new",
		},
		{
			name:           "localhost with port and tag",
			ref:            "localhost:5000/repo:latest",
			manifestDigest: "sha256:abc",
			want:           "localhost:5000/repo@sha256:abc",
		},
		{
			name:           "localhost with port no tag",
			ref:            "localhost:5000/repo",
			manifestDigest: "sha256:abc",
			want:           "localhost:5000/repo@sha256:abc",
		},
		{
			name:           "no tag or digest",
			ref:            "docker.io/library/alpine",
			manifestDigest: "sha256:xyz",
			want:           "docker.io/library/alpine@sha256:xyz",
		},
		{
			name:           "registry with port and nested repo",
			ref:            "myregistry.com:8443/org/repo/subdir:tag",
			manifestDigest: "sha256:def",
			want:           "myregistry.com:8443/org/repo/subdir@sha256:def",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := digestReference(tt.ref, tt.manifestDigest)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsSignatureArtifactType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		artifactType string
		want         bool
	}{
		{
			name:         "sigstore bundle",
			artifactType: "application/vnd.dev.sigstore.bundle.v0.3+json",
			want:         true,
		},
		{
			name:         "cosign simple signing",
			artifactType: "application/vnd.dev.cosign.simplesigning.v1+json",
			want:         true,
		},
		{
			name:         "notation signature",
			artifactType: "application/vnd.cncf.notary.signature",
			want:         true,
		},
		{
			name:         "SPDX SBOM",
			artifactType: "application/spdx+json",
			want:         false,
		},
		{
			name:         "CycloneDX SBOM",
			artifactType: "application/vnd.cyclonedx+json",
			want:         false,
		},
		{
			name:         "in-toto attestation",
			artifactType: "application/vnd.in-toto+json",
			want:         false,
		},
		{
			name:         "empty string",
			artifactType: "",
			want:         false,
		},
		{
			name:         "unknown type",
			artifactType: "application/octet-stream",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsSignatureArtifactType(tt.artifactType)
			assert.Equal(t, tt.want, got)
		})
	}
}

// mockVerifyRegistry is a test registry for verification tests.
type mockVerifyRegistry struct {
	// manifestBytes and digest returned by FetchManifest
	indexBytes    []byte
	indexDigest   string
	platformBytes []byte

	// layerDescriptor returned by ResolveLayer
	layerDesc core.LayerDescriptor

	// referrers returned by FetchReferrers, keyed by subject digest
	referrers map[string][]core.Referrer

	// referrer data returned by FetchReferrer, keyed by referrer digest
	referrerData map[string][]byte
}

func (m *mockVerifyRegistry) Push(_ context.Context, _ string, _ io.Reader, _ *core.RegistryPushOptions) (string, error) {
	return "", nil
}

func (m *mockVerifyRegistry) Pull(_ context.Context, _ string) (io.ReadCloser, int64, error) {
	return nil, 0, nil
}

//nolint:nilnil // unused mock method
func (m *mockVerifyRegistry) PullRange(_ context.Context, _ string, _, _ int64) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockVerifyRegistry) ResolveLayer(_ context.Context, _ string) (core.LayerDescriptor, error) {
	return m.layerDesc, nil
}

//nolint:nilnil // unused mock method
func (m *mockVerifyRegistry) FetchBlob(_ context.Context, _ string, _ core.LayerDescriptor) (io.ReadCloser, error) {
	return nil, nil
}

//nolint:nilnil // unused mock method
func (m *mockVerifyRegistry) FetchBlobRange(_ context.Context, _ string, _ core.LayerDescriptor, _, _ int64) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockVerifyRegistry) PushReferrer(_ context.Context, _, _ string, _ []byte, _ *core.ReferrerPushOptions) (string, error) {
	return "", nil
}

func (m *mockVerifyRegistry) FetchReferrers(_ context.Context, _, subjectDigest, _ string) ([]core.Referrer, error) {
	return m.referrers[subjectDigest], nil
}

func (m *mockVerifyRegistry) FetchReferrer(_ context.Context, _, referrerDigest string) ([]byte, error) {
	return m.referrerData[referrerDigest], nil
}

//nolint:gocritic // unnamedResult: not needed for test mock
func (m *mockVerifyRegistry) FetchManifest(_ context.Context, ref string) ([]byte, string, error) {
	// Check if this is a digest reference for the platform manifest
	if m.platformBytes != nil && m.layerDesc.ManifestDigest != m.indexDigest {
		if digestReference("test/repo:tag", m.layerDesc.ManifestDigest) == ref {
			return m.platformBytes, m.layerDesc.ManifestDigest, nil
		}
	}
	return m.indexBytes, m.indexDigest, nil
}

// mockTestVerifier is a test verifier that accepts specific signatures.
type mockTestVerifier struct {
	validSignatures map[string]bool // digest -> signature data -> valid
}

func (m *mockTestVerifier) Verify(_ context.Context, manifestDigest digest.Digest, _ []byte, sig *Signature) error {
	key := manifestDigest.String() + ":" + string(sig.Data)
	if m.validSignatures[key] {
		return nil
	}
	return ErrSignatureInvalid
}

func TestVerifySignature_SingleArch(t *testing.T) {
	t.Parallel()

	// Use valid SHA256 digest format (64 hex chars)
	manifestDigest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	sigData := []byte("valid-signature")

	registry := &mockVerifyRegistry{
		indexBytes:  []byte(`{"schemaVersion":2}`),
		indexDigest: manifestDigest,
		layerDesc: core.LayerDescriptor{
			ManifestDigest: manifestDigest, // Same as index = single-arch
		},
		referrers: map[string][]core.Referrer{
			manifestDigest: {
				{Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ArtifactType: SignatureArtifactType},
			},
		},
		referrerData: map[string][]byte{
			"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": sigData,
		},
	}

	verifier := &mockTestVerifier{
		validSignatures: map[string]bool{
			manifestDigest + ":" + string(sigData): true,
		},
	}

	c := &Client{
		registry: registry,
		verifier: verifier,
	}

	err := c.verifySignature(context.Background(), "test/repo:tag")
	require.NoError(t, err)
}

func TestVerifySignature_MultiArch_PlatformManifest(t *testing.T) {
	t.Parallel()

	// Use valid SHA256 digest format (64 hex chars)
	indexDigest := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	platformDigest := "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	sigData := []byte("platform-signature")

	registry := &mockVerifyRegistry{
		indexBytes:    []byte(`{"schemaVersion":2,"manifests":[]}`),
		indexDigest:   indexDigest,
		platformBytes: []byte(`{"schemaVersion":2,"layers":[]}`),
		layerDesc: core.LayerDescriptor{
			ManifestDigest: platformDigest, // Different from index = multi-arch
		},
		referrers: map[string][]core.Referrer{
			platformDigest: {
				{Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ArtifactType: SignatureArtifactType},
			},
		},
		referrerData: map[string][]byte{
			"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb": sigData,
		},
	}

	verifier := &mockTestVerifier{
		validSignatures: map[string]bool{
			platformDigest + ":" + string(sigData): true,
		},
	}

	c := &Client{
		registry: registry,
		verifier: verifier,
	}

	err := c.verifySignature(context.Background(), "test/repo:tag")
	require.NoError(t, err)
}

func TestVerifySignature_MultiArch_IndexManifest(t *testing.T) {
	t.Parallel()

	// Use valid SHA256 digest format (64 hex chars)
	indexDigest := "sha256:3333333333333333333333333333333333333333333333333333333333333333"
	platformDigest := "sha256:4444444444444444444444444444444444444444444444444444444444444444"
	sigData := []byte("index-signature")

	registry := &mockVerifyRegistry{
		indexBytes:    []byte(`{"schemaVersion":2,"manifests":[]}`),
		indexDigest:   indexDigest,
		platformBytes: []byte(`{"schemaVersion":2,"layers":[]}`),
		layerDesc: core.LayerDescriptor{
			ManifestDigest: platformDigest, // Different from index = multi-arch
		},
		referrers: map[string][]core.Referrer{
			// No signature on platform manifest
			platformDigest: {},
			// Signature on index (cosign default)
			indexDigest: {
				{Digest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", ArtifactType: SignatureArtifactType},
			},
		},
		referrerData: map[string][]byte{
			"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc": sigData,
		},
	}

	verifier := &mockTestVerifier{
		validSignatures: map[string]bool{
			indexDigest + ":" + string(sigData): true,
		},
	}

	c := &Client{
		registry: registry,
		verifier: verifier,
	}

	err := c.verifySignature(context.Background(), "test/repo:tag")
	require.NoError(t, err)
}

func TestVerifySignature_NoSignatures(t *testing.T) {
	t.Parallel()

	// Use valid SHA256 digest format (64 hex chars)
	manifestDigest := "sha256:5555555555555555555555555555555555555555555555555555555555555555"

	registry := &mockVerifyRegistry{
		indexBytes:  []byte(`{"schemaVersion":2}`),
		indexDigest: manifestDigest,
		layerDesc: core.LayerDescriptor{
			ManifestDigest: manifestDigest,
		},
		referrers: map[string][]core.Referrer{
			manifestDigest: {}, // No referrers
		},
	}

	c := &Client{
		registry: registry,
		verifier: &mockTestVerifier{},
	}

	err := c.verifySignature(context.Background(), "test/repo:tag")
	assert.ErrorIs(t, err, ErrNoSignature)
}

func TestVerifySignature_OnlySBOM(t *testing.T) {
	t.Parallel()

	// Use valid SHA256 digest format (64 hex chars)
	manifestDigest := "sha256:6666666666666666666666666666666666666666666666666666666666666666"

	registry := &mockVerifyRegistry{
		indexBytes:  []byte(`{"schemaVersion":2}`),
		indexDigest: manifestDigest,
		layerDesc: core.LayerDescriptor{
			ManifestDigest: manifestDigest,
		},
		referrers: map[string][]core.Referrer{
			manifestDigest: {
				// SBOM, not a signature
				{Digest: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", ArtifactType: "application/spdx+json"},
			},
		},
		referrerData: map[string][]byte{
			"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd": []byte("sbom-data"),
		},
	}

	c := &Client{
		registry: registry,
		verifier: &mockTestVerifier{},
	}

	// Should return ErrNoSignature, not ErrSignatureInvalid
	err := c.verifySignature(context.Background(), "test/repo:tag")
	assert.ErrorIs(t, err, ErrNoSignature)
}
