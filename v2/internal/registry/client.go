package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/meigma/blobber/v2/internal/cache"
	"github.com/meigma/blobber/v2/internal/estargz"
	"github.com/meigma/blobber/v2/internal/oci"
)

// Compile-time interface check.
var _ Registry = (*registry)(nil)

// eStargz TOC digest annotation key.
const tocDigestAnnotation = "containerd.io/snapshot/stargz/toc.digest"

type registry struct {
	client        oci.Client
	logger        *slog.Logger
	manifestCache cache.ManifestCache
}

// Option configures a Registry.
type Option func(*registry)

// WithLogger sets the logger for debug output.
func WithLogger(logger *slog.Logger) Option {
	return func(r *registry) {
		r.logger = logger
	}
}

// WithManifestCache enables caching of manifest bytes by digest.
func WithManifestCache(cache cache.ManifestCache) Option {
	return func(r *registry) {
		r.manifestCache = cache
	}
}

// New creates a new Registry backed by the given OCI client.
func New(client oci.Client, opts ...Option) Registry {
	r := &registry{
		client: client,
	}
	for _, opt := range opts {
		opt(r)
	}
	if r.logger == nil {
		r.logger = slog.New(slog.DiscardHandler)
	}
	return r
}

// Push uploads a blob and creates a single-layer manifest.
func (r *registry) Push(ctx context.Context, ref string, blob io.Reader, metadata PushMetadata) (*PushResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Parse and validate reference.
	parsedRef, err := oci.ParseReference(ref)
	if err != nil {
		return nil, fmt.Errorf("parse reference: %w", err)
	}

	tag := parsedRef.Tag()
	if tag == "" {
		return nil, fmt.Errorf("reference must include a tag: %s", ref)
	}

	// Validate required metadata.
	if metadata.TOCDigest == "" {
		return nil, errors.New("TOCDigest is required")
	}

	// Push the blob.
	blobDesc := ocispec.Descriptor{
		MediaType: metadata.MediaType,
		Digest:    metadata.BlobDigest,
		Size:      metadata.BlobSize,
		Annotations: map[string]string{
			tocDigestAnnotation: metadata.TOCDigest.String(),
		},
	}

	if err = r.client.PushBlob(ctx, ref, blobDesc, blob); err != nil {
		return nil, fmt.Errorf("push blob: %w", err)
	}

	// Create and push config.
	config := ocispec.Image{
		Platform: ocispec.Platform{
			Architecture: "unknown",
			OS:           "unknown",
		},
		RootFS: ocispec.RootFS{
			Type:    "layers",
			DiffIDs: []digest.Digest{metadata.UncompressedDigest},
		},
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageConfig,
		Digest:    digest.FromBytes(configJSON),
		Size:      int64(len(configJSON)),
	}

	if err = r.client.PushBlob(ctx, ref, configDesc, bytes.NewReader(configJSON)); err != nil {
		return nil, fmt.Errorf("push config: %w", err)
	}

	// Create and push manifest.
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{blobDesc},
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	manifestDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manifestJSON),
		Size:      int64(len(manifestJSON)),
	}

	if err = r.client.PushManifest(ctx, ref, manifestDesc, manifestJSON); err != nil {
		return nil, fmt.Errorf("push manifest: %w", err)
	}

	// Tag the manifest.
	if err = r.client.Tag(ctx, ref, manifestDesc, tag); err != nil {
		return nil, fmt.Errorf("tag manifest: %w", err)
	}

	// Construct digest reference.
	digestRef := parsedRef.RepositoryReference() + "@" + manifestDesc.Digest.String()

	r.logger.Debug("pushed blob",
		"ref", ref,
		"digest", manifestDesc.Digest,
		"blob_size", metadata.BlobSize,
	)

	return &PushResult{
		Reference: digestRef,
		Manifest: BlobManifest{
			Digest: manifestDesc.Digest,
			Size:   manifestDesc.Size,
			Blob: BlobDescriptor{
				Digest:    metadata.BlobDigest,
				Size:      metadata.BlobSize,
				MediaType: metadata.MediaType,
			},
			Referrers: nil, // No referrers on fresh push
		},
	}, nil
}

