package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/v2/internal/estargz"
	"github.com/meigma/blobber/v2/internal/oci"
	"github.com/meigma/blobber/v2/internal/oci/mocks"
)

// testRef is a valid reference used across tests.
const testRef = "ghcr.io/test/repo:v1"

// testBlobData is sample blob content for tests.
var testBlobData = []byte("test blob content")

// testMetadata creates a PushMetadata with valid test values.
func testMetadata() PushMetadata {
	return PushMetadata{
		BlobDigest:         digest.FromBytes(testBlobData),
		BlobSize:           int64(len(testBlobData)),
		UncompressedDigest: digest.FromBytes([]byte("uncompressed")),
		TOCDigest:          digest.FromBytes([]byte("toc")),
		MediaType:          "application/vnd.oci.image.layer.v1.tar+gzip",
	}
}

// testManifestJSON creates a valid single-layer manifest JSON.
func testManifestJSON(t *testing.T, blobDigest digest.Digest, blobSize int64) []byte {
	t.Helper()
	manifest := ocispec.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageConfig,
			Digest:    digest.FromBytes([]byte("{}")),
			Size:      2,
		},
		Layers: []ocispec.Descriptor{
			{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Digest:    blobDigest,
				Size:      blobSize,
			},
		},
	}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	return data
}

func TestPush(t *testing.T) {
	t.Parallel()

	t.Run("success with tag", func(t *testing.T) {
		t.Parallel()

		metadata := testMetadata()
		mock := &mocks.ClientMock{
			PushBlobFunc: func(_ context.Context, _ string, _ ocispec.Descriptor, _ io.Reader) error {
				return nil
			},
			PushManifestFunc: func(_ context.Context, _ string, _ ocispec.Descriptor, _ []byte) error {
				return nil
			},
			TagFunc: func(_ context.Context, _ string, _ ocispec.Descriptor, _ string) error {
				return nil
			},
		}

		reg := New(mock)
		result, err := reg.Push(context.Background(), testRef, bytes.NewReader(testBlobData), metadata)

		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify result structure.
		assert.True(t, strings.HasPrefix(result.Reference, "ghcr.io/test/repo@sha256:"))
		assert.Equal(t, metadata.BlobDigest, result.Manifest.Blob.Digest)
		assert.Equal(t, metadata.BlobSize, result.Manifest.Blob.Size)
		assert.Equal(t, metadata.MediaType, result.Manifest.Blob.MediaType)
		assert.Empty(t, result.Manifest.Referrers)
	})

	t.Run("error on digest ref (tag required)", func(t *testing.T) {
		t.Parallel()

		metadata := testMetadata()
		digestRef := "ghcr.io/test/repo@" + metadata.BlobDigest.String()

		reg := New(&mocks.ClientMock{})
		result, err := reg.Push(context.Background(), digestRef, bytes.NewReader(testBlobData), metadata)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "must include a tag")
	})

	t.Run("error on missing TOCDigest", func(t *testing.T) {
		t.Parallel()

		metadata := testMetadata()
		metadata.TOCDigest = "" // Clear TOCDigest

		reg := New(&mocks.ClientMock{})
		result, err := reg.Push(context.Background(), testRef, bytes.NewReader(testBlobData), metadata)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "TOCDigest is required")
	})

	t.Run("error pushing blob", func(t *testing.T) {
		t.Parallel()

		mock := &mocks.ClientMock{
			PushBlobFunc: func(_ context.Context, _ string, _ ocispec.Descriptor, _ io.Reader) error {
				return errors.New("blob push failed")
			},
		}

		reg := New(mock)
		result, err := reg.Push(context.Background(), testRef, bytes.NewReader(testBlobData), testMetadata())

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "push blob")
	})

	t.Run("error pushing config", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		mock := &mocks.ClientMock{
			PushBlobFunc: func(_ context.Context, _ string, _ ocispec.Descriptor, _ io.Reader) error {
				callCount++
				if callCount == 2 {
					return errors.New("config push failed")
				}
				return nil
			},
		}

		reg := New(mock)
		result, err := reg.Push(context.Background(), testRef, bytes.NewReader(testBlobData), testMetadata())

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "push config")
	})

	t.Run("error pushing manifest", func(t *testing.T) {
		t.Parallel()

		mock := &mocks.ClientMock{
			PushBlobFunc: func(_ context.Context, _ string, _ ocispec.Descriptor, _ io.Reader) error {
				return nil
			},
			PushManifestFunc: func(_ context.Context, _ string, _ ocispec.Descriptor, _ []byte) error {
				return errors.New("manifest push failed")
			},
		}

		reg := New(mock)
		result, err := reg.Push(context.Background(), testRef, bytes.NewReader(testBlobData), testMetadata())

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "push manifest")
	})

	t.Run("error tagging", func(t *testing.T) {
		t.Parallel()

		mock := &mocks.ClientMock{
			PushBlobFunc: func(_ context.Context, _ string, _ ocispec.Descriptor, _ io.Reader) error {
				return nil
			},
			PushManifestFunc: func(_ context.Context, _ string, _ ocispec.Descriptor, _ []byte) error {
				return nil
			},
			TagFunc: func(_ context.Context, _ string, _ ocispec.Descriptor, _ string) error {
				return errors.New("tag failed")
			},
		}

		reg := New(mock)
		result, err := reg.Push(context.Background(), testRef, bytes.NewReader(testBlobData), testMetadata())

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "tag manifest")
	})

	t.Run("context canceled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		reg := New(&mocks.ClientMock{})
		result, err := reg.Push(ctx, testRef, bytes.NewReader(testBlobData), testMetadata())

		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("invalid reference", func(t *testing.T) {
		t.Parallel()

		reg := New(&mocks.ClientMock{})
		result, err := reg.Push(context.Background(), "invalid ref!", bytes.NewReader(testBlobData), testMetadata())

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "parse reference")
	})
}

