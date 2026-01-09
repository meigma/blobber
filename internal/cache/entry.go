package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Range represents a contiguous byte range within a blob.
type Range struct {
	// Offset is the starting byte position.
	Offset int64 `json:"offset"`
	// Length is the number of bytes in this range.
	Length int64 `json:"length"`
}

// End returns the exclusive end byte position (Offset + Length).
func (r Range) End() int64 {
	return r.Offset + r.Length
}

// Entry is the metadata stored per-digest at entries/sha256/<digest>.json.
type Entry struct {
	// Version is the metadata format version.
	Version int `json:"version"`
	// Digest is the blob digest (sha256:...).
	Digest string `json:"digest"`
	// Size is the total blob size in bytes.
	Size int64 `json:"size"`
	// MediaType is the OCI media type.
	MediaType string `json:"media_type"`
	// Complete indicates the full download is complete.
	Complete bool `json:"complete"`
	// Verified indicates the digest was verified after download.
	Verified bool `json:"verified"`
	// Ranges contains the byte ranges that have been downloaded.
	// Only populated for partial downloads (Complete=false).
	// Ranges are non-overlapping and sorted by offset.
	Ranges []Range `json:"ranges,omitempty"`
	// CreatedAt is when the entry was first created.
	CreatedAt time.Time `json:"created_at"`
	// LastAccessed is when the blob was last accessed.
	LastAccessed time.Time `json:"last_accessed"`
	// Ref is the OCI reference used to fetch this blob.
	// Used for resuming partial downloads with range requests.
	Ref string `json:"ref,omitempty"`
}

// loadEntry reads a cache entry from disk.
func loadEntry(path string) (*Entry, error) {
	if err := ensureCacheFile(path); err != nil {
		return nil, err
	}
	//nolint:gosec // G304: path is derived from digest hash, not user input
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("unmarshal entry: %w", err)
	}

	return &entry, nil
}

// saveEntry writes a cache entry to disk atomically.
// Uses write-to-temp + rename + fsync for durability.
func saveEntry(path string, entry *Entry) error {
	// Set timestamps if not already set
	now := time.Now()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.LastAccessed = now

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}

	// Write to temp file
	tmpPath := path + ".tmp"
	//nolint:gosec // G304: tmpPath is derived from digest hash, not user input
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write entry: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync entry: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close entry: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename entry: %w", err)
	}

	return nil
}