// FetchManifest retrieves a blob's manifest and its referrers.
func (r *registry) FetchManifest(ctx context.Context, ref string, opts ...FetchOption) (*BlobManifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	options := &fetchOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Fetch the manifest (with optional caching).
	manifestBytes, manifestDesc, cacheHit, err := r.fetchManifest(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}

	result, err := manifestFromBytes(manifestBytes, manifestDesc)
	if err != nil && cacheHit {
		r.logger.Debug("cached manifest invalid, refetching", "ref", ref, "digest", manifestDesc.Digest, "error", err)
		manifestBytes, manifestDesc, err = r.client.FetchManifest(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("fetch manifest: %w", err)
		}
		r.storeManifest(manifestDesc.Digest, manifestBytes)
		result, err = manifestFromBytes(manifestBytes, manifestDesc)
	}
	if err != nil {
		return nil, err
	}

	// Fetch referrers unless skipped.
	if !options.skipReferrers {
		referrers, err := r.client.ListReferrers(ctx, ref, manifestDesc.Digest.String(), "")
		if err != nil {
			return nil, fmt.Errorf("list referrers: %w", err)
		}

		result.Referrers = make([]Referrer, len(referrers))
		for i, desc := range referrers {
			result.Referrers[i] = Referrer{
				Digest:       desc.Digest,
				ArtifactType: desc.ArtifactType,
				Size:         desc.Size,
				Annotations:  desc.Annotations,
			}
		}
	}

	r.logger.Debug("fetched manifest",
		"ref", ref,
		"digest", manifestDesc.Digest,
		"referrer_count", len(result.Referrers),
	)

	return result, nil
}

func (r *registry) fetchManifest(ctx context.Context, ref string) ([]byte, ocispec.Descriptor, bool, error) {
	if r.manifestCache == nil {
		manifestBytes, manifestDesc, err := r.client.FetchManifest(ctx, ref)
		return manifestBytes, manifestDesc, false, err
	}

	parsedRef, err := oci.ParseReference(ref)
	if err != nil {
		manifestBytes, manifestDesc, fetchErr := r.client.FetchManifest(ctx, ref)
		return manifestBytes, manifestDesc, false, fetchErr
	}

	digestRef := parsedRef.Digest()
	if digestRef != "" {
		d, err := digest.Parse(digestRef)
		if err == nil {
			if manifestBytes, ok := r.manifestCache.Load(d); ok && len(manifestBytes) > 0 {
				actual := digest.FromBytes(manifestBytes)
				if actual == d {
					r.logger.Debug("manifest cache hit", "ref", ref, "digest", d)
					return manifestBytes, ocispec.Descriptor{
						Digest: d,
						Size:   int64(len(manifestBytes)),
					}, true, nil
				}
				r.logger.Debug("manifest cache digest mismatch", "ref", ref, "expected", d, "actual", actual)
			}
		}
	}

	manifestBytes, manifestDesc, err := r.client.FetchManifest(ctx, ref)
	if err != nil {
		return nil, ocispec.Descriptor{}, false, err
	}
	r.storeManifest(manifestDesc.Digest, manifestBytes)
	return manifestBytes, manifestDesc, false, nil
}

func (r *registry) storeManifest(d digest.Digest, raw []byte) {
	if r.manifestCache == nil || d == "" || len(raw) == 0 {
		return
	}
	if err := r.manifestCache.Store(d, raw); err != nil {
		r.logger.Debug("manifest cache store failed", "digest", d, "error", err)
	}
}

func manifestFromBytes(manifestBytes []byte, manifestDesc ocispec.Descriptor) (*BlobManifest, error) {
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	if len(manifest.Layers) != 1 {
		return nil, fmt.Errorf("invalid manifest: expected 1 layer, got %d", len(manifest.Layers))
	}

	layer := manifest.Layers[0]

	return &BlobManifest{
		Digest: manifestDesc.Digest,
		Size:   manifestDesc.Size,
		Blob: BlobDescriptor{
			Digest:    layer.Digest,
			Size:      layer.Size,
			MediaType: layer.MediaType,
		},
		Raw: manifestBytes,
	}, nil
}

// ResolveRef resolves a ref to its blob digest without fetching the blob.
func (r *registry) ResolveRef(ctx context.Context, ref string) (digest.Digest, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	manifest, err := r.FetchManifest(ctx, ref, WithoutReferrers())
	if err != nil {
		return "", err
	}

	r.logger.Debug("resolved ref",
		"ref", ref,
		"digest", manifest.Blob.Digest,
	)

	return manifest.Blob.Digest, nil
}

// FetchBlob downloads the entire blob content.
func (r *registry) FetchBlob(ctx context.Context, ref string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Fetch manifest to get blob descriptor.
	manifest, err := r.FetchManifest(ctx, ref, WithoutReferrers())
	if err != nil {
		return nil, err
	}

	return r.FetchBlobByDigest(ctx, ref, manifest.Blob.Digest, manifest.Blob.Size)
}

