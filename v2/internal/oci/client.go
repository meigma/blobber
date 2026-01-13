package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"

	"github.com/meigma/blobber/v2/internal/estargz"
)

// Compile-time interface check.
var _ Client = (*client)(nil)

// client implements Client using ORAS.
type client struct {
	plainHTTP bool
	userAgent string
	credStore credentials.Store
	logger    *slog.Logger
	rangeHook func(offset, length int64)
}

// NewClient creates a new Client with the given options.
func NewClient(opts ...Option) Client {
	c := &client{
		userAgent: "blobber/2.0",
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.logger == nil {
		c.logger = slog.New(slog.DiscardHandler)
	}
	return c
}

// newRepository creates an authenticated remote repository.
func (c *client) newRepository(ref Reference) (*remote.Repository, error) {
	repo, err := remote.NewRepository(ref.RepositoryReference())
	if err != nil {
		return nil, err
	}

	repo.PlainHTTP = c.plainHTTP
	repo.Client = &auth.Client{
		Client:     retry.DefaultClient,
		Cache:      auth.NewCache(),
		Credential: c.credentialFunc(),
		Header: http.Header{
			"User-Agent": []string{c.userAgent},
		},
	}

	return repo, nil
}

// credentialFunc returns the credential function for authentication.
// If no credential store is configured, returns nil (anonymous access).
// Uses ORAS's built-in address mapping for Docker Hub compatibility.
func (c *client) credentialFunc() auth.CredentialFunc {
	if c.credStore == nil {
		return nil
	}
	return credentials.Credential(c.credStore)
}

// PushBlob uploads a blob to the registry.
//
//nolint:gocritic // hugeParam: desc passed by value to match OCI ecosystem patterns
func (c *client) PushBlob(ctx context.Context, ref string, desc ocispec.Descriptor, content io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	parsedRef, err := ParseReference(ref)
	if err != nil {
		return err
	}

	repo, err := c.newRepository(parsedRef)
	if err != nil {
		return fmt.Errorf("create repository: %w", err)
	}

	if err := repo.Blobs().Push(ctx, desc, content); err != nil {
		return mapError(err)
	}

	c.logger.Debug("pushed blob", "ref", ref, "digest", desc.Digest, "size", desc.Size)
	return nil
}

// FetchBlob downloads a blob from the registry.
//
//nolint:gocritic // hugeParam: desc passed by value to match OCI ecosystem patterns
func (c *client) FetchBlob(ctx context.Context, ref string, desc ocispec.Descriptor) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	parsedRef, err := ParseReference(ref)
	if err != nil {
		return nil, err
	}

	repo, err := c.newRepository(parsedRef)
	if err != nil {
		return nil, fmt.Errorf("create repository: %w", err)
	}

	rc, err := repo.Blobs().Fetch(ctx, desc)
	if err != nil {
		return nil, mapError(err)
	}

	c.logger.Debug("fetched blob", "ref", ref, "digest", desc.Digest, "size", desc.Size)
	return rc, nil
}

// FetchBlobRange downloads a byte range from a blob.
//
//nolint:gocritic // hugeParam: desc passed by value to match OCI ecosystem patterns
func (c *client) FetchBlobRange(ctx context.Context, ref string, desc ocispec.Descriptor, offset, length int64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if c.rangeHook != nil {
		c.rangeHook(offset, length)
	}

	// Validate range parameters.
	if offset < 0 {
		return nil, errors.New("offset must be non-negative")
	}
	if length <= 0 {
		return nil, errors.New("length must be positive")
	}
	if offset > math.MaxInt64-length {
		return nil, errors.New("range overflow: offset + length exceeds maximum")
	}

	// Validate against blob size.
	if offset >= desc.Size {
		return nil, io.EOF
	}

	parsedRef, err := ParseReference(ref)
	if err != nil {
		return nil, err
	}

	repo, err := c.newRepository(parsedRef)
	if err != nil {
		return nil, fmt.Errorf("create repository: %w", err)
	}

	return c.fetchRange(ctx, parsedRef, repo, desc.Digest.String(), offset, length)
}

