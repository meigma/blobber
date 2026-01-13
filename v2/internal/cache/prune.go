package cache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// cacheEntry holds info loaded from a blob's cache.json for pruning decisions.
type cacheEntry struct {
	dir          string
	digest       string
	size         int64
	lastAccessed time.Time
}

// PruneFileCache removes cached blobs based on the given strategy.
//
// The pruning logic:
//  1. Loads all cache.json files from blobs/
//  2. If MaxAge > 0: marks entries older than cutoff for removal
//  3. If MaxSize > 0: sorts remaining by lastAccessed, removes LRU until under size
//  4. Deletes marked directories
//
// Returns an error if the cache directory cannot be read. Individual
// deletion failures are logged but don't stop the pruning process.
func PruneFileCache(ctx context.Context, path string, strategy PruneStrategy) error {
	blobsDir := filepath.Join(path, "blobs")

	entries, err := loadCacheEntries(blobsDir)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	toRemove := selectEntriesForRemoval(entries, strategy)
	return removeEntries(ctx, toRemove)
}

// selectEntriesForRemoval determines which entries should be pruned based on strategy.
func selectEntriesForRemoval(entries []cacheEntry, strategy PruneStrategy) map[string]bool {
	toRemove := make(map[string]bool)

	// Mark entries older than MaxAge.
	if strategy.MaxAge > 0 {
		markStaleEntries(entries, strategy.MaxAge, toRemove)
	}

	// Mark LRU entries if over MaxSize.
	if strategy.MaxSize > 0 {
		markLRUEntries(entries, strategy.MaxSize, toRemove)
	}

	return toRemove
}

// markStaleEntries marks entries older than maxAge for removal.
func markStaleEntries(entries []cacheEntry, maxAge time.Duration, toRemove map[string]bool) {
	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		if e.lastAccessed.Before(cutoff) {
			toRemove[e.dir] = true
		}
	}
}

// markLRUEntries marks least-recently-used entries until total size is under maxSize.
func markLRUEntries(entries []cacheEntry, maxSize int64, toRemove map[string]bool) {
	// Filter out already-marked entries and calculate remaining size.
	var remaining []cacheEntry
	var totalSize int64
	for _, e := range entries {
		if !toRemove[e.dir] {
			remaining = append(remaining, e)
			totalSize += e.size
		}
	}

	if totalSize <= maxSize {
		return
	}

	// Sort by lastAccessed (oldest first).
	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].lastAccessed.Before(remaining[j].lastAccessed)
	})

	// Mark oldest entries until under limit.
	for _, e := range remaining {
		if totalSize <= maxSize {
			break
		}
		toRemove[e.dir] = true
		totalSize -= e.size
	}
}

// removeEntries deletes the directories marked for removal.
func removeEntries(ctx context.Context, toRemove map[string]bool) error {
	for dir := range toRemove {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		// Errors are ignored - partial cleanup is acceptable.
		os.RemoveAll(dir)
	}
	return nil
}

// loadCacheEntries reads all cache.json files from the blobs directory.
func loadCacheEntries(blobsDir string) ([]cacheEntry, error) {
	dirs, err := os.ReadDir(blobsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	entries := make([]cacheEntry, 0, len(dirs))
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}

		blobDir := filepath.Join(blobsDir, d.Name())
		cachePath := filepath.Join(blobDir, "cache.json")

		data, err := os.ReadFile(cachePath) //nolint:gosec // path is from internal directory listing
		if err != nil {
			// Skip directories without cache.json (untracked entries).
			continue
		}

		var entry blobEntry
		if unmarshalErr := json.Unmarshal(data, &entry); unmarshalErr != nil {
			// Skip malformed entries.
			continue
		}

		entries = append(entries, cacheEntry{
			dir:          blobDir,
			digest:       entry.Digest,
			size:         entry.Size,
			lastAccessed: entry.LastAccessed,
		})
	}

	return entries, nil
}
