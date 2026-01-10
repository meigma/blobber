package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/core"
)

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("creates registry with defaults", func(t *testing.T) {
		t.Parallel()

		r := New()
		require.NotNil(t, r)
		assert.False(t, r.plainHTTP)
		assert.Equal(t, "blobber/1.0", r.userAgent)
		assert.Nil(t, r.credStore)
	})

	t.Run("applies WithPlainHTTP option", func(t *testing.T) {
		t.Parallel()

		r := New(WithPlainHTTP(true))
		require.NotNil(t, r)
		assert.True(t, r.plainHTTP)
	})

	t.Run("applies WithUserAgent option", func(t *testing.T) {
		t.Parallel()

		r := New(WithUserAgent("custom-agent/2.0"))
		require.NotNil(t, r)
		assert.Equal(t, "custom-agent/2.0", r.userAgent)
	})

	t.Run("applies WithCredentialStore option", func(t *testing.T) {
		t.Parallel()

		store := StaticCredentials("ghcr.io", "user", "pass")
		r := New(WithCredentialStore(store))
		require.NotNil(t, r)
		assert.Equal(t, store, r.credStore)
	})

	t.Run("applies multiple options", func(t *testing.T) {
		t.Parallel()

		store := StaticCredentials("ghcr.io", "user", "pass")
		r := New(
			WithPlainHTTP(true),
			WithUserAgent("multi/1.0"),
			WithCredentialStore(store),
		)
		require.NotNil(t, r)
		assert.True(t, r.plainHTTP)
		assert.Equal(t, "multi/1.0", r.userAgent)
		assert.Equal(t, store, r.credStore)
	})
}

func TestOrasRegistry_InterfaceCompliance(t *testing.T) {
	t.Parallel()

	// This test verifies the compile-time interface check works.
	// The actual check is: var _ contracts.Registry = (*orasRegistry)(nil)
	// If the interface isn't satisfied, the code won't compile.
	r := New()
	assert.NotNil(t, r)
}

func TestIsIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mediaType string
		want      bool
	}{
		{
			name:      "OCI image index",
			mediaType: ocispec.MediaTypeImageIndex,
			want:      true,
		},
		{
			name:      "Docker manifest list",
			mediaType: "application/vnd.docker.distribution.manifest.list.v2+json",
			want:      true,
		},
		{
			name:      "OCI image manifest",
			mediaType: ocispec.MediaTypeImageManifest,
			want:      false,
		},
		{
			name:      "Docker manifest v2",
			mediaType: "application/vnd.docker.distribution.manifest.v2+json",
			want:      false,
		},
		{
			name:      "empty media type",
			mediaType: "",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isIndex(tt.mediaType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestErrRangeNotSupported(t *testing.T) {
	t.Parallel()

	// Verify the error is defined and has a meaningful message.
	assert.NotNil(t, ErrRangeNotSupported)
	assert.Contains(t, ErrRangeNotSupported.Error(), "range")
}

func TestErrMultipleLayers(t *testing.T) {
	t.Parallel()

	// Verify the error is defined and has a meaningful message.
	assert.NotNil(t, ErrMultipleLayers)
	assert.Contains(t, ErrMultipleLayers.Error(), "multiple layers")
}

func TestPullRange_Validation(t *testing.T) {
	t.Parallel()

	r := New()

	tests := []struct {
		name    string
		ref     string
		offset  int64
		length  int64
		wantErr string
	}{
		{
			name:    "negative offset",
			ref:     "ghcr.io/test/repo:tag",
			offset:  -1,
			length:  100,
			wantErr: "offset must be non-negative",
		},
		{
			name:    "zero length",
			ref:     "ghcr.io/test/repo:tag",
			offset:  0,
			length:  0,
			wantErr: "length must be positive",
		},
		{
			name:    "negative length",
			ref:     "ghcr.io/test/repo:tag",
			offset:  0,
			length:  -1,
			wantErr: "length must be positive",
		},
		{
			name:    "overflow",
			ref:     "ghcr.io/test/repo:tag",
			offset:  1<<63 - 1, // max int64
			length:  2,
			wantErr: "range overflow",
		},
		{
			name:    "invalid reference",
			ref:     "invalid ref with spaces",
			offset:  0,
			length:  100,
			wantErr: "invalid reference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := r.PullRange(context.Background(), tt.ref, tt.offset, tt.length)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPullRange_ContextCancellation(t *testing.T) {
	t.Parallel()

	r := New()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := r.PullRange(ctx, "ghcr.io/test/repo:tag", 0, 100)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestPush_Validation(t *testing.T) {
	t.Parallel()

	r := New()

	tests := []struct {
		name    string
		ref     string
		opts    *core.RegistryPushOptions
		wantErr string
	}{
		{
			name: "missing DiffID",
			ref:  "localhost:5000/test/repo:tag",
			opts: &core.RegistryPushOptions{
				BlobDigest: "sha256:abc123",
				BlobSize:   100,
			},
			wantErr: "DiffID",
		},
		{
			name: "missing BlobDigest",
			ref:  "localhost:5000/test/repo:tag",
			opts: &core.RegistryPushOptions{
				DiffID:   "sha256:abc123",
				BlobSize: 100,
			},
			wantErr: "BlobDigest",
		},
		{
			name: "missing BlobSize",
			ref:  "localhost:5000/test/repo:tag",
			opts: &core.RegistryPushOptions{
				DiffID:     "sha256:abc123",
				BlobDigest: "sha256:abc123",
			},
			wantErr: "BlobSize",
		},
		{
			name:    "missing all required fields",
			ref:     "localhost:5000/test/repo:tag",
			opts:    &core.RegistryPushOptions{},
			wantErr: "DiffID, BlobDigest, BlobSize",
		},
		{
			name: "invalid reference",
			ref:  "invalid ref with spaces",
			opts: &core.RegistryPushOptions{
				DiffID:     "sha256:abc123",
				BlobDigest: "sha256:abc123",
				BlobSize:   100,
			},
			wantErr: "invalid reference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := r.Push(context.Background(), tt.ref, strings.NewReader("test"), tt.opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPush_ContextCancellation(t *testing.T) {
	t.Parallel()

	r := New()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	opts := &core.RegistryPushOptions{
		DiffID:     "sha256:abc123",
		BlobDigest: "sha256:abc123",
		BlobSize:   100,
	}

	_, err := r.Push(ctx, "localhost:5000/test/repo:tag", strings.NewReader("test"), opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestPull_Validation(t *testing.T) {
	t.Parallel()

	r := New()

	tests := []struct {
		name    string
		ref     string
		wantErr error
	}{
		{
			name:    "invalid reference",
			ref:     "invalid ref with spaces",
			wantErr: core.ErrInvalidRef,
		},
		{
			name:    "empty reference",
			ref:     "",
			wantErr: core.ErrInvalidRef,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := r.Pull(context.Background(), tt.ref)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestPull_ContextCancellation(t *testing.T) {
	t.Parallel()

	r := New()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, _, err := r.Pull(ctx, "ghcr.io/test/repo:tag")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// mockRegistryServer creates a test server that simulates an OCI registry.
func mockRegistryServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	for pattern, handler := range handlers {
		mux.HandleFunc(pattern, handler)
	}

	return httptest.NewServer(mux)
}

func TestPull_WithMockRegistry(t *testing.T) {
	t.Parallel()

	// Create test layer content
	layerContent := []byte("test layer content")
	layerDigest := digest.FromBytes(layerContent)

	// Create test manifest
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageConfig,
			Digest:    digest.FromString("config"),
			Size:      2,
		},
		Layers: []ocispec.Descriptor{
			{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Digest:    layerDigest,
				Size:      int64(len(layerContent)),
			},
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestDigest := digest.FromBytes(manifestJSON)

	server := mockRegistryServer(t, map[string]http.HandlerFunc{
		"/v2/": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
		"/v2/test/repo/manifests/latest": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Docker-Content-Digest", manifestDigest.String())
			w.Write(manifestJSON)
		},
		"/v2/test/repo/blobs/" + layerDigest.String(): func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Docker-Content-Digest", layerDigest.String())
			w.Write(layerContent)
		},
	})
	defer server.Close()

	// Extract host from server URL (remove http://)
	host := strings.TrimPrefix(server.URL, "http://")

	r := New(WithPlainHTTP(true))
	ref := host + "/test/repo:latest"

	rc, size, err := r.Pull(context.Background(), ref)
	require.NoError(t, err)
	defer rc.Close()

	assert.Equal(t, int64(len(layerContent)), size)

	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, layerContent, content)
}

func TestPull_NotFound(t *testing.T) {
	t.Parallel()

	server := mockRegistryServer(t, map[string]http.HandlerFunc{
		"/v2/": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
		"/v2/test/repo/manifests/latest": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"errors":[{"code":"MANIFEST_UNKNOWN","message":"manifest unknown"}]}`))
		},
	})
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	r := New(WithPlainHTTP(true))
	ref := host + "/test/repo:latest"

	_, _, err := r.Pull(context.Background(), ref)
	require.Error(t, err)
	assert.ErrorIs(t, err, core.ErrNotFound, "expected ErrNotFound, got: %v", err)
}

func TestPull_Unauthorized(t *testing.T) {
	t.Parallel()

	server := mockRegistryServer(t, map[string]http.HandlerFunc{
		"/v2/": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
		"/v2/test/repo/manifests/latest": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}`))
		},
	})
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	r := New(WithPlainHTTP(true))
	ref := host + "/test/repo:latest"

	_, _, err := r.Pull(context.Background(), ref)
	require.Error(t, err)
	assert.ErrorIs(t, err, core.ErrUnauthorized, "expected ErrUnauthorized, got: %v", err)
}

func TestPull_MultipleLayers(t *testing.T) {
	t.Parallel()

	// Create manifest with multiple layers (invalid for blobber)
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageConfig,
			Digest:    digest.FromString("config"),
			Size:      2,
		},
		Layers: []ocispec.Descriptor{
			{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Digest:    digest.FromString("layer1"),
				Size:      100,
			},
			{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Digest:    digest.FromString("layer2"),
				Size:      200,
			},
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestDigest := digest.FromBytes(manifestJSON)

	server := mockRegistryServer(t, map[string]http.HandlerFunc{
		"/v2/": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
		"/v2/test/repo/manifests/latest": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Docker-Content-Digest", manifestDigest.String())
			w.Write(manifestJSON)
		},
	})
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	r := New(WithPlainHTTP(true))
	ref := host + "/test/repo:latest"

	_, _, err = r.Pull(context.Background(), ref)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMultipleLayers, "expected ErrMultipleLayers")
	assert.ErrorIs(t, err, core.ErrInvalidArchive, "expected ErrInvalidArchive")
}

func TestPull_EmptyManifest(t *testing.T) {
	t.Parallel()

	// Create manifest with no layers
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageConfig,
			Digest:    digest.FromString("config"),
			Size:      2,
		},
		Layers: []ocispec.Descriptor{},
	}
	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestDigest := digest.FromBytes(manifestJSON)

	server := mockRegistryServer(t, map[string]http.HandlerFunc{
		"/v2/": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
		"/v2/test/repo/manifests/latest": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Docker-Content-Digest", manifestDigest.String())
			w.Write(manifestJSON)
		},
	})
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	r := New(WithPlainHTTP(true))
	ref := host + "/test/repo:latest"

	_, _, err = r.Pull(context.Background(), ref)
	require.Error(t, err)
	assert.ErrorIs(t, err, core.ErrNotFound, "expected ErrNotFound")
}

func TestPullRange_WithMockRegistry(t *testing.T) {
	t.Parallel()

	// Create test layer content
	layerContent := []byte("0123456789abcdef")
	layerDigest := digest.FromBytes(layerContent)

	// Create test manifest
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageConfig,
			Digest:    digest.FromString("config"),
			Size:      2,
		},
		Layers: []ocispec.Descriptor{
			{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Digest:    layerDigest,
				Size:      int64(len(layerContent)),
			},
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestDigest := digest.FromBytes(manifestJSON)

	server := mockRegistryServer(t, map[string]http.HandlerFunc{
		"/v2/": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
		"/v2/test/repo/manifests/latest": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Docker-Content-Digest", manifestDigest.String())
			w.Write(manifestJSON)
		},
		"/v2/test/repo/blobs/" + layerDigest.String(): func(w http.ResponseWriter, r *http.Request) {
			rangeHeader := r.Header.Get("Range")
			if rangeHeader == "" {
				// Full content request
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Write(layerContent)
				return
			}

			// Parse range header: "bytes=start-end"
			var start, end int64
			_, parseErr := parseRangeHeader(rangeHeader, &start, &end)
			if parseErr != nil {
				http.Error(w, "invalid range", http.StatusBadRequest)
				return
			}

			// Return partial content
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(layerContent)))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(layerContent[start : end+1])
		},
	})
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	r := New(WithPlainHTTP(true))
	ref := host + "/test/repo:latest"

	rc, err := r.PullRange(context.Background(), ref, 5, 5)
	require.NoError(t, err)
	defer rc.Close()

	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, []byte("56789"), content)
}

