// Package registry provides OCI registry operations using ORAS.
package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"

	"github.com/gilmanlab/blobber/core"
)

// Compile-time interface implementation check.
var _ core.Registry = (*orasRegistry)(nil)

// Option configures an orasRegistry.
type Option func(*orasRegistry)

// orasRegistry implements blobber.Registry using ORAS.
type orasRegistry struct {
	plainHTTP bool
	userAgent string
	credStore credentials.Store
}

// New creates a new Registry backed by ORAS.
func New(opts ...Option) *orasRegistry {
	r := &orasRegistry{
		userAgent: "blobber/1.0",
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Push uploads a blob and creates a manifest using streaming.
// Returns the manifest digest.
//
// The opts.BlobDigest and opts.BlobSize fields are required for streaming push.
// These are computed during archive build to avoid loading the entire blob into memory.
//
//nolint:gocyclo // Registry push has multiple required steps that cannot be easily decomposed
func (r *orasRegistry) Push(ctx context.Context, ref string, layer io.Reader, opts *core.RegistryPushOptions) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Require pre-computed digests and size for streaming push.
	var missing []string
	if opts.DiffID == "" {
		missing = append(missing, "DiffID")
	}
	if opts.BlobDigest == "" {
		missing = append(missing, "BlobDigest")
	}
	if opts.BlobSize == 0 {
		missing = append(missing, "BlobSize")
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("required fields missing for streaming push: %s", strings.Join(missing, ", "))
	}

	parsedRef, err := registry.ParseReference(ref)
	if err != nil {
		return "", core.ErrInvalidRef
	}

	repo, err := r.newRepository(parsedRef)
	if err != nil {
		return "", fmt.Errorf("create repository: %w", err)
	}

	blobDigest, err := digest.Parse(opts.BlobDigest)
	if err != nil {
		return "", fmt.Errorf("parse blob digest: %w", err)
	}

	diffID, err := digest.Parse(opts.DiffID)
	if err != nil {
		return "", fmt.Errorf("parse diff id: %w", err)
	}

	// Build layer annotations.
	annotations := make(map[string]string)
	for k, v := range opts.Annotations {
		annotations[k] = v
	}
	if opts.TOCDigest != "" {
		annotations["containerd.io/snapshot/stargz/toc.digest"] = opts.TOCDigest
	}

	// Create layer descriptor.
	mediaType := opts.MediaType
	if mediaType == "" {
		mediaType = "application/vnd.oci.image.layer.v1.tar+gzip"
	}

	layerDesc := ocispec.Descriptor{
		MediaType:   mediaType,
		Digest:      blobDigest,
		Size:        opts.BlobSize,
		Annotations: annotations,
	}

	// Push blob directly (streams from reader).
	if err = repo.Blobs().Push(ctx, layerDesc, layer); err != nil {
		return "", fmt.Errorf("push layer: %w", mapError(err))
	}

	// Create and push minimal valid OCI config.
	// Per OCI spec, config must have architecture, os, and rootfs.
	// DiffIDs must be the uncompressed layer digest, not the compressed blob digest.
	config := ocispec.Image{
		Platform: ocispec.Platform{
			Architecture: runtime.GOARCH,
			OS:           runtime.GOOS,
		},
		RootFS: ocispec.RootFS{
			Type:    "layers",
			DiffIDs: []digest.Digest{diffID},
		},
	}
	configData, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}

	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageConfig,
		Digest:    digest.FromBytes(configData),
		Size:      int64(len(configData)),
	}

	if err = repo.Blobs().Push(ctx, configDesc, bytes.NewReader(configData)); err != nil {
		return "", fmt.Errorf("push config: %w", mapError(err))
	}

	// Create manifest.
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{layerDesc},
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("marshal manifest: %w", err)
	}

	manifestDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manifestJSON),
		Size:      int64(len(manifestJSON)),
	}

	// Push manifest.
	if err = repo.Manifests().Push(ctx, manifestDesc, bytes.NewReader(manifestJSON)); err != nil {
		return "", fmt.Errorf("push manifest: %w", mapError(err))
	}

	// Tag the manifest.
	if err = repo.Tag(ctx, manifestDesc, parsedRef.Reference); err != nil {
		return "", fmt.Errorf("tag manifest: %w", mapError(err))
	}

	return manifestDesc.Digest.String(), nil
}

