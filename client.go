package blobber

import (
	"context"
	"fmt"
	"log/slog"

	"oras.land/oras-go/v2/registry/remote/credentials"

	"github.com/gilmanlab/blobber/internal/archive"
	"github.com/gilmanlab/blobber/internal/cache"
	"github.com/gilmanlab/blobber/internal/registry"
	"github.com/gilmanlab/blobber/internal/safepath"
)

// Client provides operations against OCI registries.
type Client struct {
	registry  Registry
	builder   ArchiveBuilder
	reader    ArchiveReader
	validator PathValidator
	logger    *slog.Logger

	// configuration passed to registry
	credStore credentials.Store
	plainHTTP bool
	userAgent string

	// cache configuration (opt-in)
	cacheDir           string
	cache              *cache.Cache
	backgroundPrefetch bool
	lazyLoading        bool
}

// NewClient creates a new blobber client.
//
// By default, credentials are resolved from Docker config (~/.docker/config.json)
// and credential helpers. Use WithCredentials or WithCredentialStore to override.
func NewClient(opts ...ClientOption) (*Client, error) {
	c := &Client{
		logger: slog.New(slog.DiscardHandler),
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	// Set up credential store if not provided
	if c.credStore == nil {
		store, err := registry.DefaultCredentialStore()
		if err != nil {
			return nil, fmt.Errorf("create credential store: %w", err)
		}
		c.credStore = store
	}

	// Wire up default implementations
	var regOpts []registry.Option
	if c.credStore != nil {
		regOpts = append(regOpts, registry.WithCredentialStore(c.credStore))
	}
	if c.plainHTTP {
		regOpts = append(regOpts, registry.WithPlainHTTP(true))
	}
	if c.userAgent != "" {
		regOpts = append(regOpts, registry.WithUserAgent(c.userAgent))
	}

	c.registry = registry.New(regOpts...)
	c.builder = archive.NewBuilder(c.logger)
	c.reader = archive.NewReader()
	c.validator = safepath.NewValidator()

	// Initialize cache if configured
	if c.cacheDir != "" {
		cacheInstance, err := cache.New(c.cacheDir, c.registry, c.logger)
		if err != nil {
			return nil, fmt.Errorf("create cache: %w", err)
		}
		c.cache = cacheInstance
	}

	return c, nil
}

// OpenImage opens a remote image for reading.
// The ref must be fully qualified (e.g., "ghcr.io/org/repo:tag").
//
// The returned Image caches the eStargz reader for efficient multiple file access.
// The caller must call Image.Close when done to release resources.
//
// If a cache is configured (via WithCacheDir), the blob will be fetched from cache
// if available, or downloaded and cached for future use.
func (c *Client) OpenImage(ctx context.Context, ref string) (*Image, error) {
	// Use cache if available
	if c.cache != nil {
		return c.openImageCached(ctx, ref)
	}

	// Pull the blob from registry directly
	blob, size, err := c.registry.Pull(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("pull %s: %w", ref, err)
	}
	defer blob.Close()

	// Create Image with cached reader
	img, err := NewImageFromBlob(ref, blob, size, c.validator, c.logger)
	if err != nil {
		return nil, fmt.Errorf("open image %s: %w", ref, err)
	}

	return img, nil
}

// openImageCached opens an image using the cache.
func (c *Client) openImageCached(ctx context.Context, ref string) (*Image, error) {
	// Resolve the layer descriptor first
	desc, err := c.registry.ResolveLayer(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", ref, err)
	}

	// Get blob handle from cache
	var handle BlobHandle
	if c.lazyLoading {
		// Lazy loading: fetch bytes on-demand via ReadAt
		handle, err = c.cache.OpenLazy(ctx, ref, desc)
	} else {
		// Eager loading: download entire blob upfront
		handle, err = c.cache.Open(ctx, ref, desc)
	}
	if err != nil {
		return nil, fmt.Errorf("open cached blob %s: %w", ref, err)
	}

	// Start background prefetch if enabled and blob is not complete
	if c.backgroundPrefetch && !handle.Complete() {
		c.cache.Prefetch(ctx, ref, desc)
	}

	// Create Image from the cached handle
	img, err := NewImageFromHandle(ref, handle, c.validator, c.logger)
	if err != nil {
		handle.Close()
		return nil, fmt.Errorf("open image %s: %w", ref, err)
	}

	return img, nil
}
