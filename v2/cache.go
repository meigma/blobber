package blobber

import (
	"context"
	"time"

	"github.com/meigma/blobber/v2/internal/cache"
)

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

// PruneFileCache removes cached blobs based on the given strategy.
//
// This function should be called explicitly by the consumer (e.g., on startup,
// via cron, or when disk space is low). Blobber does not prune automatically.
//
// The path should match the path used with WithFileCache when creating the client.
//
// Example:
//
//	err := blobber.PruneFileCache(ctx, "/var/cache/blobber", blobber.PruneStrategy{
//	    MaxSize: 10 * 1024 * 1024 * 1024, // 10 GB
//	    MaxAge:  7 * 24 * time.Hour,       // 7 days
//	})
func PruneFileCache(ctx context.Context, path string, strategy PruneStrategy) error {
	return cache.PruneFileCache(ctx, path, cache.PruneStrategy{
		MaxSize: strategy.MaxSize,
		MaxAge:  strategy.MaxAge,
	})
}
