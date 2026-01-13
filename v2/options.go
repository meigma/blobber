package blobber

import (
	"log/slog"
	"time"

	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/containerd/stargz-snapshotter/estargz/zstdchunked"
	"github.com/klauspost/compress/zstd"
	"oras.land/oras-go/v2/registry/remote/credentials"
)

// Compressor is a compression algorithm for eStargz archives.
type Compressor = estargz.Compressor

// GzipCompression returns a gzip compressor.
// This is the default compression if none is specified.
func GzipCompression() Compressor {
	return estargz.NewGzipCompressor()
}

// ZstdCompression returns a zstd compressor.
// Zstd offers better compression ratios and faster decompression than gzip.
func ZstdCompression() Compressor {
	return &zstdchunked.Compressor{
		CompressionLevel: zstd.SpeedDefault,
	}
}

// ClientOption configures a Client.
type ClientOption func(*clientOptions)

type clientOptions struct {
	credStore     credentials.Store
	plainHTTP     bool
	userAgent     string
	logger        *slog.Logger
	registry      interface{} // For testing: accepts registry.Registry
	refCachePath  string
	refCacheTTL   time.Duration
	fileCachePath string
}

// WithCredentialStore sets the credential store for registry authentication.
// If not set, requests are made without authentication.
func WithCredentialStore(store credentials.Store) ClientOption {
	return func(o *clientOptions) {
		o.credStore = store
	}
}

// WithPlainHTTP enables insecure HTTP connections instead of HTTPS.
// This should only be used for local development or testing.
func WithPlainHTTP(plainHTTP bool) ClientOption {
	return func(o *clientOptions) {
		o.plainHTTP = plainHTTP
	}
}

// WithUserAgent sets the User-Agent header for HTTP requests.
func WithUserAgent(ua string) ClientOption {
	return func(o *clientOptions) {
		o.userAgent = ua
	}
}

// WithLogger sets the logger for debug output.
func WithLogger(logger *slog.Logger) ClientOption {
	return func(o *clientOptions) {
		o.logger = logger
	}
}

// WithRefCache enables caching of ref→digest mappings.
//
// The TTL controls how long cached mappings are considered fresh. After the TTL
// expires, the ref is re-resolved from the registry. This avoids repeated manifest
// fetches for the same ref within the TTL window.
//
// The cache is stored at the given path. Multiple clients can share the same cache
// path safely.
func WithRefCache(path string, ttl time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.refCachePath = path
		o.refCacheTTL = ttl
	}
}

// WithFileCache enables caching of extracted files.
//
// When enabled, Pull() extracts files directly to the cache and serves from there.
// Subsequent Pull() calls for the same digest return immediately from cache.
//
// For Stream(), individual files are cached on-demand when fully read.
//
// The cache is stored at the given path. Use [PruneFileCache] to manage cache size.
func WithFileCache(path string) ClientOption {
	return func(o *clientOptions) {
		o.fileCachePath = path
	}
}

// PushOption configures a Push operation.
type PushOption func(*pushOptions)

type pushOptions struct {
	compression Compressor
}

// WithCompression sets the compression algorithm for the Push operation.
// If not specified, gzip compression is used.
func WithCompression(c Compressor) PushOption {
	return func(o *pushOptions) {
		o.compression = c
	}
}

// PullOption configures a Pull operation.
type PullOption func(*pullOptions)

type pullOptions struct {
	policy Policy
}

// WithPullPolicy sets a policy to verify the manifest before downloading.
// If the policy returns an error, the Pull operation is aborted.
func WithPullPolicy(p Policy) PullOption {
	return func(o *pullOptions) {
		o.policy = p
	}
}

// StreamOption configures a Stream operation.
type StreamOption func(*streamOptions)

type streamOptions struct {
	policy Policy
}

// WithStreamPolicy sets a policy to verify the manifest before streaming.
// If the policy returns an error, the Stream operation is aborted.
func WithStreamPolicy(p Policy) StreamOption {
	return func(o *streamOptions) {
		o.policy = p
	}
}
