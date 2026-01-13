package blobber

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/opencontainers/go-digest"

	"github.com/meigma/blobber/v2/internal/cache"
	"github.com/meigma/blobber/v2/internal/domain"
	"github.com/meigma/blobber/v2/internal/estargz"
	"github.com/meigma/blobber/v2/internal/oci"
	"github.com/meigma/blobber/v2/internal/registry"
)

// Client provides operations for pushing and pulling blobs to OCI registries.
type Client struct {
	registry  registry.Registry
	logger    *slog.Logger
	refCache  cache.RefCache
	fileCache cache.FileCache
	manCache  cache.ManifestCache
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

	// Initialize caches if configured.
	var refCache cache.RefCache
	var fileCache cache.FileCache
	var manCache cache.ManifestCache

	if options.refCachePath != "" {
		var err error
		refCache, err = cache.NewRefCache(options.refCachePath, options.refCacheTTL)
		if err != nil {
			logger.Warn("failed to create ref cache, caching disabled",
				"path", options.refCachePath,
				"error", err,
			)
		}
	}

	if options.fileCachePath != "" {
		var err error
		fileCache, err = cache.NewFileCache(options.fileCachePath)
		if err != nil {
			logger.Warn("failed to create file cache, caching disabled",
				"path", options.fileCachePath,
				"error", err,
			)
		}
	}

	if options.manifestCache && options.fileCachePath != "" {
		var err error
		manCache, err = cache.NewManifestCache(options.fileCachePath)
		if err != nil {
			logger.Warn("failed to create manifest cache, caching disabled",
				"path", options.fileCachePath,
				"error", err,
			)
		}
	}
	if options.manifestCache && options.fileCachePath == "" {
		logger.Warn("manifest cache requested without file cache path, caching disabled")
	}

	// Use injected registry if provided (for testing).
	if options.registry != nil {
		if r, ok := options.registry.(registry.Registry); ok {
			return &Client{
				registry:  r,
				logger:    logger,
				refCache:  refCache,
				fileCache: fileCache,
				manCache:  manCache,
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
	if options.rangeHook != nil {
		ociOpts = append(ociOpts, oci.WithRangeHook(options.rangeHook))
	}

	ociClient := oci.NewClient(ociOpts...)

	// Create registry with the OCI client.
	var regOpts []registry.Option
	if options.logger != nil {
		regOpts = append(regOpts, registry.WithLogger(options.logger))
	}
	if manCache != nil {
		regOpts = append(regOpts, registry.WithManifestCache(manCache))
	}

	reg := registry.New(ociClient, regOpts...)

	return &Client{
		registry:  reg,
		logger:    logger,
		refCache:  refCache,
		fileCache: fileCache,
		manCache:  manCache,
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

// resolveRef resolves a ref to its blob digest, using RefCache if available.
func (c *Client) resolveRef(ctx context.Context, ref string) (digest.Digest, error) {
	// Check RefCache first.
	if c.refCache != nil {
		if d, ok := c.refCache.Lookup(ref); ok {
			c.logger.Debug("ref cache hit", "ref", ref, "digest", d)
			return d, nil
		}
	}

	// Cache miss or no cache: resolve via network.
	d, err := c.registry.ResolveRef(ctx, ref)
	if err != nil {
		return "", err
	}

	// Store in cache.
	if c.refCache != nil {
		c.refCache.Store(ref, d)
		c.logger.Debug("ref cache stored", "ref", ref, "digest", d)
	}

	return d, nil
}

// Pull downloads a blob and returns a BlobHandle for accessing its contents.
//
// If a file cache is configured and the blob has been previously pulled,
// the cached files are served directly without network access.
//
// Otherwise, the blob is downloaded and extracted to the cache (if configured)
// or a temp file (if no cache).
//
// Call Close() when done to release resources.
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

	// Verify policy if provided - requires full manifest with referrers.
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

	// Populate RefCache if configured
	// This allows subsequent Pull/Stream calls to benefit from cached ref resolution.
	if c.refCache != nil && c.fileCache == nil {
		if _, err := c.resolveRef(ctx, ref); err != nil {
			return nil, fmt.Errorf("resolve ref: %w", err)
		}
	}

	// Check file cache for hit using resolveRef (benefits from RefCache).
	if c.fileCache != nil {
		blobDigest, resolveErr := c.resolveRef(ctx, ref)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve ref: %w", resolveErr)
		}

		if c.fileCache.IsComplete(blobDigest) {
			// Cache hit - serve from cache directory.
			cacheDir, dirErr := c.fileCache.Dir(blobDigest)
			if dirErr != nil {
				return nil, fmt.Errorf("get cache dir: %w", dirErr)
			}

			// Touch access time for LRU tracking.
			if touchErr := c.fileCache.TouchAccess(blobDigest); touchErr != nil {
				c.logger.Warn("failed to touch cache access time",
					"digest", blobDigest,
					"error", touchErr,
				)
			}

			c.logger.Debug("file cache hit",
				"ref", ref,
				"digest", blobDigest,
			)

			return newDirHandle(ctx, cacheDir)
		}

		// Cache miss - download, extract, and cache.
		// We need the size for FetchBlobByDigest, so fetch manifest.
		manifest, manifestErr := c.registry.FetchManifest(ctx, ref)
		if manifestErr != nil {
			return nil, fmt.Errorf("fetch manifest: %w", manifestErr)
		}

		return c.pullToCache(ctx, ref, blobDigest, manifest.Blob.Size)
	}

	// No cache configured - use temp file.
	return c.pullToTemp(ctx, ref)
}

// pullToCache downloads a blob, extracts it to the cache, and returns a dir handle.
func (c *Client) pullToCache(ctx context.Context, ref string, blobDigest digest.Digest, blobSize int64) (*BlobHandle, error) {
	// Get cache directory.
	cacheDir, err := c.fileCache.Dir(blobDigest)
	if err != nil {
		return nil, fmt.Errorf("get cache dir: %w", err)
	}

	// Download blob to temp file using known digest (avoids redundant manifest fetch).
	rc, err := c.registry.FetchBlobByDigest(ctx, ref, blobDigest, blobSize)
	if err != nil {
		return nil, fmt.Errorf("fetch blob: %w", err)
	}
	defer rc.Close()

	tempFile, err := os.CreateTemp("", "blobber-*.blob")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, copyErr := io.Copy(tempFile, rc); copyErr != nil {
		tempFile.Close()
		return nil, fmt.Errorf("write temp file: %w", copyErr)
	}

	if closeErr := tempFile.Close(); closeErr != nil {
		return nil, fmt.Errorf("close temp file: %w", closeErr)
	}

	// Open as estargz and extract to cache.
	localHandle, err := newLocalHandle(ctx, tempPath)
	if err != nil {
		return nil, fmt.Errorf("create handle: %w", err)
	}
	defer localHandle.Close()

	extractedSize, err := extractToDir(ctx, localHandle, cacheDir)
	if err != nil {
		return nil, fmt.Errorf("extract to cache: %w", err)
	}

	// Mark cache as complete.
	if err := c.fileCache.MarkComplete(blobDigest, extractedSize); err != nil {
		c.logger.Warn("failed to mark cache complete",
			"digest", blobDigest,
			"error", err,
		)
	}

	c.logger.Debug("cached blob",
		"ref", ref,
		"digest", blobDigest,
		"cache_dir", cacheDir,
	)

	return newDirHandle(ctx, cacheDir)
}

// pullToTemp downloads a blob to a temp file and returns a local handle.
func (c *Client) pullToTemp(ctx context.Context, ref string) (*BlobHandle, error) {
	rc, err := c.registry.FetchBlob(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("fetch blob: %w", err)
	}
	defer rc.Close()

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

	handle, err := newLocalHandle(ctx, tempPath)
	if err != nil {
		os.Remove(tempPath)
		return nil, fmt.Errorf("create handle: %w", err)
	}

	c.logger.Debug("pulled blob to temp",
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

	// Fetch manifest to get blob digest and size.
	// Note: Unlike Pull(), we always need the manifest for size, so RefCache
	// doesn't help avoid this fetch.
	manifest, err := c.registry.FetchManifest(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}

	blobDigest := manifest.Blob.Digest
	blobSize := manifest.Blob.Size

	// Store in RefCache for future Pull() calls.
	if c.refCache != nil {
		c.refCache.Store(ref, blobDigest)
	}

	// Verify policy if provided.
	if options.policy != nil {
		fetcher := &referrerFetcher{
			registry: c.registry,
			ref:      ref,
		}

		if verifyErr := options.policy.Verify(ctx, *manifest, fetcher); verifyErr != nil {
			return nil, fmt.Errorf("policy verification failed: %w", verifyErr)
		}
	}

	// Open blob for range requests using known digest.
	blobReader, err := c.registry.OpenBlobByDigest(ctx, ref, blobDigest, blobSize)
	if err != nil {
		return nil, fmt.Errorf("open blob: %w", err)
	}

	if c.fileCache != nil {
		tocCache := cache.NewTOCCache(c.fileCache)
		blobReader = estargz.WrapWithTOCCache(blobReader, blobDigest, tocCache)
	}

	// Create fetchFull callback for CopyTo optimization.
	fetchFull := func() (io.ReadCloser, error) {
		return c.registry.FetchBlobByDigest(ctx, ref, blobDigest, blobSize)
	}

	// Create network-backed handle.
	handle, err := newNetworkHandle(ctx, blobReader, fetchFull)
	if err != nil {
		blobReader.Close()
		return nil, fmt.Errorf("create handle: %w", err)
	}

	// Inject file cache for on-demand file caching.
	if c.fileCache != nil {
		handle.fileCache = c.fileCache
		handle.blobDigest = blobDigest
	}

	c.logger.Debug("streaming blob",
		"ref", ref,
		"digest", blobDigest,
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

// extractToDir extracts all files from an fs.FS to a directory and returns total bytes written.
func extractToDir(ctx context.Context, src fs.FS, dst string) (int64, error) {
	var total int64

	err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Check context cancellation.
		if err := ctx.Err(); err != nil {
			return err
		}

		dstPath := filepath.Join(dst, path)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o700)
		}

		// Create parent directory if needed.
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
			return fmt.Errorf("create parent dir for %s: %w", path, err)
		}

		// Copy file contents.
		srcFile, err := src.Open(path)
		if err != nil {
			return fmt.Errorf("open source %s: %w", path, err)
		}
		defer srcFile.Close()

		dstFile, err := os.Create(dstPath) //nolint:gosec // dstPath is derived from cache dir
		if err != nil {
			return fmt.Errorf("create dest %s: %w", path, err)
		}
		defer dstFile.Close()

		n, err := io.Copy(dstFile, srcFile)
		if err != nil {
			return fmt.Errorf("copy %s: %w", path, err)
		}
		total += n

		return nil
	})
	if err != nil {
		return 0, err
	}

	return total, nil
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
