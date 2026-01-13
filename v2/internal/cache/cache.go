// Package cache provides caching for OCI registry operations.
//
// The cache package provides two independent caching mechanisms:
//
//   - RefCache: Caches ref→digest mappings with TTL-based freshness
//   - FileCache: Caches extracted files by digest (immutable, no TTL)
//
// These caches can be used independently or together. RefCache reduces
// manifest fetch network calls, while FileCache enables serving file
// content from local disk.
//
// The cache uses a simple on-disk format with JSON metadata files.
// All writes use atomic operations (write-to-temp + rename) for durability.
package cache

//go:generate go run github.com/matryer/moq@latest -out mocks/refcache.go -pkg mocks . RefCache
//go:generate go run github.com/matryer/moq@latest -out mocks/filecache.go -pkg mocks . FileCache

import (
	"io/fs"
	"time"

	"github.com/opencontainers/go-digest"
)

// RefCache caches ref→digest mappings with TTL-based freshness.
//
// Refs (like "ghcr.io/org/repo:v1") are mutable - tags can move to different
// digests. The TTL controls how long a cached mapping is considered fresh
// before requiring re-validation from the registry.
type RefCache interface {
	// Lookup returns the cached digest for a ref if it exists and is fresh.
	// Returns ok=false if the ref is not cached or the cached entry is stale.
	Lookup(ref string) (d digest.Digest, ok bool)

	// Store records a ref→digest mapping with the current timestamp.
	// If an entry already exists, it is overwritten.
	Store(ref string, d digest.Digest)
}

// FileCache caches extracted files by blob digest.
//
// Files are stored under their blob's digest, preserving directory structure.
// Since content is addressed by digest, cached files are immutable and don't
// require TTL-based freshness checks.
//
// The cache supports two usage patterns:
//   - Pull: Extract all files to cache directory, serve via Dir()
//   - Stream: Cache individual files on-demand via Get()/WrapFile()
type FileCache interface {
	// Dir returns the cache directory path for a digest.
	// Creates the directory structure if it doesn't exist.
	// Used by Pull() to extract files directly to the cache.
	Dir(d digest.Digest) (string, error)

	// IsComplete returns true if the digest has been fully cached.
	// A digest is complete after MarkComplete() has been called.
	IsComplete(d digest.Digest) bool

	// MarkComplete records that all files for a digest have been written.
	// This creates the cache metadata file with size and access time.
	// Should be called after Pull() extraction completes.
	MarkComplete(d digest.Digest, size int64) error

	// Get returns a cached file if it exists.
	// Returns fs.ErrNotExist if the file is not cached.
	// Cached files may exist even when the blob isn't complete.
	// Updates the last access time for LRU tracking.
	Get(d digest.Digest, path string) (fs.File, error)

	// WrapFile wraps an fs.File to cache its contents on full read.
	// The returned file tees reads to a temp file. When EOF is reached
	// and all expected bytes have been read, the checksum is verified
	// and the file is promoted to the cache.
	//
	// If the file is closed before EOF or checksum verification fails,
	// the temp file is discarded and no caching occurs.
	//
	// Used by Stream() for on-demand file caching.
	WrapFile(d digest.Digest, path string, f fs.File, size int64, checksum string) fs.File

	// TouchAccess updates the last access time for a digest.
	// Called on cache hits to maintain accurate LRU ordering.
	TouchAccess(d digest.Digest) error
}

// PruneStrategy configures cache eviction behavior.
//
// Both MaxSize and MaxAge can be set together. When combined:
//  1. Entries older than MaxAge are removed first
//  2. If still over MaxSize, LRU eviction removes oldest entries until under limit
type PruneStrategy struct {
	// MaxSize evicts LRU entries until total cache size is under this limit.
	// A value of 0 means no size limit.
	MaxSize int64

	// MaxAge evicts entries not accessed within this duration.
	// A value of 0 means no age limit.
	MaxAge time.Duration
}
