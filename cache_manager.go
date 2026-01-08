package blobber

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/meigma/blobber/internal/cache"
)

// CacheInfo contains statistics about a cache directory.
type CacheInfo struct {
	// Path is the absolute path to the cache directory.
	Path string
	// TotalSize is the sum of all cached blob sizes in bytes.
	TotalSize int64
	// EntryCount is the number of cached blobs.
	EntryCount int
	// Entries contains detailed information about each cached blob.
	// Sorted by LastAccessed, most recent first.
	Entries []CacheEntry
}

// CacheEntry describes a single cached blob.
type CacheEntry struct {
	// Digest is the blob's SHA256 digest (sha256:...).
	Digest string
	// Size is the blob size in bytes.
	Size int64
	// LastAccessed is when the blob was last accessed.
	LastAccessed time.Time
	// Complete indicates whether the full blob is cached.
	Complete bool
}

// CachePruneOptions configures cache pruning behavior.
type CachePruneOptions struct {
	// MaxSize is the maximum total cache size in bytes.
	// Entries are evicted LRU until the cache is under this limit.
	// Zero means no size limit.
	MaxSize int64

	// MaxAge is the maximum age for cache entries.
	// Entries not accessed within this duration are evicted.
	// Zero means no age limit.
	MaxAge time.Duration
}

// CachePruneResult contains statistics about a prune operation.
type CachePruneResult struct {
	// EntriesRemoved is the number of entries that were evicted.
	EntriesRemoved int
	// BytesRemoved is the total bytes freed.
	BytesRemoved int64
	// EntriesRemaining is the number of entries still in cache.
	EntriesRemaining int
	// BytesRemaining is the total bytes still in cache.
	BytesRemaining int64
}

// CacheStats returns statistics about the cache at the given path.
// If the cache directory doesn't exist, returns an empty CacheInfo.
func CacheStats(path string) (*CacheInfo, error) {
	absPath, err := resolveCachePath(path)
	if err != nil {
		return nil, err
	}

	// Check if cache directory exists
	if _, statErr := os.Stat(absPath); os.IsNotExist(statErr) {
		return &CacheInfo{Path: absPath}, nil
	}

	c, err := cache.New(absPath, nil, slog.New(slog.DiscardHandler))
	if err != nil {
		return nil, fmt.Errorf("open cache: %w", err)
	}

	entries, err := c.Entries()
	if err != nil {
		return nil, fmt.Errorf("list entries: %w", err)
	}

	info := &CacheInfo{
		Path:       absPath,
		EntryCount: len(entries),
		Entries:    make([]CacheEntry, len(entries)),
	}

	for i, e := range entries {
		info.TotalSize += e.Size
		info.Entries[i] = CacheEntry{
			Digest:       e.Digest,
			Size:         e.Size,
			LastAccessed: e.LastAccessed,
			Complete:     e.Complete,
		}
	}

	return info, nil
}

// CacheClear removes all entries from the cache at the given path.
// Returns nil if the cache directory doesn't exist.
func CacheClear(path string) error {
	absPath, err := resolveCachePath(path)
	if err != nil {
		return err
	}

	// Check if cache directory exists
	if _, statErr := os.Stat(absPath); os.IsNotExist(statErr) {
		return nil
	}

	c, err := cache.New(absPath, nil, slog.New(slog.DiscardHandler))
	if err != nil {
		return fmt.Errorf("open cache: %w", err)
	}

	if err := c.Clear(); err != nil {
		return fmt.Errorf("clear cache: %w", err)
	}

	return nil
}

// CachePrune removes entries based on the provided options.
// Entries exceeding MaxAge are removed first, then LRU eviction
// is performed until TotalSize is under MaxSize.
// Returns nil if the cache directory doesn't exist.
func CachePrune(ctx context.Context, path string, opts CachePruneOptions) (*CachePruneResult, error) {
	absPath, err := resolveCachePath(path)
	if err != nil {
		return nil, err
	}

	// Check if cache directory exists
	if _, statErr := os.Stat(absPath); os.IsNotExist(statErr) {
		return &CachePruneResult{}, nil
	}

	c, err := cache.New(absPath, nil, slog.New(slog.DiscardHandler))
	if err != nil {
		return nil, fmt.Errorf("open cache: %w", err)
	}

	result, err := c.Prune(ctx, cache.PruneOptions{
		MaxSize: opts.MaxSize,
		MaxAge:  opts.MaxAge,
	})
	if err != nil {
		return nil, fmt.Errorf("prune cache: %w", err)
	}

	return &CachePruneResult{
		EntriesRemoved:   result.EntriesRemoved,
		BytesRemoved:     result.BytesRemoved,
		EntriesRemaining: result.EntriesRemaining,
		BytesRemaining:   result.BytesRemaining,
	}, nil
}

// resolveCachePath expands ~ and converts to absolute path.
func resolveCachePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("cache path is empty")
	}

	// Expand ~ to home directory
	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home directory: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	return absPath, nil
}
