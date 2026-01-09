package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/core"
)

func TestPushReferrer_InvalidRef(t *testing.T) {
	t.Parallel()

	r := New(WithPlainHTTP(true))

	_, err := r.PushReferrer(context.Background(), "invalid ref", "sha256:abc", []byte("data"), &core.ReferrerPushOptions{
		ArtifactType: "application/test",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, core.ErrInvalidRef)
}

func TestPushReferrer_ContextCancelled(t *testing.T) {
	t.Parallel()

	r := New(WithPlainHTTP(true))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.PushReferrer(ctx, "localhost:5000/test:v1", "sha256:abc", []byte("data"), &core.ReferrerPushOptions{
		ArtifactType: "application/test",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestFetchReferrers_InvalidRef(t *testing.T) {
	t.Parallel()

	r := New(WithPlainHTTP(true))

	_, err := r.FetchReferrers(context.Background(), "invalid ref", "sha256:abc", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, core.ErrInvalidRef)
}

func TestFetchReferrers_ContextCancelled(t *testing.T) {
	t.Parallel()

	r := New(WithPlainHTTP(true))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.FetchReferrers(ctx, "localhost:5000/test:v1", "sha256:abc", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestFetchReferrer_InvalidRef(t *testing.T) {
	t.Parallel()

	r := New(WithPlainHTTP(true))

	_, err := r.FetchReferrer(context.Background(), "invalid ref", "sha256:abc")
	require.Error(t, err)
	assert.ErrorIs(t, err, core.ErrInvalidRef)
}

func TestFetchReferrer_ContextCancelled(t *testing.T) {
	t.Parallel()

	r := New(WithPlainHTTP(true))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.FetchReferrer(ctx, "localhost:5000/test:v1", "sha256:abc")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestFetchReferrers_EmptyResponse(t *testing.T) {
	t.Parallel()

	subjectDigest := digest.FromString("test-subject")

	// Create an empty referrers response (OCI index with no manifests)
	indexResp := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{},
	}
	indexJSON, err := json.Marshal(indexResp)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v2/test/repo/referrers/") {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageIndex)
			w.Write(indexJSON)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	r := New(WithPlainHTTP(true))
	ref := host + "/test/repo:latest"

	referrers, err := r.FetchReferrers(context.Background(), ref, subjectDigest.String(), "")
	require.NoError(t, err)
	assert.Empty(t, referrers)
}

func TestFetchReferrers_WithResults(t *testing.T) {
	t.Parallel()

	subjectDigest := digest.FromString("test-subject")
	refDigest1 := digest.FromString("referrer1")
	refDigest2 := digest.FromString("referrer2")

	// Create a referrers response with some descriptors
	indexResp := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{
			{
				MediaType:    ocispec.MediaTypeImageManifest,
				Digest:       refDigest1,
				Size:         100,
				ArtifactType: "application/vnd.test.sig+json",
				Annotations: map[string]string{
					"key1": "value1",
				},
			},
			{
				MediaType:    ocispec.MediaTypeImageManifest,
				Digest:       refDigest2,
				Size:         200,
				ArtifactType: "application/vnd.test.attestation+json",
			},
		},
	}
	indexJSON, err := json.Marshal(indexResp)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v2/test/repo/referrers/") {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageIndex)
			w.Write(indexJSON)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	r := New(WithPlainHTTP(true))
	ref := host + "/test/repo:latest"

	referrers, err := r.FetchReferrers(context.Background(), ref, subjectDigest.String(), "")
	require.NoError(t, err)
	require.Len(t, referrers, 2)

	assert.Equal(t, refDigest1.String(), referrers[0].Digest)
	assert.Equal(t, "application/vnd.test.sig+json", referrers[0].ArtifactType)
	assert.Equal(t, "value1", referrers[0].Annotations["key1"])

	assert.Equal(t, refDigest2.String(), referrers[1].Digest)
	assert.Equal(t, "application/vnd.test.attestation+json", referrers[1].ArtifactType)
}

func TestFetchReferrer_Success(t *testing.T) {
	t.Parallel()

	layerContent := []byte(`{"signature":"test-sig-data"}`)
	layerDigest := digest.FromBytes(layerContent)

	manifest := ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: "application/vnd.test.sig+json",
		Config: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeEmptyJSON,
			Digest:    digest.FromString("{}"),
			Size:      2,
		},
		Layers: []ocispec.Descriptor{
			{
				MediaType: "application/vnd.test.sig+json",
				Digest:    layerDigest,
				Size:      int64(len(layerContent)),
			},
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestDigest := digest.FromBytes(manifestJSON)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/v2/test/repo/manifests/"+manifestDigest.String() {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Docker-Content-Digest", manifestDigest.String())
			w.Header().Set("Content-Length", strconv.Itoa(len(manifestJSON)))
			if r.Method == http.MethodHead {
				return
			}
			w.Write(manifestJSON)
			return
		}
		if r.URL.Path == "/v2/test/repo/blobs/"+layerDigest.String() {
			w.Header().Set("Content-Type", "application/vnd.test.sig+json")
			w.Header().Set("Docker-Content-Digest", layerDigest.String())
			w.Header().Set("Content-Length", strconv.Itoa(len(layerContent)))
			if r.Method == http.MethodHead {
				return
			}
			w.Write(layerContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	r := New(WithPlainHTTP(true))
	ref := host + "/test/repo:latest"

	data, err := r.FetchReferrer(context.Background(), ref, manifestDigest.String())
	require.NoError(t, err)
	assert.Equal(t, layerContent, data)
}

func TestFetchReferrer_NoLayers(t *testing.T) {
	t.Parallel()

	manifest := ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: "application/vnd.test.sig+json",
		Config: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeEmptyJSON,
			Digest:    digest.FromString("{}"),
			Size:      2,
		},
		Layers: []ocispec.Descriptor{}, // No layers
	}
	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestDigest := digest.FromBytes(manifestJSON)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/v2/test/repo/manifests/"+manifestDigest.String() {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Docker-Content-Digest", manifestDigest.String())
			w.Header().Set("Content-Length", strconv.Itoa(len(manifestJSON)))
			if r.Method == http.MethodHead {
				return
			}
			w.Write(manifestJSON)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	r := New(WithPlainHTTP(true))
	ref := host + "/test/repo:latest"

	_, err = r.FetchReferrer(context.Background(), ref, manifestDigest.String())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no layers")
}
