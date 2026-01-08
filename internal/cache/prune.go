package cache

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// PruneOptions configures cache pruning behavior.
type PruneOptions struct {
	// MaxSize is the maximum total cache size in bytes.
	// Entries are evicted LRU until the cache is under this limit.
	// Zero means no size limit.
	MaxSize int64

	// MaxAge is the maximum age for cache entries.
	// Entries older than this (based on LastAccessed) are evicted.
	// Zero means no age limit.
	MaxAge time.Duration
}

// PruneResult contains statistics about a prune operation.
type PruneResult struct {
	// EntriesRemoved is the number of entries that were evicted.
	EntriesRemoved int
	// BytesRemoved is the total bytes freed.
	BytesRemoved int64
	// EntriesRemaining is the number of entries still in cache.
	EntriesRemaining int
	// BytesRemaining is the total bytes still in cache.
	BytesRemaining int64
}

// Prune removes cache entries based on the provided options.
// Entries are evicted based on TTL first, then LRU until size limits are met.
// Returns statistics about the pruning operation.
func (c *Cache) Prune(ctx context.Context, opts PruneOptions) (PruneResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := PruneResult{}

	entries, err := c.loadAllEntries()
	if err != nil {
		return result, err
	}
	if len(entries) == 0 {
		return result, nil
	}

	toRemove := c.selectEntriesToRemove(entries, opts)

	result, validDigests, err := c.executeRemovals(ctx, entries, toRemove)
	if err != nil {
		return result, err
	}

	// Clean up orphaned refs after pruning
	c.cleanupOrphanedRefs(validDigests)

	c.logger.Debug("cache pruned",
		"removed", result.EntriesRemoved,
		"bytes_removed", result.BytesRemoved,
		"remaining", result.EntriesRemaining,
		"bytes_remaining", result.BytesRemaining)

	return result, nil
}

// selectEntriesToRemove determines which entries should be evicted based on TTL and size limits.
func (c *Cache) selectEntriesToRemove(entries []*Entry, opts PruneOptions) map[string]bool {
	toRemove := make(map[string]bool)

	// Phase 1: Mark entries exceeding TTL
	if opts.MaxAge > 0 {
		cutoff := time.Now().Add(-opts.MaxAge)
		for _, e := range entries {
			if e.LastAccessed.Before(cutoff) {
				toRemove[e.Digest] = true
			}
		}
	}

	// Phase 2: Mark LRU entries to meet size limit
	if opts.MaxSize > 0 {
		c.markLRUEntries(entries, toRemove, opts.MaxSize)
	}

	return toRemove
}

// markLRUEntries marks oldest entries for removal until size is under limit.
func (c *Cache) markLRUEntries(entries []*Entry, toRemove map[string]bool, maxSize int64) {
	// Build list of remaining entries and calculate total size
	remaining := make([]*Entry, 0, len(entries))
	var totalSize int64
	for _, e := range entries {
		if !toRemove[e.Digest] {
			remaining = append(remaining, e)
			totalSize += e.Size
		}
	}

	if totalSize <= maxSize {
		return
	}

	// Sort by LastAccessed (oldest first)
	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].LastAccessed.Before(remaining[j].LastAccessed)
	})

	// Mark oldest entries until under limit
	for _, e := range remaining {
		if totalSize <= maxSize {
			break
		}
		toRemove[e.Digest] = true
		totalSize -= e.Size
	}
}

// executeRemovals removes marked entries and returns statistics and the set of remaining digests.
func (c *Cache) executeRemovals(ctx context.Context, entries []*Entry, toRemove map[string]bool) (PruneResult, map[string]bool, error) {
	var result PruneResult
	validDigests := make(map[string]bool)
	for _, e := range entries {
		if ctx.Err() != nil {
			return result, validDigests, ctx.Err()
		}
		if toRemove[e.Digest] {
			if err := c.evictLocked(e.Digest); err != nil {
				c.logger.Warn("failed to evict entry", "digest", e.Digest, "error", err)
				continue
			}
			result.EntriesRemoved++
			result.BytesRemoved += e.Size
		} else {
			result.EntriesRemaining++
			result.BytesRemaining += e.Size
			validDigests[e.Digest] = true
		}
	}
	return result, validDigests, nil
}