// FetchBlobByDigest downloads the blob using a known digest.
func (r *registry) FetchBlobByDigest(ctx context.Context, ref string, d digest.Digest, size int64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	blobDesc := ocispec.Descriptor{
		Digest: d,
		Size:   size,
	}

	rc, err := r.client.FetchBlob(ctx, ref, blobDesc)
	if err != nil {
		return nil, fmt.Errorf("fetch blob: %w", err)
	}

	r.logger.Debug("fetched blob by digest",
		"ref", ref,
		"digest", d,
		"size", size,
	)

	return rc, nil
}

// OpenBlob returns a reader for random access to the blob.
func (r *registry) OpenBlob(ctx context.Context, ref string) (estargz.BlobSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Fetch manifest to get blob descriptor.
	manifest, err := r.FetchManifest(ctx, ref, WithoutReferrers())
	if err != nil {
		return nil, err
	}

	return r.OpenBlobByDigest(ctx, ref, manifest.Blob.Digest, manifest.Blob.Size)
}

// OpenBlobByDigest returns a reader for random access using a known digest.
func (r *registry) OpenBlobByDigest(ctx context.Context, ref string, d digest.Digest, size int64) (estargz.BlobSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	blobDesc := ocispec.Descriptor{
		Digest: d,
		Size:   size,
	}

	reader, err := r.client.BlobReader(ctx, ref, blobDesc)
	if err != nil {
		return nil, fmt.Errorf("create blob reader: %w", err)
	}

	r.logger.Debug("opened blob by digest",
		"ref", ref,
		"digest", d,
		"size", size,
	)

	return reader, nil
}

// AttachArtifact attaches content to a manifest as a referrer.
func (r *registry) AttachArtifact(ctx context.Context, ref, artifactType string, content []byte, annotations map[string]string) (digest.Digest, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Resolve the subject manifest.
	subjectDesc, err := r.client.ResolveManifest(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("resolve manifest: %w", err)
	}

	// Create artifact descriptor.
	artifactDesc := ocispec.Descriptor{
		MediaType:   artifactType,
		Digest:      digest.FromBytes(content),
		Size:        int64(len(content)),
		Annotations: annotations,
	}

	// Push the referrer.
	manifestDigest, err := r.client.PushReferrer(ctx, ref, subjectDesc, artifactDesc, content)
	if err != nil {
		return "", fmt.Errorf("push referrer: %w", err)
	}

	r.logger.Debug("attached artifact",
		"ref", ref,
		"artifact_type", artifactType,
		"manifest_digest", manifestDigest,
	)

	return manifestDigest, nil
}

// FetchReferrerContent downloads the content of a referrer artifact.
//
// The referrerDigest is the digest of the referrer manifest (as returned by
// AttachArtifact or found in Referrer.Digest from FetchManifest).
// This method fetches the referrer manifest, validates it has a single layer,
// and returns the content of that layer.
func (r *registry) FetchReferrerContent(ctx context.Context, ref string, referrerDigest digest.Digest) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Parse the original reference to get the repository.
	parsedRef, err := oci.ParseReference(ref)
	if err != nil {
		return nil, fmt.Errorf("parse reference: %w", err)
	}

	// Construct a digest reference for the referrer manifest.
	digestRef := parsedRef.RepositoryReference() + "@" + referrerDigest.String()

	// Fetch the referrer manifest.
	manifestBytes, _, err := r.client.FetchManifest(ctx, digestRef)
	if err != nil {
		return nil, fmt.Errorf("fetch referrer manifest: %w", err)
	}

	// Parse the manifest.
	var manifest ocispec.Manifest
	if unmarshalErr := json.Unmarshal(manifestBytes, &manifest); unmarshalErr != nil {
		return nil, fmt.Errorf("parse referrer manifest: %w", unmarshalErr)
	}

	// Validate single-layer constraint.
	if len(manifest.Layers) != 1 {
		return nil, fmt.Errorf("invalid referrer manifest: expected 1 layer, got %d", len(manifest.Layers))
	}

	layer := manifest.Layers[0]

	// Fetch the layer blob content.
	rc, err := r.client.FetchBlob(ctx, ref, layer)
	if err != nil {
		return nil, fmt.Errorf("fetch referrer content: %w", err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read referrer content: %w", err)
	}

	r.logger.Debug("fetched referrer content",
		"ref", ref,
		"manifest_digest", referrerDigest,
		"layer_digest", layer.Digest,
		"size", len(content),
	)

	return content, nil
}