// fetchRange performs the HTTP range request.
func (c *client) fetchRange(ctx context.Context, ref Reference, repo *remote.Repository, blobDigest string, offset, length int64) (io.ReadCloser, error) {
	scheme := "https"
	if c.plainHTTP {
		scheme = "http"
	}
	blobURL := &url.URL{
		Scheme: scheme,
		Host:   ref.Registry(),
	}
	blobURL = blobURL.JoinPath("v2", ref.Repository(), "blobs", blobDigest)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, blobURL.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))

	resp, err := repo.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch range: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusPartialContent:
		c.logger.Debug("fetched blob range", "url", blobURL.String(), "offset", offset, "length", length)
		return resp.Body, nil
	case http.StatusOK:
		// Registry ignored Range header and returned full blob.
		resp.Body.Close()
		return nil, ErrRangeNotSupported
	case http.StatusUnauthorized, http.StatusForbidden:
		resp.Body.Close()
		return nil, ErrUnauthorized
	case http.StatusNotFound:
		resp.Body.Close()
		return nil, ErrNotFound
	case http.StatusRequestedRangeNotSatisfiable:
		resp.Body.Close()
		return nil, io.EOF
	default:
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

// PushManifest uploads a manifest to the registry.
//
//nolint:gocritic // hugeParam: desc passed by value to match OCI ecosystem patterns
func (c *client) PushManifest(ctx context.Context, ref string, desc ocispec.Descriptor, content []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	parsedRef, err := ParseReference(ref)
	if err != nil {
		return err
	}

	repo, err := c.newRepository(parsedRef)
	if err != nil {
		return fmt.Errorf("create repository: %w", err)
	}

	if err := repo.Manifests().Push(ctx, desc, bytes.NewReader(content)); err != nil {
		return mapError(err)
	}

	c.logger.Debug("pushed manifest", "ref", ref, "digest", desc.Digest)
	return nil
}

// FetchManifest downloads a manifest from the registry.
func (c *client) FetchManifest(ctx context.Context, ref string) ([]byte, ocispec.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, ocispec.Descriptor{}, err
	}

	parsedRef, err := ParseReference(ref)
	if err != nil {
		return nil, ocispec.Descriptor{}, err
	}

	repo, err := c.newRepository(parsedRef)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("create repository: %w", err)
	}

	// Use the tag or digest from the reference.
	reference := parsedRef.Tag()
	if reference == "" {
		reference = parsedRef.Digest()
	}

	desc, rc, err := repo.Manifests().FetchReference(ctx, reference)
	if err != nil {
		return nil, ocispec.Descriptor{}, mapError(err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("read manifest: %w", err)
	}

	c.logger.Debug("fetched manifest", "ref", ref, "digest", desc.Digest)
	return content, desc, nil
}

// ResolveManifest resolves a reference to its manifest descriptor.
func (c *client) ResolveManifest(ctx context.Context, ref string) (ocispec.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return ocispec.Descriptor{}, err
	}

	parsedRef, err := ParseReference(ref)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	repo, err := c.newRepository(parsedRef)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("create repository: %w", err)
	}

	reference := parsedRef.Tag()
	if reference == "" {
		reference = parsedRef.Digest()
	}

	desc, err := repo.Manifests().Resolve(ctx, reference)
	if err != nil {
		return ocispec.Descriptor{}, mapError(err)
	}

	c.logger.Debug("resolved manifest", "ref", ref, "digest", desc.Digest)
	return desc, nil
}

// Tag associates a tag with a manifest.
//
//nolint:gocritic // hugeParam: desc passed by value to match OCI ecosystem patterns
func (c *client) Tag(ctx context.Context, ref string, desc ocispec.Descriptor, tag string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	parsedRef, err := ParseReference(ref)
	if err != nil {
		return err
	}

	repo, err := c.newRepository(parsedRef)
	if err != nil {
		return fmt.Errorf("create repository: %w", err)
	}

	if err := repo.Tag(ctx, desc, tag); err != nil {
		return mapError(err)
	}

	c.logger.Debug("tagged manifest", "ref", ref, "digest", desc.Digest, "tag", tag)
	return nil
}