func TestPullRange_RangeNotSupported(t *testing.T) {
	t.Parallel()

	layerContent := []byte("test content")
	layerDigest := digest.FromBytes(layerContent)

	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageConfig,
			Digest:    digest.FromString("config"),
			Size:      2,
		},
		Layers: []ocispec.Descriptor{
			{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Digest:    layerDigest,
				Size:      int64(len(layerContent)),
			},
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestDigest := digest.FromBytes(manifestJSON)

	server := mockRegistryServer(t, map[string]http.HandlerFunc{
		"/v2/": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
		"/v2/test/repo/manifests/latest": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Docker-Content-Digest", manifestDigest.String())
			w.Write(manifestJSON)
		},
		"/v2/test/repo/blobs/" + layerDigest.String(): func(w http.ResponseWriter, r *http.Request) {
			// Ignore Range header and return full content (simulates registry that doesn't support Range)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			w.Write(layerContent)
		},
	})
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	r := New(WithPlainHTTP(true))
	ref := host + "/test/repo:latest"

	_, err = r.PullRange(context.Background(), ref, 0, 5)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRangeNotSupported, "expected ErrRangeNotSupported")
}

// parseRangeHeader parses a Range header like "bytes=5-9" into start and end values.
func parseRangeHeader(header string, start, end *int64) (bool, error) {
	// Simple parser for "bytes=start-end"
	if !strings.HasPrefix(header, "bytes=") {
		return false, errors.New("invalid range format")
	}
	parts := strings.Split(strings.TrimPrefix(header, "bytes="), "-")
	if len(parts) != 2 {
		return false, errors.New("invalid range format")
	}

	s, err := parseTestInt64(parts[0])
	if err != nil {
		return false, err
	}
	e, err := parseTestInt64(parts[1])
	if err != nil {
		return false, err
	}

	*start = s
	*end = e
	return true, nil
}

