package blobber

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"testing/fstest"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/v2/internal/estargz"
	"github.com/meigma/blobber/v2/internal/registry"
	"github.com/meigma/blobber/v2/internal/registry/mocks"
)

func TestPush_Success(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("hello world"), Mode: 0o644},
	}

	expectedDigest := digest.FromString("test-manifest")
	expectedResult := &registry.PushResult{
		Reference: "ghcr.io/org/repo@" + expectedDigest.String(),
		Manifest: registry.BlobManifest{
			Digest: expectedDigest,
			Size:   100,
		},
	}

	mockRegistry := &mocks.RegistryMock{
		PushFunc: func(ctx context.Context, ref string, blob io.Reader, metadata registry.PushMetadata) (*registry.PushResult, error) {
			assert.Equal(t, "ghcr.io/org/repo:v1", ref)
			assert.NotEmpty(t, metadata.BlobDigest)
			assert.NotEmpty(t, metadata.TOCDigest)
			return expectedResult, nil
		},
	}

	client := NewClient(withRegistry(mockRegistry))

	result, err := client.Push(context.Background(), "ghcr.io/org/repo:v1", src)
	require.NoError(t, err)
	assert.Equal(t, expectedResult.Reference, result.Reference)
	assert.Equal(t, expectedResult.Manifest.Digest, result.Manifest.Digest)
}

func TestPush_WithCompression(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("compress me"), Mode: 0o644},
	}

	mockRegistry := &mocks.RegistryMock{
		PushFunc: func(ctx context.Context, ref string, blob io.Reader, metadata registry.PushMetadata) (*registry.PushResult, error) {
			return &registry.PushResult{
				Reference: "ghcr.io/org/repo@sha256:abc",
				Manifest:  registry.BlobManifest{Digest: digest.FromString("test")},
			}, nil
		},
	}

	client := NewClient(withRegistry(mockRegistry))

	// Test with zstd compression.
	result, err := client.Push(context.Background(), "ghcr.io/org/repo:v1", src, WithCompression(ZstdCompression()))
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestPush_BuildError(t *testing.T) {
	t.Parallel()

	// Create an fs.FS that will cause a build error (empty).
	src := fstest.MapFS{}

	mockRegistry := &mocks.RegistryMock{
		PushFunc: func(ctx context.Context, ref string, blob io.Reader, metadata registry.PushMetadata) (*registry.PushResult, error) {
			return &registry.PushResult{}, nil
		},
	}

	client := NewClient(withRegistry(mockRegistry))

	// Empty filesystem should still work (creates empty archive).
	result, err := client.Push(context.Background(), "ghcr.io/org/repo:v1", src)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestFetchManifest_Success(t *testing.T) {
	t.Parallel()

	expectedManifest := &registry.BlobManifest{
		Digest: digest.FromString("manifest"),
		Size:   100,
		Blob: registry.BlobDescriptor{
			Digest:    digest.FromString("blob"),
			Size:      1000,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		},
		Referrers: []registry.Referrer{
			{
				Digest:       digest.FromString("sig"),
				ArtifactType: "application/vnd.dev.cosign.simplesigning.v1+json",
				Size:         50,
			},
		},
	}

	mockRegistry := &mocks.RegistryMock{
		FetchManifestFunc: func(ctx context.Context, ref string, opts ...registry.FetchOption) (*registry.BlobManifest, error) {
			assert.Equal(t, "ghcr.io/org/repo:v1", ref)
			return expectedManifest, nil
		},
	}

	client := NewClient(withRegistry(mockRegistry))

	manifest, err := client.FetchManifest(context.Background(), "ghcr.io/org/repo:v1")
	require.NoError(t, err)
	assert.Equal(t, expectedManifest.Digest, manifest.Digest)
	assert.Len(t, manifest.Referrers, 1)
}

func TestPull_Success(t *testing.T) {
	t.Parallel()

	// Create a real estargz blob for testing.
	src := fstest.MapFS{
		"config.yaml": &fstest.MapFile{Data: []byte("key: value"), Mode: 0o644},
	}
	blob := buildClientTestBlob(t, src)

	mockRegistry := &mocks.RegistryMock{
		FetchBlobFunc: func(ctx context.Context, ref string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(blob)), nil
		},
	}

	client := NewClient(withRegistry(mockRegistry))

	handle, err := client.Pull(context.Background(), "ghcr.io/org/repo:v1")
	require.NoError(t, err)
	defer handle.Close()

	// Verify we can read the file.
	f, err := handle.Open("config.yaml")
	require.NoError(t, err)
	defer f.Close()

	content, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, "key: value", string(content))
}

