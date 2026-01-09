package blobber

import (
	"context"
	"fmt"
	"io"

	"github.com/opencontainers/go-digest"

	"github.com/meigma/blobber/internal/archive"
	"github.com/meigma/blobber/internal/progress"
)

// Pull downloads all files from the image to the destination directory.
// The ref must be fully qualified (e.g., "ghcr.io/org/repo:tag").
//
// If a cache is configured (via WithCacheDir), the blob will be fetched from cache
// if available, or downloaded and cached for future use.
//
// If a verifier is configured (via WithVerifier), the signature is verified before
// downloading the blob. Verification failure prevents the pull.
//
// The layer digest is verified while downloading for integrity.
func (c *Client) Pull(ctx context.Context, ref, destDir string, opts ...PullOption) error {
	// Verify signature if verifier configured
	if c.verifier != nil {
		verifiedRef, err := c.verifySignature(ctx, ref)
		if err != nil {
			return err
		}
		ref = verifiedRef
	}

	// Apply options
	cfg := &pullConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Use cache if available
	if c.cache != nil {
		return c.pullCached(ctx, ref, destDir, cfg)
	}

	// Resolve descriptor so we can verify the downloaded blob digest.
	desc, err := c.registry.ResolveLayer(ctx, ref)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", ref, err)
	}

	// Pull blob from registry directly
	blob, err := c.registry.FetchBlob(ctx, ref, desc)
	if err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}

	// Wrap blob for progress tracking if callback provided
	blobReader := wrapReaderForProgress(blob, desc.Size, cfg.progress)

	return c.extractWithDigest(ctx, ref, destDir, blobReader, desc, cfg.limits)
}

// pullCached pulls an image using the cache.
func (c *Client) pullCached(ctx context.Context, ref, destDir string, cfg *pullConfig) error {
	var desc LayerDescriptor
	var err error

	// Try TTL-based resolution first
	if c.cacheTTL > 0 {
		if cachedDesc, ok := c.cache.LookupByRef(ref, c.cacheTTL); ok {
			if c.hasCachedBlob(cachedDesc) {
				c.logger.Debug("using TTL-cached descriptor for pull", "ref", ref, "digest", cachedDesc.Digest)
				desc = cachedDesc
			}
		}
	}

	// If no valid TTL cache hit, resolve from registry
	if desc.Digest == "" {
		desc, err = c.registry.ResolveLayer(ctx, ref)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", ref, err)
		}
		// Update the reference index
		c.cache.UpdateRefIndex(ref, desc)
	}

	// Get blob stream from cache with streaming pass-through.
	// OpenStreamThrough streams from registry while concurrently caching,
	// preserving streaming extraction performance on cache miss.
	blob, err := c.cache.OpenStreamThrough(ctx, ref, desc)
	if err != nil {
		return fmt.Errorf("open cached blob %s: %w", ref, err)
	}

	// Wrap blob for progress tracking if callback provided
	blobReader := wrapReaderForProgress(blob, desc.Size, cfg.progress)

	return c.extractWithDigest(ctx, ref, destDir, blobReader, desc, cfg.limits)
}

func (c *Client) extractWithDigest(ctx context.Context, ref, destDir string, blob io.ReadCloser, desc LayerDescriptor, limits ExtractLimits) error {
	defer blob.Close()

	if desc.Digest == "" {
		return fmt.Errorf("missing blob digest for %s", ref)
	}

	digester := digest.SHA256.Digester()
	reader := io.TeeReader(blob, digester.Hash())
	if err := archive.Extract(ctx, reader, destDir, c.validator, limits); err != nil {
		return fmt.Errorf("extract %s: %w", ref, err)
	}

	computed := digester.Digest().String()
	if computed != desc.Digest {
		return fmt.Errorf("blob digest mismatch for %s: expected %s, got %s", ref, desc.Digest, computed)
	}

	return nil
}

// wrapReaderForProgress wraps an io.ReadCloser with progress tracking.
// If callback is nil, returns the original reader unchanged.
func wrapReaderForProgress(r io.ReadCloser, total int64, callback ProgressCallback) io.ReadCloser {
	if callback == nil {
		return r
	}
	return &progressReadCloser{
		Reader: progress.NewReader(r, total, func(transferred, totalBytes int64) {
			callback(ProgressEvent{
				Operation:        "pull",
				BytesTransferred: transferred,
				TotalBytes:       totalBytes,
			})
		}),
		closer: r,
	}
}

// progressReadCloser wraps a progress.Reader and delegates Close to the original reader.
type progressReadCloser struct {
	*progress.Reader
	closer io.Closer
}

func (p *progressReadCloser) Close() error {
	return p.closer.Close()
}
