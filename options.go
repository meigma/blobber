package blobber

import (
	"log/slog"
	"time"

	"oras.land/oras-go/v2/registry/remote/credentials"

	"github.com/gilmanlab/blobber/core"
	"github.com/gilmanlab/blobber/internal/registry"
)

// ClientOption configures a Client.
type ClientOption func(*Client) error

// PushOption configures a Push operation.
type PushOption func(*pushConfig)

// PullOption configures a Pull operation.
type PullOption func(*pullConfig)

// ExtractLimits defines safety limits for extraction.
// Re-exported from core package.
type ExtractLimits = core.ExtractLimits

// pushConfig holds configuration for Push operations.
type pushConfig struct {
	annotations map[string]string
	mediaType   string
	compression Compression
}

// pullConfig holds configuration for Pull operations.
type pullConfig struct {
	limits ExtractLimits
}

// WithAnnotations sets OCI annotations on the pushed image.
func WithAnnotations(annotations map[string]string) PushOption {
	return func(c *pushConfig) {
		c.annotations = annotations
	}
}

// WithCompression sets the compression algorithm (gzip or zstd).
func WithCompression(c Compression) PushOption {
	return func(cfg *pushConfig) {
		cfg.compression = c
	}
}

// WithCredentials sets explicit credentials for a specific registry.
func WithCredentials(registryHost, username, password string) ClientOption {
	return func(c *Client) error {
		c.credStore = staticCredentials(registryHost, username, password)
		return nil
	}
}

// WithCredentialStore sets a custom credential store.
func WithCredentialStore(store credentials.Store) ClientOption {
	return func(c *Client) error {
		c.credStore = store
		return nil
	}
}

// WithExtractLimits sets safety limits for extraction.
func WithExtractLimits(limits ExtractLimits) PullOption {
	return func(c *pullConfig) {
		c.limits = limits
	}
}

// WithInsecure allows connections to registries without TLS.
func WithInsecure(insecure bool) ClientOption {
	return func(c *Client) error {
		c.plainHTTP = insecure
		return nil
	}
}

// WithLogger sets a logger for the client. By default, logging is disabled.
func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) error {
		c.logger = logger
		return nil
	}
}

// WithMediaType sets a custom media type for the layer.
func WithMediaType(mt string) PushOption {
	return func(c *pushConfig) {
		c.mediaType = mt
	}
}

// WithUserAgent sets a custom User-Agent header for registry requests.
func WithUserAgent(ua string) ClientOption {
	return func(c *Client) error {
		c.userAgent = ua
		return nil
	}
}

// WithDescriptorCache enables in-memory caching for layer descriptor resolution.
// This can return stale results for mutable tags; prefer digest references.
func WithDescriptorCache(enabled bool) ClientOption {
	return func(c *Client) error {
		c.descCache = enabled
		return nil
	}
}

// WithCacheDir enables blob caching at the specified directory path.
// When caching is enabled, blobs are stored locally after download and
// served from the cache on subsequent requests.
//
// The cache directory structure is:
//
//	<path>/blobs/sha256/<hash>     - cached blob files
//	<path>/entries/sha256/<hash>.json - cache metadata
//
// If the directory does not exist, it will be created.
// Caching is opt-in; if not specified, no caching is performed.
func WithCacheDir(path string) ClientOption {
	return func(c *Client) error {
		absPath, err := resolveCachePath(path)
		if err != nil {
			return err
		}
		c.cacheDir = absPath
		return nil
	}
}

// WithBackgroundPrefetch enables background prefetching of complete blobs
// when a partial cache hit occurs. This allows reading from the partial cache
// immediately while the remaining data is downloaded in the background.
//
// This option only has effect when caching is enabled (via WithCacheDir).
// The prefetch runs in a background goroutine and will stop if the context
// is canceled.
func WithBackgroundPrefetch(enabled bool) ClientOption {
	return func(c *Client) error {
		c.backgroundPrefetch = enabled
		return nil
	}
}

// WithLazyLoading enables on-demand fetching for OpenImage.
// When enabled, blobs are not downloaded upfront. Instead, byte ranges
// are fetched lazily as they are read (via io.ReaderAt calls).
//
// This is ideal for eStargz archives where only the TOC (table of contents)
// and specific file chunks need to be accessed, avoiding full blob downloads.
//
// The lazy loading workflow:
//  1. OpenImage resolves the layer descriptor (no download yet)
//  2. estargz reads the footer (last ~10KB) to parse the TOC
//  3. Only the requested file chunks are fetched when files are opened
//  4. Downloaded ranges are cached for future access
//
// This option only has effect when caching is enabled (via WithCacheDir).
// If the blob is already complete in cache, it is served directly.
func WithLazyLoading(enabled bool) ClientOption {
	return func(c *Client) error {
		c.lazyLoading = enabled
		return nil
	}
}

// WithCacheTTL sets the TTL for cache validation.
// When set, cached entries will be used without re-validating with the
// registry if they were validated within the TTL duration.
//
// A zero or negative TTL means always validate (current behavior).
// This option only has effect when caching is enabled (via WithCacheDir).
//
// WARNING: Using a TTL means you may get stale data if a tag is updated
// on the registry. For immutable references (digests), TTL is always safe.
// For mutable tags, choose a TTL that balances freshness vs. performance.
func WithCacheTTL(ttl time.Duration) ClientOption {
	return func(c *Client) error {
		c.cacheTTL = ttl
		return nil
	}
}

// staticCredentials returns a credential store with a single static credential.
func staticCredentials(registryHost, username, password string) credentials.Store {
	return registry.StaticCredentials(registryHost, username, password)
}
