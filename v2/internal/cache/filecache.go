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
	Complete     *bool     `json:"complete,omitempty"`
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

// LockBlob acquires an exclusive lock for cache mutations.
func (c *fileCache) LockBlob(d digest.Digest) (func() error, error) {
	return lockDir(c.blobDir(d))
}

// TryLockBlob attempts to acquire a lock without blocking.
func (c *fileCache) TryLockBlob(d digest.Digest) (func() error, bool, error) {
	return tryLockDir(c.blobDir(d))
}

// IsComplete returns true if the digest has been fully cached.
func (c *fileCache) IsComplete(d digest.Digest) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, err := c.loadEntry(d)
	if err != nil {
		return false
	}
	return isCompleteEntry(entry)
}

// MarkComplete records that all files for a digest have been written.
func (c *fileCache) MarkComplete(d digest.Digest, size int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, err := c.loadEntry(d)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		entry = blobEntry{Digest: d.String()}
	}

	if size > entry.Size {
		entry.Size = size
	}
	entry.Digest = d.String()
	entry.LastAccessed = time.Now()
	entry.Complete = boolPtr(true)

	return c.writeEntry(d, entry)
}

// Get returns a cached file if it exists.
func (c *fileCache) Get(d digest.Digest, path string) (fs.File, error) {
	if !fs.ValidPath(path) {
		return nil, fs.ErrInvalid
	}
	if path == "." {
		return nil, fs.ErrNotExist
	}

	root, err := os.OpenRoot(c.blobDir(d))
	if err != nil {
		return nil, mapNotExist(err)
	}
	defer root.Close()

	f, err := root.Open(filepath.FromSlash(path))
	if err != nil {
		return nil, mapNotExist(err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if info.IsDir() {
		f.Close()
		return nil, fs.ErrNotExist
	}

	if err := c.recordAccess(d, info.Size()); err != nil {
		// Best-effort access tracking.
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

	entry, err := c.loadEntry(d)
	if err != nil {
		return err
	}

	if entry.Digest == "" {
		entry.Digest = d.String()
	}
	entry.LastAccessed = time.Now()
	if entry.Complete == nil {
		entry.Complete = boolPtr(true)
	}

	return c.writeEntry(d, entry)
}

// blobDir returns the directory path for a blob's cache.
func (c *fileCache) blobDir(d digest.Digest) string {
	return filepath.Join(c.path, hashString(d.String()))
}

// entryPath returns the cache.json path for a blob.
func (c *fileCache) entryPath(d digest.Digest) string {
	return filepath.Join(c.blobDir(d), "cache.json")
}

func (c *fileCache) recordAccess(d digest.Digest, size int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, err := c.loadEntry(d)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		entry = blobEntry{
			Digest:       d.String(),
			Size:         size,
			LastAccessed: time.Now(),
			Complete:     boolPtr(false),
		}
		return c.writeEntry(d, entry)
	}

	entry.LastAccessed = time.Now()
	if entry.Complete == nil {
		entry.Complete = boolPtr(true)
	}
	return c.writeEntry(d, entry)
}

func (c *fileCache) recordCachedFile(d digest.Digest, size int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, err := c.loadEntry(d)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		entry = blobEntry{
			Digest:       d.String(),
			Size:         size,
			LastAccessed: time.Now(),
			Complete:     boolPtr(false),
		}
		return c.writeEntry(d, entry)
	}

	if entry.Digest == "" {
		entry.Digest = d.String()
	}
	if isCompleteEntry(entry) {
		entry.LastAccessed = time.Now()
		if entry.Complete == nil {
			entry.Complete = boolPtr(true)
		}
		return c.writeEntry(d, entry)
	}

	entry.Size += size
	entry.LastAccessed = time.Now()
	if entry.Complete == nil {
		entry.Complete = boolPtr(false)
	}

	return c.writeEntry(d, entry)
}

func (c *fileCache) loadEntry(d digest.Digest) (blobEntry, error) {
	data, err := os.ReadFile(c.entryPath(d)) //nolint:gosec // entryPath is derived from internal hash
	if err != nil {
		return blobEntry{}, err
	}

	var entry blobEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return blobEntry{}, err
	}

	return entry, nil
}

func (c *fileCache) writeEntry(d digest.Digest, entry blobEntry) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	return atomicWrite(c.entryPath(d), data)
}

func isCompleteEntry(entry blobEntry) bool {
	if entry.Complete == nil {
		return true
	}
	return *entry.Complete
}

func boolPtr(v bool) *bool {
	return &v
}

func mapNotExist(err error) error {
	if os.IsNotExist(err) {
		return fs.ErrNotExist
	}
	return err
}