func parseTestInt64(s string) (int64, error) {
	var result int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("invalid character in number")
		}
		result = result*10 + int64(c-'0')
	}
	return result, nil
}

func TestPull_OCIIndex(t *testing.T) {
	t.Parallel()

	// Create test layer content
	layerContent := []byte("test layer content")
	layerDigest := digest.FromBytes(layerContent)

	// Create platform-specific manifest
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageConfig,
			Digest:    digest.FromString("config"),
			Size:      2,
		},
		Layers: []ocispec.Descriptor{
			{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Digest:    layerDigest,
				Size:      int64(len(layerContent)),
			},
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestDigest := digest.FromBytes(manifestJSON)

	// Create OCI index pointing to the manifest
	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{
			{
				MediaType: ocispec.MediaTypeImageManifest,
				Digest:    manifestDigest,
				Size:      int64(len(manifestJSON)),
				Platform: &ocispec.Platform{
					Architecture: "amd64",
					OS:           "linux",
				},
			},
		},
	}
	indexJSON, err := json.Marshal(index)
	require.NoError(t, err)
	indexDigest := digest.FromBytes(indexJSON)

	server := mockRegistryServer(t, map[string]http.HandlerFunc{
		"/v2/": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
		"/v2/test/repo/manifests/latest": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageIndex)
			w.Header().Set("Docker-Content-Digest", indexDigest.String())
			w.Write(indexJSON)
		},
		"/v2/test/repo/manifests/" + manifestDigest.String(): func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Docker-Content-Digest", manifestDigest.String())
			w.Write(manifestJSON)
		},
		"/v2/test/repo/blobs/" + layerDigest.String(): func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Docker-Content-Digest", layerDigest.String())
			w.Write(layerContent)
		},
	})
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	r := New(WithPlainHTTP(true))
	ref := host + "/test/repo:latest"

	rc, size, err := r.Pull(context.Background(), ref)
	require.NoError(t, err)
	defer rc.Close()

	assert.Equal(t, int64(len(layerContent)), size)

	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, layerContent, content)
}