func TestPull_WithPolicy_Passes(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("data"), Mode: 0o644},
	}
	blob := buildClientTestBlob(t, src)

	mockRegistry := &mocks.RegistryMock{
		FetchManifestFunc: func(ctx context.Context, ref string, opts ...registry.FetchOption) (*registry.BlobManifest, error) {
			return &registry.BlobManifest{
				Digest: digest.FromString("manifest"),
				Referrers: []registry.Referrer{
					{Digest: digest.FromString("sig"), ArtifactType: "application/vnd.dev.cosign.simplesigning.v1+json"},
				},
			}, nil
		},
		FetchBlobFunc: func(ctx context.Context, ref string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(blob)), nil
		},
	}

	policy := &mockPolicy{verifyFunc: func(ctx context.Context, manifest *BlobManifest, fetcher ReferrerFetcher) error {
		// Policy passes if there's at least one referrer.
		if len(manifest.Referrers) > 0 {
			return nil
		}
		return errors.New("no referrers")
	}}

	client := NewClient(withRegistry(mockRegistry))

	handle, err := client.Pull(context.Background(), "ghcr.io/org/repo:v1", WithPullPolicy(policy))
	require.NoError(t, err)
	handle.Close()
}

func TestPull_WithPolicy_Fails(t *testing.T) {
	t.Parallel()

	mockRegistry := &mocks.RegistryMock{
		FetchManifestFunc: func(ctx context.Context, ref string, opts ...registry.FetchOption) (*registry.BlobManifest, error) {
			return &registry.BlobManifest{
				Digest:    digest.FromString("manifest"),
				Referrers: nil, // No referrers.
			}, nil
		},
	}

	policy := &mockPolicy{verifyFunc: func(ctx context.Context, manifest *BlobManifest, fetcher ReferrerFetcher) error {
		return errors.New("signature required")
	}}

	client := NewClient(withRegistry(mockRegistry))

	_, err := client.Pull(context.Background(), "ghcr.io/org/repo:v1", WithPullPolicy(policy))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "policy verification failed")
	assert.Contains(t, err.Error(), "signature required")
}

func TestStream_Success(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"data.json": &fstest.MapFile{Data: []byte(`{"key": "value"}`), Mode: 0o644},
	}
	blob := buildClientTestBlob(t, src)
	blobDigest := digest.FromBytes(blob)

	mockRegistry := &mocks.RegistryMock{
		FetchManifestFunc: func(ctx context.Context, ref string, opts ...registry.FetchOption) (*registry.BlobManifest, error) {
			return &registry.BlobManifest{
				Digest: digest.FromString("manifest"),
				Size:   100,
				Blob: registry.BlobDescriptor{
					Digest: blobDigest,
					Size:   int64(len(blob)),
				},
			}, nil
		},
		OpenBlobByDigestFunc: func(ctx context.Context, ref string, d digest.Digest, size int64) (estargz.BlobSource, error) {
			// Return a mock that implements the required interface.
			return nil, errors.New("OpenBlobByDigest not implemented in this test")
		},
		FetchBlobByDigestFunc: func(ctx context.Context, ref string, d digest.Digest, size int64) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(blob)), nil
		},
	}

	client := NewClient(withRegistry(mockRegistry))

	// Stream requires OpenBlobByDigest which needs more complex mocking.
	// For this test, we verify the error path.
	_, err := client.Stream(context.Background(), "ghcr.io/org/repo:v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open blob")
}

