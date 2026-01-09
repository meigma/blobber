package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TagListEntry caches the tag list for a repository with validation metadata.
type TagListEntry struct {
	// Repository is the OCI repository (e.g., "ghcr.io/org/repo").
	Repository string `json:"repository"`
	// Tags is the list of tags in the repository.
	Tags []string `json:"tags"`
	// ValidatedAt is when this tag list was last fetched.
	ValidatedAt time.Time `json:"validated_at"`
}

// tagListPath returns the path for a tag list cache entry.
// Uses SHA256 hash of the repository to avoid filesystem issues with special chars.
func (c *Cache) tagListPath(repository string) string {
	hash := sha256.Sum256([]byte(repository))
	hashStr := hex.EncodeToString(hash[:])
	return filepath.Join(c.path, "tags", hashStr+jsonExt)
}

// loadTagList loads a tag list entry from disk.
func loadTagList(path string) (*TagListEntry, error) {
	if err := ensureCacheFile(path); err != nil {
		return nil, err
	}
	//nolint:gosec // G304: path is derived from repository hash, not user input
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var entry TagListEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("unmarshal tag list entry: %w", err)
	}

	return &entry, nil
}

// saveTagList writes a tag list entry to disk atomically.
// Uses write-to-temp + rename + fsync for durability.
func saveTagList(path string, entry *TagListEntry) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create tags directory: %w", err)
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tag list entry: %w", err)
	}

	// Write to temp file
	tmpPath := path + ".tmp"
	//nolint:gosec // G304: tmpPath is derived from repository hash, not user input
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write tag list entry: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync tag list entry: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close tag list entry: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename tag list entry: %w", err)
	}

	return nil
}

// ListTags returns the tags for a repository, using cache if available and valid.
// The ttl parameter controls how long cached tag lists are considered valid.
// If ttl is 0 or negative, the cache is bypassed and tags are fetched from the registry.
func (c *Cache) ListTags(ctx context.Context, repository string, ttl time.Duration) ([]string, error) {
	// Check cache first if TTL is positive
	if ttl > 0 {
		tagPath := c.tagListPath(repository)
		entry, err := loadTagList(tagPath)
		if err == nil && time.Since(entry.ValidatedAt) <= ttl {
			c.logger.Debug("tag list cache hit", "repository", repository, "tags", len(entry.Tags))
			return entry.Tags, nil
		}
		if err == nil {
			c.logger.Debug("tag list cache expired", "repository", repository, "validated_at", entry.ValidatedAt)
		}
	}

	// Cache miss or expired - fetch from registry
	c.logger.Debug("tag list cache miss", "repository", repository)
	tags, err := c.fallback.ListTags(ctx, repository)
	if err != nil {
		return nil, err
	}

	// Update cache
	if ttl > 0 {
		tagPath := c.tagListPath(repository)
		entry := &TagListEntry{
			Repository:  repository,
			Tags:        tags,
			ValidatedAt: time.Now(),
		}
		if saveErr := saveTagList(tagPath, entry); saveErr != nil {
			c.logger.Debug("failed to save tag list cache", "repository", repository, "error", saveErr)
		}
	}

	return tags, nil
}
