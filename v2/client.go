package blobber

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"

	"github.com/opencontainers/go-digest"

	"github.com/meigma/blobber/v2/internal/domain"
	"github.com/meigma/blobber/v2/internal/estargz"
	"github.com/meigma/blobber/v2/internal/oci"
	"github.com/meigma/blobber/v2/internal/registry"
)

// Client provides operations for pushing and pulling blobs to OCI registries.
type Client struct {
	registry registry.Registry
	logger   *slog.Logger
}

// withRegistry is an unexported option for injecting a registry (for testing).
func withRegistry(r registry.Registry) ClientOption {
	return func(o *clientOptions) {
		o.registry = r
	}
}

// NewClient creates a new Client with the given options.
func NewClient(opts ...ClientOption) *Client {
	options := &clientOptions{}
	for _, opt := range opts {
		opt(options)
	}

	logger := options.logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	// Use injected registry if provided (for testing).
	if options.registry != nil {
		if r, ok := options.registry.(registry.Registry); ok {
			return &Client{
				registry: r,
				logger:   logger,
			}
		}
	}

	// Create OCI client with configured options.
	var ociOpts []oci.Option
	if options.credStore != nil {
		ociOpts = append(ociOpts, oci.WithCredentialStore(options.credStore))
	}
	if options.plainHTTP {
		ociOpts = append(ociOpts, oci.WithPlainHTTP(true))
	}
	if options.userAgent != "" {
		ociOpts = append(ociOpts, oci.WithUserAgent(options.userAgent))
	}
	if options.logger != nil {
		ociOpts = append(ociOpts, oci.WithLogger(options.logger))
	}

	ociClient := oci.NewClient(ociOpts...)

	// Create registry with the OCI client.
	var regOpts []registry.Option
	if options.logger != nil {
		regOpts = append(regOpts, registry.WithLogger(options.logger))
	}

	reg := registry.New(ociClient, regOpts...)

	return &Client{
		registry: reg,
		logger:   logger,
	}
}

// Push creates an eStargz blob from the source filesystem and pushes it to the registry.
//
// The ref must include a tag (e.g., "ghcr.io/org/repo:v1").
// Returns the push result containing the digest reference and manifest metadata.
func (c *Client) Push(ctx context.Context, ref string, src fs.FS, opts ...PushOption) (*PushResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	options := &pushOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Create temp file for archive to avoid buffering in memory.
	tempFile, err := os.CreateTemp("", "blobber-push-*.estargz")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	// Build the eStargz archive.
	var builderOpts []estargz.BuilderOption
	if options.compression != nil {
		builderOpts = append(builderOpts, estargz.WithCompression(options.compression))
	}
	builderOpts = append(builderOpts, estargz.WithBuilderLogger(c.logger))

	builder := estargz.NewBuilder(builderOpts...)

	// Build the estargz archive to temp file.
	result, err := builder.Build(ctx, tempFile, src)
	if err != nil {
		tempFile.Close()
		return nil, fmt.Errorf("build archive: %w", err)
	}

	// Detect media type from magic bytes.
	mediaType, err := detectMediaType(tempFile)
	if err != nil {
		tempFile.Close()
		return nil, fmt.Errorf("detect media type: %w", err)
	}

	// Seek back to start for push.
	if _, seekErr := tempFile.Seek(0, io.SeekStart); seekErr != nil {
		tempFile.Close()
		return nil, fmt.Errorf("seek temp file: %w", seekErr)
	}

	// Construct push metadata from build result.
	metadata := domain.PushMetadata{
		BlobDigest:         result.BlobDigest,
		BlobSize:           result.BlobSize,
		UncompressedDigest: result.UncompressedDigest,
		TOCDigest:          result.TOCDigest,
		MediaType:          mediaType,
	}

	// Push to registry.
	pushResult, err := c.registry.Push(ctx, ref, tempFile, metadata)
	tempFile.Close()
	if err != nil {
		return nil, fmt.Errorf("push: %w", err)
	}

	c.logger.Debug("pushed blob",
		"ref", ref,
		"digest", pushResult.Manifest.Digest,
	)

	return pushResult, nil
}

// FetchManifest retrieves a blob's manifest and its referrers.
//
// The ref can be a tag or digest reference.
// Referrer metadata is eagerly fetched by default.
func (c *Client) FetchManifest(ctx context.Context, ref string) (*BlobManifest, error) {
	return c.registry.FetchManifest(ctx, ref)
}

