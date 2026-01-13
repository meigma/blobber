package cache

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/opencontainers/go-digest"
)

// blobEntry is the JSON structure stored in cache.json for each blob.
type blobEntry struct {
	Digest       string    `json:"digest"`
	Size         int64     `json:"size"`
	LastAccessed time.Time `json:"lastAccessed"`
}

// fileCache implements FileCache with filesystem-backed storage.
type fileCache struct {
	path string
	mu   sync.RWMutex
}

// NewFileCache creates a FileCache backed by the given directory.
func NewFileCache(path string) (FileCache, error) {
	blobsDir := filepath.Join(path, "blobs")
	if err := os.MkdirAll(blobsDir, 0o700); err != nil {
		return nil, err
	}

	return &fileCache{
		path: blobsDir,
	}, nil
}

// Dir returns the cache directory path for a digest.
func (c *fileCache) Dir(d digest.Digest) (string, error) {
	dir := c.blobDir(d)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// IsComplete returns true if the digest has been fully cached.
func (c *fileCache) IsComplete(d digest.Digest) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entryPath := c.entryPath(d)
	_, err := os.Stat(entryPath)
	return err == nil
}

// MarkComplete records that all files for a digest have been written.
func (c *fileCache) MarkComplete(d digest.Digest, size int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := blobEntry{
		Digest:       d.String(),
		Size:         size,
		LastAccessed: time.Now(),
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	return atomicWrite(c.entryPath(d), data)
}

// Get returns a cached file if it exists.
func (c *fileCache) Get(d digest.Digest, path string) (fs.File, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check if blob is complete.
	if !c.isCompleteUnlocked(d) {
		return nil, fs.ErrNotExist
	}

	filePath := filepath.Join(c.blobDir(d), path)
	f, err := os.Open(filePath) //nolint:gosec // path is from caller, validated by os.Open
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fs.ErrNotExist
		}
		return nil, err
	}

	return f, nil
}

// WrapFile wraps an fs.File to cache its contents on full read.
//
// The wrapped file tees reads to a temp file. When the file is fully read
// (all bytes consumed) and the checksum matches, the temp file is atomically
// renamed to the cache location.
//
// If the file is closed before fully read, or the checksum doesn't match,
// the temp file is discarded silently.
func (c *fileCache) WrapFile(d digest.Digest, path string, f fs.File, size int64, checksum string) fs.File {
	return newCachedFile(f, c, d, path, size, checksum)
}

// TouchAccess updates the last access time for a digest.
func (c *fileCache) TouchAccess(d digest.Digest) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entryPath := c.entryPath(d)
	data, err := os.ReadFile(entryPath) //nolint:gosec // entryPath is derived from internal hash
	if err != nil {
		return err
	}

	var entry blobEntry
	if unmarshalErr := json.Unmarshal(data, &entry); unmarshalErr != nil {
		return unmarshalErr
	}

	entry.LastAccessed = time.Now()

	newData, marshalErr := json.MarshalIndent(entry, "", "  ")
	if marshalErr != nil {
		return marshalErr
	}

	return atomicWrite(entryPath, newData)
}

// blobDir returns the directory path for a blob's cache.
func (c *fileCache) blobDir(d digest.Digest) string {
	return filepath.Join(c.path, hashString(d.String()))
}

// entryPath returns the cache.json path for a blob.
func (c *fileCache) entryPath(d digest.Digest) string {
	return filepath.Join(c.blobDir(d), "cache.json")
}

// isCompleteUnlocked checks if a blob is complete without acquiring the lock.
// Caller must hold at least a read lock.
func (c *fileCache) isCompleteUnlocked(d digest.Digest) bool {
	entryPath := c.entryPath(d)
	_, err := os.Stat(entryPath)
	return err == nil
}