// PushReferrer uploads a referrer artifact that references a subject.
//
//nolint:gocritic // hugeParam: descriptors passed by value to match OCI ecosystem patterns
func (c *client) PushReferrer(ctx context.Context, ref string, subject ocispec.Descriptor, artifact ocispec.Descriptor, content []byte) (digest.Digest, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	parsedRef, err := ParseReference(ref)
	if err != nil {
		return "", err
	}

	repo, err := c.newRepository(parsedRef)
	if err != nil {
		return "", fmt.Errorf("create repository: %w", err)
	}

	// Push the artifact content as a blob.
	if pushErr := repo.Blobs().Push(ctx, artifact, bytes.NewReader(content)); pushErr != nil {
		return "", fmt.Errorf("push artifact blob: %w", mapError(pushErr))
	}

	// Create empty config (OCI 1.1 artifact pattern).
	emptyConfig := []byte("{}")
	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeEmptyJSON,
		Digest:    digest.FromBytes(emptyConfig),
		Size:      int64(len(emptyConfig)),
	}

	if pushErr := repo.Blobs().Push(ctx, configDesc, bytes.NewReader(emptyConfig)); pushErr != nil {
		return "", fmt.Errorf("push config: %w", mapError(pushErr))
	}

	// Create manifest with subject reference.
	manifest := ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: artifact.MediaType,
		Config:       configDesc,
		Layers:       []ocispec.Descriptor{artifact},
		Subject:      &subject,
		Annotations:  artifact.Annotations,
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("marshal manifest: %w", err)
	}

	manifestDesc := ocispec.Descriptor{
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: artifact.MediaType,
		Digest:       digest.FromBytes(manifestJSON),
		Size:         int64(len(manifestJSON)),
	}

	if err := repo.Manifests().Push(ctx, manifestDesc, bytes.NewReader(manifestJSON)); err != nil {
		return "", fmt.Errorf("push manifest: %w", mapError(err))
	}

	c.logger.Debug("pushed referrer", "ref", ref, "subject", subject.Digest, "artifact", artifact.Digest)
	return manifestDesc.Digest, nil
}

// ListReferrers returns all referrers for a subject digest.
func (c *client) ListReferrers(ctx context.Context, ref, subjectDigest, artifactType string) ([]ocispec.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	parsedRef, err := ParseReference(ref)
	if err != nil {
		return nil, err
	}

	repo, err := c.newRepository(parsedRef)
	if err != nil {
		return nil, fmt.Errorf("create repository: %w", err)
	}

	subjDigest, err := digest.Parse(subjectDigest)
	if err != nil {
		return nil, fmt.Errorf("parse subject digest: %w", err)
	}

	subjectDesc := ocispec.Descriptor{
		Digest: subjDigest,
	}

	var referrers []ocispec.Descriptor
	err = repo.Referrers(ctx, subjectDesc, artifactType, func(refs []ocispec.Descriptor) error {
		referrers = append(referrers, refs...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list referrers: %w", mapError(err))
	}

	c.logger.Debug("listed referrers", "ref", ref, "subject", subjectDigest, "count", len(referrers))
	return referrers, nil
}

// BlobReader creates a BlobSource for random access to a remote blob.
//
//nolint:gocritic // hugeParam: desc passed by value to match OCI ecosystem patterns
func (c *client) BlobReader(ctx context.Context, ref string, desc ocispec.Descriptor) (estargz.BlobSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	parsedRef, err := ParseReference(ref)
	if err != nil {
		return nil, err
	}

	// Validate the repository exists by creating it (doesn't make network call).
	if _, err := c.newRepository(parsedRef); err != nil {
		return nil, fmt.Errorf("create repository: %w", err)
	}

	c.logger.Debug("created blob reader", "ref", ref, "digest", desc.Digest, "size", desc.Size)
	return newBlobReader(ctx, c, ref, desc), nil
}
