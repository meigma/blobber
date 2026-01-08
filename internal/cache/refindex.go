package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gilmanlab/blobber/core"
)

// RefEntry maps a reference to its resolved digest with validation metadata.
type RefEntry struct {
	// Ref is the original OCI reference (e.g., "ghcr.io/org/repo:v1.0").
	Ref string `json:"ref"`
	// Digest is the resolved layer digest (sha256:...).
	Digest string `json:"digest"`
	// Size is the layer size in bytes.
	Size int64 `json:"size"`
	// MediaType is the layer media type.
	MediaType string `json:"media_type"`
	// ValidatedAt is when this ref→digest mapping was last confirmed.
	ValidatedAt time.Time `json:"validated_at"`
}

// refPath returns the path for a reference index entry.
// Uses SHA256 hash of the reference to avoid filesystem issues with special chars.
func (c *Cache) refPath(ref string) string {
	hash := sha256.Sum256([]byte(ref))
	hashStr := hex.EncodeToString(hash[:])
	return filepath.Join(c.path, "refs", hashStr+jsonExt)
}

// loadRefEntry loads a reference index entry from disk.
func loadRefEntry(path string) (*RefEntry, error) {
	//nolint:gosec // G304: path is derived from ref hash, not user input
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var entry RefEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("unmarshal ref entry: %w", err)
	}

	return &entry, nil
}

// saveRefEntry writes a reference index entry to disk atomically.
// Uses write-to-temp + rename + fsync for durability.
func saveRefEntry(path string, entry *RefEntry) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create refs directory: %w", err)
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ref entry: %w", err)
	}

	// Write to temp file
	tmpPath := path + ".tmp"
	//nolint:gosec // G304: tmpPath is derived from ref hash, not user input
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write ref entry: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync ref entry: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close ref entry: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename ref entry: %w", err)
	}

	return nil
}

// LookupByRef checks if a reference has a valid cached digest within the TTL.
// Returns the LayerDescriptor and true if the cached mapping is valid,
// or zero value and false if not found or expired.
//
// This method does NOT validate that the blob itself is cached - only that
// we have a recent ref→digest mapping. The caller should still check the
// blob cache using the returned descriptor.
func (c *Cache) LookupByRef(ref string, ttl time.Duration) (core.LayerDescriptor, bool) {
	if ttl <= 0 {
		return core.LayerDescriptor{}, false
	}

	refPath := c.refPath(ref)
	refEntry, err := loadRefEntry(refPath)
	if err != nil {
		return core.LayerDescriptor{}, false
	}

	// Check if mapping is still valid
	if time.Since(refEntry.ValidatedAt) > ttl {
		c.logger.Debug("ref cache expired", "ref", ref, "validated_at", refEntry.ValidatedAt)
		return core.LayerDescriptor{}, false
	}

	c.logger.Debug("ref cache hit", "ref", ref, "digest", refEntry.Digest)
	return core.LayerDescriptor{
		Digest:    refEntry.Digest,
		Size:      refEntry.Size,
		MediaType: refEntry.MediaType,
	}, true
}

// UpdateRefIndex updates the reference index with a validated ref→digest mapping.
// This should be called after successfully resolving a reference from the registry.
func (c *Cache) UpdateRefIndex(ref string, desc core.LayerDescriptor) {
	refPath := c.refPath(ref)
	entry := &RefEntry{
		Ref:         ref,
		Digest:      desc.Digest,
		Size:        desc.Size,
		MediaType:   desc.MediaType,
		ValidatedAt: time.Now(),
	}
	if err := saveRefEntry(refPath, entry); err != nil {
		c.logger.Debug("failed to update ref index", "ref", ref, "error", err)
	}
}
