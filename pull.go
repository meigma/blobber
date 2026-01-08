package blobber

import (
	"context"
	"fmt"

	"github.com/meigma/blobber/internal/archive"
)

// Pull downloads all files from the image to the destination directory.
// The ref must be fully qualified (e.g., "ghcr.io/org/repo:tag").
//
// If a cache is configured (via WithCacheDir), the blob will be fetched from cache
// if available, or downloaded and cached for future use.
func (c *Client) Pull(ctx context.Context, ref, destDir string, opts ...PullOption) error {
	// Apply options
	cfg := &pullConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Use cache if available
	if c.cache != nil {
		return c.pullCached(ctx, ref, destDir, cfg)
	}

	// Pull blob from registry directly
	blob, _, err := c.registry.Pull(ctx, ref)
	if err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}
	defer blob.Close()

	// Extract to destination
	if err := archive.Extract(ctx, blob, destDir, c.validator, cfg.limits); err != nil {
		return fmt.Errorf("extract %s: %w", ref, err)
	}

	return nil
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
	defer blob.Close()

	// Extract to destination
	if err := archive.Extract(ctx, blob, destDir, c.validator, cfg.limits); err != nil {
		return fmt.Errorf("extract %s: %w", ref, err)
	}

	return nil
}