// Pull returns a reader for the image's layer blob and its size.
func (r *orasRegistry) Pull(ctx context.Context, ref string) (io.ReadCloser, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}

	parsedRef, err := registry.ParseReference(ref)
	if err != nil {
		return nil, 0, core.ErrInvalidRef
	}

	repo, err := r.newRepository(parsedRef)
	if err != nil {
		return nil, 0, fmt.Errorf("create repository: %w", err)
	}

	// Resolve layer descriptor.
	layerDesc, err := r.resolveLayerDescriptor(ctx, repo, parsedRef.Reference)
	if err != nil {
		if errors.Is(err, ErrMultipleLayers) {
			return nil, 0, fmt.Errorf("%s: %w: %w", ref, core.ErrInvalidArchive, err)
		}
		return nil, 0, err
	}

	// Fetch the layer blob.
	blobReader, err := repo.Blobs().Fetch(ctx, layerDesc)
	if err != nil {
		return nil, 0, mapError(err)
	}

	return blobReader, layerDesc.Size, nil
}

// PullRange fetches a byte range from the layer blob.
// Used for selective file retrieval from eStargz.
//
//nolint:gocyclo // Range request has multiple validation and response handling paths
func (r *orasRegistry) PullRange(ctx context.Context, ref string, offset, length int64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Validate range parameters.
	if offset < 0 {
		return nil, errors.New("offset must be non-negative")
	}
	if length <= 0 {
		return nil, errors.New("length must be positive")
	}
	// Ensure offset + length won't overflow int64.
	if offset > math.MaxInt64-length {
		return nil, errors.New("range overflow: offset + length exceeds maximum")
	}

	parsedRef, err := registry.ParseReference(ref)
	if err != nil {
		return nil, core.ErrInvalidRef
	}

	repo, err := r.newRepository(parsedRef)
	if err != nil {
		return nil, fmt.Errorf("create repository: %w", err)
	}

	// Resolve layer descriptor.
	layerDesc, err := r.resolveLayerDescriptor(ctx, repo, parsedRef.Reference)
	if err != nil {
		if errors.Is(err, ErrMultipleLayers) {
			return nil, fmt.Errorf("%s: %w: %w", ref, core.ErrInvalidArchive, err)
		}
		return nil, err
	}

	// Construct blob URL for range request.
	scheme := "https"
	if r.plainHTTP {
		scheme = "http"
	}
	blobURL := &url.URL{
		Scheme: scheme,
		Host:   parsedRef.Registry,
	}
	blobURL = blobURL.JoinPath("v2", parsedRef.Repository, "blobs", layerDesc.Digest.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, blobURL.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set Range header for partial content.
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))

	// Use the repository's auth client for proper token/bearer auth.
	resp, err := repo.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch range: %w", err)
	}

	// Check for successful response.
	switch resp.StatusCode {
	case http.StatusPartialContent:
		// Success - registry supports Range requests.
		return resp.Body, nil
	case http.StatusOK:
		// Registry ignored Range header and returned full blob.
		resp.Body.Close()
		return nil, ErrRangeNotSupported
	case http.StatusUnauthorized, http.StatusForbidden:
		resp.Body.Close()
		return nil, core.ErrUnauthorized
	case http.StatusNotFound:
		resp.Body.Close()
		return nil, core.ErrNotFound
	default:
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

// WithCredentialStore sets the credential store.
func WithCredentialStore(store credentials.Store) Option {
	return func(r *orasRegistry) {
		r.credStore = store
	}
}

// WithPlainHTTP enables insecure HTTP connections.
func WithPlainHTTP(plainHTTP bool) Option {
	return func(r *orasRegistry) {
		r.plainHTTP = plainHTTP
	}
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(r *orasRegistry) {
		r.userAgent = ua
	}
}

// newRepository creates an authenticated remote repository.
func (r *orasRegistry) newRepository(ref registry.Reference) (*remote.Repository, error) {
	repoRef := fmt.Sprintf("%s/%s", ref.Registry, ref.Repository)
	repo, err := remote.NewRepository(repoRef)
	if err != nil {
		return nil, err
	}

	repo.PlainHTTP = r.plainHTTP
	repo.Client = &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
		Credential: func(ctx context.Context, hostport string) (auth.Credential, error) {
			if r.credStore == nil {
				return auth.EmptyCredential, nil
			}
			return r.credStore.Get(ctx, hostport)
		},
		Header: http.Header{
			"User-Agent": []string{r.userAgent},
		},
	}

	return repo, nil
}