func TestPull_DockerManifestList(t *testing.T) {
	t.Parallel()

	// Create test layer content
	layerContent := []byte("test layer content")
	layerDigest := digest.FromBytes(layerContent)

	// Create platform-specific manifest
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageConfig,
			Digest:    digest.FromString("config"),
			Size:      2,
		},
		Layers: []ocispec.Descriptor{
			{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Digest:    layerDigest,
				Size:      int64(len(layerContent)),
			},
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestDigest := digest.FromBytes(manifestJSON)

	// Create Docker manifest list (uses Docker media type)
	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: "application/vnd.docker.distribution.manifest.list.v2+json",
		Manifests: []ocispec.Descriptor{
			{
				MediaType: ocispec.MediaTypeImageManifest,
				Digest:    manifestDigest,
				Size:      int64(len(manifestJSON)),
				Platform: &ocispec.Platform{
					Architecture: "arm64",
					OS:           "linux",
				},
			},
		},
	}
	indexJSON, err := json.Marshal(index)
	require.NoError(t, err)
	indexDigest := digest.FromBytes(indexJSON)

	server := mockRegistryServer(t, map[string]http.HandlerFunc{
		"/v2/": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
		"/v2/test/repo/manifests/latest": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.list.v2+json")
			w.Header().Set("Docker-Content-Digest", indexDigest.String())
			w.Write(indexJSON)
		},
		"/v2/test/repo/manifests/" + manifestDigest.String(): func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Docker-Content-Digest", manifestDigest.String())
			w.Write(manifestJSON)
		},
		"/v2/test/repo/blobs/" + layerDigest.String(): func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Docker-Content-Digest", layerDigest.String())
			w.Write(layerContent)
		},
	})
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	r := New(WithPlainHTTP(true))
	ref := host + "/test/repo:latest"

	rc, size, err := r.Pull(context.Background(), ref)
	require.NoError(t, err)
	defer rc.Close()

	assert.Equal(t, int64(len(layerContent)), size)

	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, layerContent, content)
}

func TestPull_EmptyIndex(t *testing.T) {
	t.Parallel()

	// Create empty OCI index
	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{},
	}
	indexJSON, err := json.Marshal(index)
	require.NoError(t, err)
	indexDigest := digest.FromBytes(indexJSON)

	server := mockRegistryServer(t, map[string]http.HandlerFunc{
		"/v2/": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
		"/v2/test/repo/manifests/latest": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageIndex)
			w.Header().Set("Docker-Content-Digest", indexDigest.String())
			w.Write(indexJSON)
		},
	})
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	r := New(WithPlainHTTP(true))
	ref := host + "/test/repo:latest"

	_, _, err = r.Pull(context.Background(), ref)
	require.Error(t, err)
	assert.ErrorIs(t, err, core.ErrNotFound, "expected ErrNotFound")
}
