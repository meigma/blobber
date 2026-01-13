package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/opencontainers/go-digest"
)

// refEntry is the JSON structure stored in cache.json for each ref.
type refEntry struct {
	Ref       string    `json:"ref"`
	Digest    string    `json:"digest"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// refCache implements RefCache with filesystem-backed storage.
type refCache struct {
	path string
	ttl  time.Duration
	mu   sync.RWMutex
}

// NewRefCache creates a RefCache backed by the given directory.
//
// The TTL controls how long cached ref→digest mappings are considered fresh.
// After TTL expires, Lookup returns ok=false, triggering a re-fetch.
func NewRefCache(path string, ttl time.Duration) (RefCache, error) {
	refsDir := filepath.Join(path, "refs")
	if err := os.MkdirAll(refsDir, 0o700); err != nil {
		return nil, err
	}

	return &refCache{
		path: refsDir,
		ttl:  ttl,
	}, nil
}

// Lookup returns the cached digest for a ref if it exists and is fresh.
func (c *refCache) Lookup(ref string) (digest.Digest, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entryPath := c.entryPath(ref)
	data, err := os.ReadFile(entryPath) //nolint:gosec // entryPath is derived from internal hash
	if err != nil {
		return "", false
	}

	var entry refEntry
	if unmarshalErr := json.Unmarshal(data, &entry); unmarshalErr != nil {
		return "", false
	}

	// Check if entry is stale.
	if time.Since(entry.UpdatedAt) > c.ttl {
		return "", false
	}

	d, err := digest.Parse(entry.Digest)
	if err != nil {
		return "", false
	}

	return d, true
}

// Store records a ref→digest mapping with the current timestamp.
func (c *refCache) Store(ref string, d digest.Digest) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := refEntry{
		Ref:       ref,
		Digest:    d.String(),
		UpdatedAt: time.Now(),
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return // Silently fail - caching is best-effort.
	}

	entryDir := c.entryDir(ref)
	if err := os.MkdirAll(entryDir, 0o700); err != nil {
		return
	}

	entryPath := c.entryPath(ref)
	if err := atomicWrite(entryPath, data); err != nil {
		return
	}
}

// entryDir returns the directory path for a ref's cache entry.
func (c *refCache) entryDir(ref string) string {
	return filepath.Join(c.path, hashString(ref))
}

// entryPath returns the cache.json path for a ref.
func (c *refCache) entryPath(ref string) string {
	return filepath.Join(c.entryDir(ref), "cache.json")
}

// hashString returns a filesystem-safe hash of the input string.
func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// atomicWrite writes data to path atomically using write-to-temp + rename.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".cache-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	// Clean up temp file on any error.
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	success = true
	return nil
}