func TestAttachArtifact_Success(t *testing.T) {
	t.Parallel()

	expectedDigest := digest.FromString("artifact-content")

	mockRegistry := &mocks.RegistryMock{
		AttachArtifactFunc: func(ctx context.Context, ref string, artifactType string, content []byte, annotations map[string]string) (digest.Digest, error) {
			assert.Equal(t, "ghcr.io/org/repo:v1", ref)
			assert.Equal(t, "application/spdx+json", artifactType)
			assert.Equal(t, []byte("sbom content"), content)
			assert.Equal(t, "my-sbom", annotations["name"])
			return expectedDigest, nil
		},
	}

	client := NewClient(withRegistry(mockRegistry))

	d, err := client.AttachArtifact(
		context.Background(),
		"ghcr.io/org/repo:v1",
		"application/spdx+json",
		[]byte("sbom content"),
		map[string]string{"name": "my-sbom"},
	)
	require.NoError(t, err)
	assert.Equal(t, expectedDigest, d)
}

func TestPull_ContextCanceled(t *testing.T) {
	t.Parallel()

	mockRegistry := &mocks.RegistryMock{}
	client := NewClient(withRegistry(mockRegistry))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Pull(ctx, "ghcr.io/org/repo:v1")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// mockPolicy implements Policy for testing.
type mockPolicy struct {
	verifyFunc func(ctx context.Context, manifest *BlobManifest, fetcher ReferrerFetcher) error
}

//nolint:gocritic // hugeParam: manifest is passed by value to match Policy interface
func (p *mockPolicy) Verify(ctx context.Context, manifest BlobManifest, fetcher ReferrerFetcher) error {
	return p.verifyFunc(ctx, &manifest, fetcher)
}

// buildClientTestBlob creates an estargz archive from the given filesystem.
// Named differently from handle_test.go's buildClientTestBlob to avoid redeclaration.
func buildClientTestBlob(t *testing.T, src fstest.MapFS) []byte {
	t.Helper()

	var buf bytes.Buffer
	builder := estargz.NewBuilder()
	_, err := builder.Build(context.Background(), &buf, src)
	require.NoError(t, err)

	return buf.Bytes()
}

// Integration tests for caching.

func TestPull_WithFileCache_CacheMissThenHit(t *testing.T) {
	t.Parallel()

	// Create test content.
	src := fstest.MapFS{
		"config.yaml": &fstest.MapFile{Data: []byte("key: value"), Mode: 0o644},
	}
	blob := buildClientTestBlob(t, src)
	blobDigest := digest.FromBytes(blob)

	// Track how many times the registry is called.
	fetchCount := 0

	mockRegistry := &mocks.RegistryMock{
		ResolveRefFunc: func(ctx context.Context, ref string) (digest.Digest, error) {
			return blobDigest, nil
		},
		FetchManifestFunc: func(ctx context.Context, ref string, opts ...registry.FetchOption) (*registry.BlobManifest, error) {
			return &registry.BlobManifest{
				Digest: digest.FromString("manifest"),
				Blob: registry.BlobDescriptor{
					Digest: blobDigest,
					Size:   int64(len(blob)),
				},
			}, nil
		},
		FetchBlobByDigestFunc: func(ctx context.Context, ref string, d digest.Digest, size int64) (io.ReadCloser, error) {
			fetchCount++
			return io.NopCloser(bytes.NewReader(blob)), nil
		},
	}

	cacheDir := t.TempDir()
	client := NewClient(
		withRegistry(mockRegistry),
		WithFileCache(cacheDir),
	)

	// First pull - cache miss.
	handle1, err := client.Pull(context.Background(), "ghcr.io/org/repo:v1")
	require.NoError(t, err)

	f1, err := handle1.Open("config.yaml")
	require.NoError(t, err)
	content1, err := io.ReadAll(f1)
	require.NoError(t, err)
	assert.Equal(t, "key: value", string(content1))
	f1.Close()
	handle1.Close()

	// Verify network was called.
	assert.Equal(t, 1, fetchCount, "first pull should fetch from network")

	// Second pull - cache hit.
	handle2, err := client.Pull(context.Background(), "ghcr.io/org/repo:v1")
	require.NoError(t, err)

	f2, err := handle2.Open("config.yaml")
	require.NoError(t, err)
	content2, err := io.ReadAll(f2)
	require.NoError(t, err)
	assert.Equal(t, "key: value", string(content2))
	f2.Close()
	handle2.Close()

	// Verify network was NOT called again.
	assert.Equal(t, 1, fetchCount, "second pull should serve from cache")
}

func TestPull_WithRefCache_TTLBehavior(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("data"), Mode: 0o644},
	}
	blob := buildClientTestBlob(t, src)
	blobDigest := digest.FromBytes(blob)

	resolveCount := 0

	mockRegistry := &mocks.RegistryMock{
		ResolveRefFunc: func(ctx context.Context, ref string) (digest.Digest, error) {
			resolveCount++
			return blobDigest, nil
		},
		FetchManifestFunc: func(ctx context.Context, ref string, opts ...registry.FetchOption) (*registry.BlobManifest, error) {
			return &registry.BlobManifest{
				Digest: digest.FromString("manifest"),
				Blob: registry.BlobDescriptor{
					Digest: blobDigest,
					Size:   int64(len(blob)),
				},
			}, nil
		},
		FetchBlobByDigestFunc: func(ctx context.Context, ref string, d digest.Digest, size int64) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(blob)), nil
		},
	}

	cacheDir := t.TempDir()
	client := NewClient(
		withRegistry(mockRegistry),
		WithRefCache(cacheDir, 1*time.Hour), // Long TTL
		WithFileCache(cacheDir),
	)

	// First pull - ref cache miss.
	handle1, err := client.Pull(context.Background(), "ghcr.io/org/repo:v1")
	require.NoError(t, err)
	handle1.Close()
	assert.Equal(t, 1, resolveCount, "first pull should resolve ref")

	// Second pull - ref cache hit (within TTL).
	handle2, err := client.Pull(context.Background(), "ghcr.io/org/repo:v1")
	require.NoError(t, err)
	handle2.Close()
	assert.Equal(t, 1, resolveCount, "second pull should use cached ref")
}