// Size returns the total size of all cached blobs in bytes.
func (c *Cache) Size() (int64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries, err := c.loadAllEntries()
	if err != nil {
		return 0, err
	}

	var total int64
	for _, e := range entries {
		total += e.Size
	}
	return total, nil
}

// Entries returns metadata for all cache entries.
// The returned slice is sorted by LastAccessed (most recent first).
func (c *Cache) Entries() ([]*Entry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries, err := c.loadAllEntries()
	if err != nil {
		return nil, err
	}

	// Sort by LastAccessed, most recent first
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].LastAccessed.After(entries[j].LastAccessed)
	})

	return entries, nil
}

// loadAllEntries loads all entry metadata files from disk.
// Caller must hold at least c.mu.RLock().
func (c *Cache) loadAllEntries() ([]*Entry, error) {
	entriesDir := filepath.Join(c.path, "entries", "sha256")
	files, err := os.ReadDir(entriesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	entries := make([]*Entry, 0, len(files))
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		if filepath.Ext(name) != jsonExt {
			continue
		}

		entryPath := filepath.Join(entriesDir, name)
		entry, loadErr := loadEntry(entryPath)
		if loadErr != nil {
			c.logger.Debug("failed to load entry", "path", entryPath, "error", loadErr)
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// evictLocked removes a blob from the cache.
// Caller must hold c.mu.Lock().
func (c *Cache) evictLocked(digest string) error {
	blobPath := c.blobPath(digest)
	entryPath := c.entryPath(digest)
	partialPath := blobPath + ".partial"

	// Remove all files, ignoring "not exists" errors
	for _, path := range []string{blobPath, partialPath, entryPath} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	// Clean up any refs pointing to this digest
	c.removeRefsByDigest(digest)

	return nil
}

// removeRefsByDigest removes all reference entries that point to the given digest.
// This is called when a blob is evicted to prevent stale ref→digest mappings.
func (c *Cache) removeRefsByDigest(digest string) {
	refsDir := filepath.Join(c.path, "refs")
	files, err := os.ReadDir(refsDir)
	if err != nil {
		return // refs dir may not exist
	}

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != jsonExt {
			continue
		}

		refPath := filepath.Join(refsDir, f.Name())
		entry, err := loadRefEntry(refPath)
		if err != nil {
			continue
		}

		if entry.Digest == digest {
			if err := os.Remove(refPath); err != nil && !os.IsNotExist(err) {
				c.logger.Debug("failed to remove ref entry", "path", refPath, "error", err)
			}
		}
	}
}

// cleanupOrphanedRefs removes ref entries that point to digests not in the valid set.
// This is called after pruning to ensure ref index stays consistent with blob cache.
func (c *Cache) cleanupOrphanedRefs(validDigests map[string]bool) {
	refsDir := filepath.Join(c.path, "refs")
	files, err := os.ReadDir(refsDir)
	if err != nil {
		return // refs dir may not exist
	}

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != jsonExt {
			continue
		}

		refPath := filepath.Join(refsDir, f.Name())
		entry, err := loadRefEntry(refPath)
		if err != nil {
			// Remove corrupt/unreadable entries
			os.Remove(refPath)
			continue
		}

		if !validDigests[entry.Digest] {
			if err := os.Remove(refPath); err != nil && !os.IsNotExist(err) {
				c.logger.Debug("failed to remove orphaned ref", "ref", entry.Ref, "error", err)
			} else {
				c.logger.Debug("removed orphaned ref", "ref", entry.Ref, "digest", entry.Digest)
			}
		}
	}
}