// Pull downloads a blob and returns a BlobHandle for accessing its contents.
//
// The returned handle is backed by a local temp file for fast, seekable access.
// Call Close() when done to clean up the temp file.
//
// Use WithPullPolicy to verify the manifest before downloading.
func (c *Client) Pull(ctx context.Context, ref string, opts ...PullOption) (*BlobHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	options := &pullOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Verify policy if provided.
	if options.policy != nil {
		manifest, err := c.registry.FetchManifest(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("fetch manifest for policy: %w", err)
		}

		fetcher := &referrerFetcher{
			registry: c.registry,
			ref:      ref,
		}

		if err := options.policy.Verify(ctx, *manifest, fetcher); err != nil {
			return nil, fmt.Errorf("policy verification failed: %w", err)
		}
	}

	// Fetch the blob.
	rc, err := c.registry.FetchBlob(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("fetch blob: %w", err)
	}
	defer rc.Close()

	// Write to temp file.
	tempFile, err := os.CreateTemp("", "blobber-*.blob")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	if _, copyErr := io.Copy(tempFile, rc); copyErr != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return nil, fmt.Errorf("write temp file: %w", copyErr)
	}

	if closeErr := tempFile.Close(); closeErr != nil {
		os.Remove(tempPath)
		return nil, fmt.Errorf("close temp file: %w", closeErr)
	}

	// Create local-backed handle.
	handle, err := newLocalHandle(ctx, tempPath)
	if err != nil {
		os.Remove(tempPath)
		return nil, fmt.Errorf("create handle: %w", err)
	}

	c.logger.Debug("pulled blob",
		"ref", ref,
		"temp_path", tempPath,
	)

	return handle, nil
}

// Stream returns a BlobHandle for accessing blob contents via network range requests.
//
// The returned handle fetches file content on-demand without downloading the full blob.
// This is efficient for accessing a few files from a large blob.
// Call Close() when done to release resources.
//
// Use WithStreamPolicy to verify the manifest before streaming.
func (c *Client) Stream(ctx context.Context, ref string, opts ...StreamOption) (*BlobHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	options := &streamOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Verify policy if provided.
	if options.policy != nil {
		manifest, err := c.registry.FetchManifest(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("fetch manifest for policy: %w", err)
		}

		fetcher := &referrerFetcher{
			registry: c.registry,
			ref:      ref,
		}

		if err := options.policy.Verify(ctx, *manifest, fetcher); err != nil {
			return nil, fmt.Errorf("policy verification failed: %w", err)
		}
	}

	// Open blob for range requests.
	blobReader, err := c.registry.OpenBlob(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("open blob: %w", err)
	}

	// Create fetchFull callback for CopyTo optimization.
	fetchFull := func() (io.ReadCloser, error) {
		return c.registry.FetchBlob(ctx, ref)
	}

	// Create network-backed handle.
	handle, err := newNetworkHandle(ctx, blobReader, fetchFull)
	if err != nil {
		blobReader.Close()
		return nil, fmt.Errorf("create handle: %w", err)
	}

	c.logger.Debug("streaming blob",
		"ref", ref,
	)

	return handle, nil
}

// AttachArtifact attaches content to a manifest as a referrer.
//
// This is used to attach signatures, SBOMs, SLSA attestations, or other artifacts
// to a blob manifest. The artifact type identifies the kind of artifact.
//
// Returns the referrer digest for optional follow-up operations (e.g., signing).
func (c *Client) AttachArtifact(ctx context.Context, ref, artifactType string, content []byte, annotations map[string]string) (digest.Digest, error) {
	return c.registry.AttachArtifact(ctx, ref, artifactType, content, annotations)
}

// referrerFetcher implements ReferrerFetcher by delegating to the registry.
type referrerFetcher struct {
	registry registry.Registry
	ref      string
}

func (f *referrerFetcher) FetchReferrer(ctx context.Context, referrerDigest digest.Digest) ([]byte, error) {
	return f.registry.FetchReferrerContent(ctx, f.ref, referrerDigest)
}

// detectMediaType reads magic bytes from the file to determine the compression media type.
func detectMediaType(f *os.File) (string, error) {
	buf := make([]byte, 4)
	n, err := f.ReadAt(buf, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read magic bytes: %w", err)
	}

	// Check for gzip magic: 0x1f 0x8b.
	if n >= 2 && buf[0] == 0x1f && buf[1] == 0x8b {
		return "application/vnd.oci.image.layer.v1.tar+gzip", nil
	}

	// Check for zstd magic: 0x28 0xb5 0x2f 0xfd.
	if n >= 4 && buf[0] == 0x28 && buf[1] == 0xb5 && buf[2] == 0x2f && buf[3] == 0xfd {
		return "application/vnd.oci.image.layer.v1.tar+zstd", nil
	}

	return "", errors.New("unknown compression format")
}