func TestFetchManifest(t *testing.T) {
	t.Parallel()

	blobDigest := digest.FromBytes(testBlobData)
	blobSize := int64(len(testBlobData))

	t.Run("success with referrers", func(t *testing.T) {
		t.Parallel()

		manifestJSON := testManifestJSON(t, blobDigest, blobSize)
		manifestDigest := digest.FromBytes(manifestJSON)

		referrers := []ocispec.Descriptor{
			{
				Digest:       digest.FromBytes([]byte("sig")),
				ArtifactType: "application/vnd.dev.cosign.simplesigning.v1+json",
				Size:         100,
				Annotations:  map[string]string{"key": "value"},
			},
		}

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return manifestJSON, ocispec.Descriptor{
					Digest: manifestDigest,
					Size:   int64(len(manifestJSON)),
				}, nil
			},
			ListReferrersFunc: func(_ context.Context, _ string, _ string, _ string) ([]ocispec.Descriptor, error) {
				return referrers, nil
			},
		}

		reg := New(mock)
		result, err := reg.FetchManifest(context.Background(), testRef)

		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, manifestDigest, result.Digest)
		assert.Equal(t, blobDigest, result.Blob.Digest)
		assert.Equal(t, blobSize, result.Blob.Size)
		require.Len(t, result.Referrers, 1)
		assert.Equal(t, referrers[0].ArtifactType, result.Referrers[0].ArtifactType)
	})

	t.Run("success without referrers option", func(t *testing.T) {
		t.Parallel()

		manifestJSON := testManifestJSON(t, blobDigest, blobSize)
		manifestDigest := digest.FromBytes(manifestJSON)

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return manifestJSON, ocispec.Descriptor{
					Digest: manifestDigest,
					Size:   int64(len(manifestJSON)),
				}, nil
			},
			// ListReferrersFunc should not be called.
		}

		reg := New(mock)
		result, err := reg.FetchManifest(context.Background(), testRef, WithoutReferrers())

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.Referrers)
	})

	t.Run("error fetching manifest", func(t *testing.T) {
		t.Parallel()

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return nil, ocispec.Descriptor{}, oci.ErrNotFound
			},
		}

		reg := New(mock)
		result, err := reg.FetchManifest(context.Background(), testRef)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, oci.ErrNotFound)
	})

	t.Run("invalid manifest json", func(t *testing.T) {
		t.Parallel()

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return []byte("invalid json"), ocispec.Descriptor{}, nil
			},
		}

		reg := New(mock)
		result, err := reg.FetchManifest(context.Background(), testRef)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "parse manifest")
	})

	t.Run("multi-layer manifest rejected", func(t *testing.T) {
		t.Parallel()

		manifest := ocispec.Manifest{
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    ocispec.Descriptor{MediaType: ocispec.MediaTypeImageConfig},
			Layers: []ocispec.Descriptor{
				{Digest: digest.FromBytes([]byte("layer1"))},
				{Digest: digest.FromBytes([]byte("layer2"))},
			},
		}
		manifestJSON, _ := json.Marshal(manifest)

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return manifestJSON, ocispec.Descriptor{}, nil
			},
		}

		reg := New(mock)
		result, err := reg.FetchManifest(context.Background(), testRef)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "expected 1 layer")
	})

	t.Run("zero-layer manifest rejected", func(t *testing.T) {
		t.Parallel()

		manifest := ocispec.Manifest{
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    ocispec.Descriptor{MediaType: ocispec.MediaTypeImageConfig},
			Layers:    []ocispec.Descriptor{},
		}
		manifestJSON, _ := json.Marshal(manifest)

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return manifestJSON, ocispec.Descriptor{}, nil
			},
		}

		reg := New(mock)
		result, err := reg.FetchManifest(context.Background(), testRef)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "expected 1 layer")
	})

	t.Run("error listing referrers", func(t *testing.T) {
		t.Parallel()

		manifestJSON := testManifestJSON(t, blobDigest, blobSize)

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return manifestJSON, ocispec.Descriptor{Digest: digest.FromBytes(manifestJSON)}, nil
			},
			ListReferrersFunc: func(_ context.Context, _ string, _ string, _ string) ([]ocispec.Descriptor, error) {
				return nil, errors.New("referrer list failed")
			},
		}

		reg := New(mock)
		result, err := reg.FetchManifest(context.Background(), testRef)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "list referrers")
	})

	t.Run("context canceled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		reg := New(&mocks.ClientMock{})
		result, err := reg.FetchManifest(ctx, testRef)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestFetchBlob(t *testing.T) {
	t.Parallel()

	blobDigest := digest.FromBytes(testBlobData)
	blobSize := int64(len(testBlobData))

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		manifestJSON := testManifestJSON(t, blobDigest, blobSize)

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return manifestJSON, ocispec.Descriptor{Digest: digest.FromBytes(manifestJSON)}, nil
			},
			FetchBlobFunc: func(_ context.Context, _ string, desc ocispec.Descriptor) (io.ReadCloser, error) {
				assert.Equal(t, blobDigest, desc.Digest)
				return io.NopCloser(bytes.NewReader(testBlobData)), nil
			},
		}

		reg := New(mock)
		rc, err := reg.FetchBlob(context.Background(), testRef)

		require.NoError(t, err)
		require.NotNil(t, rc)
		defer rc.Close()

		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		assert.Equal(t, testBlobData, data)
	})

	t.Run("error fetching manifest", func(t *testing.T) {
		t.Parallel()

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return nil, ocispec.Descriptor{}, oci.ErrNotFound
			},
		}

		reg := New(mock)
		rc, err := reg.FetchBlob(context.Background(), testRef)

		require.Error(t, err)
		assert.Nil(t, rc)
	})

	t.Run("error fetching blob", func(t *testing.T) {
		t.Parallel()

		manifestJSON := testManifestJSON(t, blobDigest, blobSize)

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return manifestJSON, ocispec.Descriptor{Digest: digest.FromBytes(manifestJSON)}, nil
			},
			FetchBlobFunc: func(_ context.Context, _ string, _ ocispec.Descriptor) (io.ReadCloser, error) {
				return nil, errors.New("blob fetch failed")
			},
		}

		reg := New(mock)
		rc, err := reg.FetchBlob(context.Background(), testRef)

		require.Error(t, err)
		assert.Nil(t, rc)
		assert.Contains(t, err.Error(), "fetch blob")
	})

	t.Run("context canceled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		reg := New(&mocks.ClientMock{})
		rc, err := reg.FetchBlob(ctx, testRef)

		require.Error(t, err)
		assert.Nil(t, rc)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestOpenBlob(t *testing.T) {
	t.Parallel()

	blobDigest := digest.FromBytes(testBlobData)
	blobSize := int64(len(testBlobData))

	t.Run("passes correct descriptor to client", func(t *testing.T) {
		t.Parallel()

		manifestJSON := testManifestJSON(t, blobDigest, blobSize)
		var capturedDesc ocispec.Descriptor

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return manifestJSON, ocispec.Descriptor{Digest: digest.FromBytes(manifestJSON)}, nil
			},
			BlobReaderFunc: func(_ context.Context, _ string, desc ocispec.Descriptor) (estargz.BlobSource, error) {
				capturedDesc = desc
				// Return error since we can't construct a real BlobReader in tests.
				return nil, errors.New("mock")
			},
		}

		reg := New(mock)
		_, _ = reg.OpenBlob(context.Background(), testRef)

		// Verify the correct blob descriptor was extracted from manifest.
		assert.Equal(t, blobDigest, capturedDesc.Digest)
		assert.Equal(t, blobSize, capturedDesc.Size)
	})

	t.Run("error fetching manifest", func(t *testing.T) {
		t.Parallel()

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return nil, ocispec.Descriptor{}, oci.ErrNotFound
			},
		}

		reg := New(mock)
		reader, err := reg.OpenBlob(context.Background(), testRef)

		require.Error(t, err)
		assert.Nil(t, reader)
	})

	t.Run("error creating blob reader", func(t *testing.T) {
		t.Parallel()

		manifestJSON := testManifestJSON(t, blobDigest, blobSize)

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return manifestJSON, ocispec.Descriptor{Digest: digest.FromBytes(manifestJSON)}, nil
			},
			BlobReaderFunc: func(_ context.Context, _ string, _ ocispec.Descriptor) (estargz.BlobSource, error) {
				return nil, errors.New("blob reader creation failed")
			},
		}

		reg := New(mock)
		reader, err := reg.OpenBlob(context.Background(), testRef)

		require.Error(t, err)
		assert.Nil(t, reader)
		assert.Contains(t, err.Error(), "create blob reader")
	})

	t.Run("context canceled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		reg := New(&mocks.ClientMock{})
		reader, err := reg.OpenBlob(ctx, testRef)

		require.Error(t, err)
		assert.Nil(t, reader)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestAttachArtifact(t *testing.T) {
	t.Parallel()

	artifactContent := []byte(`{"type": "sbom"}`)
	artifactType := "application/spdx+json"

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		subjectDigest := digest.FromBytes([]byte("manifest"))
		expectedManifestDigest := digest.FromBytes([]byte("referrer-manifest"))

		mock := &mocks.ClientMock{
			ResolveManifestFunc: func(_ context.Context, _ string) (ocispec.Descriptor, error) {
				return ocispec.Descriptor{
					Digest: subjectDigest,
					Size:   100,
				}, nil
			},
			PushReferrerFunc: func(_ context.Context, _ string, subject, artifact ocispec.Descriptor, content []byte) (digest.Digest, error) {
				assert.Equal(t, subjectDigest, subject.Digest)
				assert.Equal(t, artifactType, artifact.MediaType)
				assert.Equal(t, artifactContent, content)
				return expectedManifestDigest, nil
			},
		}

		reg := New(mock)
		resultDigest, err := reg.AttachArtifact(context.Background(), testRef, artifactType, artifactContent, nil)

		require.NoError(t, err)
		assert.Equal(t, expectedManifestDigest, resultDigest)
	})

	t.Run("success with annotations", func(t *testing.T) {
		t.Parallel()

		annotations := map[string]string{"key": "value"}
		expectedManifestDigest := digest.FromBytes([]byte("referrer-manifest"))

		mock := &mocks.ClientMock{
			ResolveManifestFunc: func(_ context.Context, _ string) (ocispec.Descriptor, error) {
				return ocispec.Descriptor{Digest: digest.FromBytes([]byte("manifest"))}, nil
			},
			PushReferrerFunc: func(_ context.Context, _ string, _, artifact ocispec.Descriptor, _ []byte) (digest.Digest, error) {
				assert.Equal(t, annotations, artifact.Annotations)
				return expectedManifestDigest, nil
			},
		}

		reg := New(mock)
		resultDigest, err := reg.AttachArtifact(context.Background(), testRef, artifactType, artifactContent, annotations)

		require.NoError(t, err)
		assert.Equal(t, expectedManifestDigest, resultDigest)
	})

	t.Run("error resolving manifest", func(t *testing.T) {
		t.Parallel()

		mock := &mocks.ClientMock{
			ResolveManifestFunc: func(_ context.Context, _ string) (ocispec.Descriptor, error) {
				return ocispec.Descriptor{}, oci.ErrNotFound
			},
		}

		reg := New(mock)
		resultDigest, err := reg.AttachArtifact(context.Background(), testRef, artifactType, artifactContent, nil)

		require.Error(t, err)
		assert.Empty(t, resultDigest)
		assert.Contains(t, err.Error(), "resolve manifest")
	})

	t.Run("error pushing referrer", func(t *testing.T) {
		t.Parallel()

		mock := &mocks.ClientMock{
			ResolveManifestFunc: func(_ context.Context, _ string) (ocispec.Descriptor, error) {
				return ocispec.Descriptor{Digest: digest.FromBytes([]byte("manifest"))}, nil
			},
			PushReferrerFunc: func(_ context.Context, _ string, _, _ ocispec.Descriptor, _ []byte) (digest.Digest, error) {
				return "", errors.New("push referrer failed")
			},
		}

		reg := New(mock)
		resultDigest, err := reg.AttachArtifact(context.Background(), testRef, artifactType, artifactContent, nil)

		require.Error(t, err)
		assert.Empty(t, resultDigest)
		assert.Contains(t, err.Error(), "push referrer")
	})

	t.Run("context canceled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		reg := New(&mocks.ClientMock{})
		resultDigest, err := reg.AttachArtifact(ctx, testRef, artifactType, artifactContent, nil)

		require.Error(t, err)
		assert.Empty(t, resultDigest)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestFetchReferrerContent(t *testing.T) {
	t.Parallel()

	const artifactContent = `{"signature": "abc123"}`
	artifactDigest := digest.FromString(artifactContent)

	// Build a valid referrer manifest JSON.
	buildReferrerManifest := func(t *testing.T, layerDigest digest.Digest, layerSize int64) []byte {
		t.Helper()
		manifest := map[string]any{
			"schemaVersion": 2,
			"mediaType":     "application/vnd.oci.image.manifest.v1+json",
			"config": map[string]any{
				"mediaType": "application/vnd.oci.empty.v1+json",
				"digest":    "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
				"size":      2,
			},
			"layers": []map[string]any{
				{
					"mediaType": "application/vnd.example.signature",
					"digest":    layerDigest.String(),
					"size":      layerSize,
				},
			},
		}
		data, err := json.Marshal(manifest)
		require.NoError(t, err)
		return data
	}

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		referrerManifest := buildReferrerManifest(t, artifactDigest, int64(len(artifactContent)))
		referrerDigest := digest.FromBytes(referrerManifest)

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, ref string) ([]byte, ocispec.Descriptor, error) {
				// Verify the digest reference is constructed correctly.
				assert.Contains(t, ref, "@"+referrerDigest.String())
				return referrerManifest, ocispec.Descriptor{Digest: referrerDigest}, nil
			},
			FetchBlobFunc: func(_ context.Context, _ string, desc ocispec.Descriptor) (io.ReadCloser, error) {
				// Verify the layer digest is used.
				assert.Equal(t, artifactDigest, desc.Digest)
				return io.NopCloser(strings.NewReader(artifactContent)), nil
			},
		}

		reg := New(mock)
		content, err := reg.FetchReferrerContent(context.Background(), testRef, referrerDigest)

		require.NoError(t, err)
		assert.Equal(t, artifactContent, string(content))
	})

	t.Run("error fetching manifest", func(t *testing.T) {
		t.Parallel()

		referrerDigest := digest.FromString("test-manifest")

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return nil, ocispec.Descriptor{}, oci.ErrNotFound
			},
		}

		reg := New(mock)
		content, err := reg.FetchReferrerContent(context.Background(), testRef, referrerDigest)

		require.Error(t, err)
		assert.Nil(t, content)
		assert.Contains(t, err.Error(), "fetch referrer manifest")
	})

	t.Run("invalid manifest JSON", func(t *testing.T) {
		t.Parallel()

		referrerDigest := digest.FromString("test-manifest")

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return []byte("not valid json"), ocispec.Descriptor{}, nil
			},
		}

		reg := New(mock)
		content, err := reg.FetchReferrerContent(context.Background(), testRef, referrerDigest)

		require.Error(t, err)
		assert.Nil(t, content)
		assert.Contains(t, err.Error(), "parse referrer manifest")
	})

	t.Run("manifest with no layers", func(t *testing.T) {
		t.Parallel()

		noLayersManifest := []byte(`{
			"schemaVersion": 2,
			"mediaType": "application/vnd.oci.image.manifest.v1+json",
			"config": {"mediaType": "application/vnd.oci.empty.v1+json", "digest": "sha256:abc", "size": 2},
			"layers": []
		}`)
		referrerDigest := digest.FromBytes(noLayersManifest)

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return noLayersManifest, ocispec.Descriptor{Digest: referrerDigest}, nil
			},
		}

		reg := New(mock)
		content, err := reg.FetchReferrerContent(context.Background(), testRef, referrerDigest)

		require.Error(t, err)
		assert.Nil(t, content)
		assert.Contains(t, err.Error(), "expected 1 layer, got 0")
	})

	t.Run("manifest with multiple layers", func(t *testing.T) {
		t.Parallel()

		multiLayerManifest := []byte(`{
			"schemaVersion": 2,
			"mediaType": "application/vnd.oci.image.manifest.v1+json",
			"config": {"mediaType": "application/vnd.oci.empty.v1+json", "digest": "sha256:abc", "size": 2},
			"layers": [
				{"mediaType": "application/octet-stream", "digest": "sha256:layer1", "size": 100},
				{"mediaType": "application/octet-stream", "digest": "sha256:layer2", "size": 200}
			]
		}`)
		referrerDigest := digest.FromBytes(multiLayerManifest)

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return multiLayerManifest, ocispec.Descriptor{Digest: referrerDigest}, nil
			},
		}

		reg := New(mock)
		content, err := reg.FetchReferrerContent(context.Background(), testRef, referrerDigest)

		require.Error(t, err)
		assert.Nil(t, content)
		assert.Contains(t, err.Error(), "expected 1 layer, got 2")
	})

	t.Run("error fetching blob", func(t *testing.T) {
		t.Parallel()

		referrerManifest := buildReferrerManifest(t, artifactDigest, int64(len(artifactContent)))
		referrerDigest := digest.FromBytes(referrerManifest)

		mock := &mocks.ClientMock{
			FetchManifestFunc: func(_ context.Context, _ string) ([]byte, ocispec.Descriptor, error) {
				return referrerManifest, ocispec.Descriptor{Digest: referrerDigest}, nil
			},
			FetchBlobFunc: func(_ context.Context, _ string, _ ocispec.Descriptor) (io.ReadCloser, error) {
				return nil, errors.New("blob fetch failed")
			},
		}

		reg := New(mock)
		content, err := reg.FetchReferrerContent(context.Background(), testRef, referrerDigest)

		require.Error(t, err)
		assert.Nil(t, content)
		assert.Contains(t, err.Error(), "fetch referrer content")
	})

	t.Run("invalid reference", func(t *testing.T) {
		t.Parallel()

		reg := New(&mocks.ClientMock{})
		content, err := reg.FetchReferrerContent(context.Background(), "not-a-valid-ref", digest.FromString("test"))

		require.Error(t, err)
		assert.Nil(t, content)
		assert.Contains(t, err.Error(), "parse reference")
	})

	t.Run("context canceled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		reg := New(&mocks.ClientMock{})
		content, err := reg.FetchReferrerContent(ctx, testRef, digest.FromString("test"))

		require.Error(t, err)
		assert.Nil(t, content)
		assert.ErrorIs(t, err, context.Canceled)
	})
}