// resolveLayerDescriptor fetches the manifest and returns the layer descriptor.
// Handles both single-arch manifests and multi-arch manifest lists (OCI index).
// Returns an error if the manifest has multiple layers, as blobber images have exactly one.
func (r *orasRegistry) resolveLayerDescriptor(ctx context.Context, repo *remote.Repository, reference string) (ocispec.Descriptor, error) {
	desc, manifestReader, err := repo.Manifests().FetchReference(ctx, reference)
	if err != nil {
		return ocispec.Descriptor{}, mapError(err)
	}
	defer manifestReader.Close()

	manifestData, err := io.ReadAll(manifestReader)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("read manifest: %w", err)
	}

	// Check if this is an OCI index (multi-arch manifest list).
	if isIndex(desc.MediaType) {
		return r.resolveFromIndex(ctx, repo, manifestData)
	}

	// Single-arch manifest - decode directly.
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("decode manifest: %w", err)
	}

	if len(manifest.Layers) == 0 {
		return ocispec.Descriptor{}, core.ErrNotFound
	}

	if len(manifest.Layers) > 1 {
		return ocispec.Descriptor{}, ErrMultipleLayers
	}

	return manifest.Layers[0], nil
}

// resolveFromIndex selects a manifest from an OCI index and returns its layer descriptor.
// Prefers the current runtime platform, falls back to the first manifest if not found.
// Returns an error if the selected manifest has multiple layers.
func (r *orasRegistry) resolveFromIndex(ctx context.Context, repo *remote.Repository, indexData []byte) (ocispec.Descriptor, error) {
	var index ocispec.Index
	if err := json.Unmarshal(indexData, &index); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("decode index: %w", err)
	}

	if len(index.Manifests) == 0 {
		return ocispec.Descriptor{}, core.ErrNotFound
	}

	// Find a suitable manifest - prefer current runtime platform.
	var selected *ocispec.Descriptor
	for i := range index.Manifests {
		m := &index.Manifests[i]
		if m.Platform != nil && m.Platform.OS == runtime.GOOS && m.Platform.Architecture == runtime.GOARCH {
			selected = m
			break
		}
	}
	if selected == nil {
		// Fall back to first manifest.
		selected = &index.Manifests[0]
	}

	// Fetch the selected manifest.
	manifestReader, err := repo.Manifests().Fetch(ctx, *selected)
	if err != nil {
		return ocispec.Descriptor{}, mapError(err)
	}
	defer manifestReader.Close()

	var manifest ocispec.Manifest
	if err := json.NewDecoder(manifestReader).Decode(&manifest); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("decode manifest: %w", err)
	}

	if len(manifest.Layers) == 0 {
		return ocispec.Descriptor{}, core.ErrNotFound
	}

	if len(manifest.Layers) > 1 {
		return ocispec.Descriptor{}, ErrMultipleLayers
	}

	return manifest.Layers[0], nil
}

// isIndex returns true if the media type indicates an OCI index or Docker manifest list.
func isIndex(mediaType string) bool {
	return mediaType == ocispec.MediaTypeImageIndex ||
		mediaType == "application/vnd.docker.distribution.manifest.list.v2+json"
}