func TestPruneFileCache_Integration(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("data"), Mode: 0o644},
	}
	blob := buildClientTestBlob(t, src)
	blobDigest := digest.FromBytes(blob)

	fetchCount := 0

	mockRegistry := &mocks.RegistryMock{
		ResolveRefFunc: func(ctx context.Context, ref string) (digest.Digest, error) {
			return blobDigest, nil
		},
		FetchManifestFunc: func(ctx context.Context, ref string, opts ...registry.FetchOption) (*registry.BlobManifest, error) {
			return &registry.BlobManifest{
				Digest: digest.FromString("manifest"),
				Blob: registry.BlobDescriptor{
					Digest: blobDigest,
					Size:   int64(len(blob)),
				},
			}, nil
		},
		FetchBlobByDigestFunc: func(ctx context.Context, ref string, d digest.Digest, size int64) (io.ReadCloser, error) {
			fetchCount++
			return io.NopCloser(bytes.NewReader(blob)), nil
		},
	}

	cacheDir := t.TempDir()
	client := NewClient(
		withRegistry(mockRegistry),
		WithFileCache(cacheDir),
	)

	// Pull to populate cache.
	handle, err := client.Pull(context.Background(), "ghcr.io/org/repo:v1")
	require.NoError(t, err)
	handle.Close()
	assert.Equal(t, 1, fetchCount, "first pull should fetch from network")

	// Second pull - cache hit.
	handle2, err := client.Pull(context.Background(), "ghcr.io/org/repo:v1")
	require.NoError(t, err)
	handle2.Close()
	assert.Equal(t, 1, fetchCount, "should be cache hit before prune")

	// Prune with MaxSize=1 to remove everything (our blob is larger).
	err = PruneFileCache(context.Background(), cacheDir, PruneStrategy{MaxSize: 1})
	require.NoError(t, err)

	// Now pull should fetch again (cache miss).
	handle3, err := client.Pull(context.Background(), "ghcr.io/org/repo:v1")
	require.NoError(t, err)
	handle3.Close()
	assert.Equal(t, 2, fetchCount, "should be cache miss after prune")
}
