package cache

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/opencontainers/go-digest"
)

// manifestCache implements ManifestCache with filesystem-backed storage.
type manifestCache struct {
	path string
	mu   sync.RWMutex
}

// NewManifestCache creates a ManifestCache backed by the given directory.
func NewManifestCache(path string) (ManifestCache, error) {
	manifestsDir := filepath.Join(path, "manifests")
	if err := os.MkdirAll(manifestsDir, 0o700); err != nil {
		return nil, err
	}

	return &manifestCache{
		path: manifestsDir,
	}, nil
}

// Load returns cached manifest bytes for a digest.
func (c *manifestCache) Load(d digest.Digest) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, err := os.ReadFile(c.entryPath(d)) //nolint:gosec // path is derived from internal hash
	if err != nil {
		return nil, false
	}

	if len(data) == 0 {
		return nil, false
	}

	return data, true
}

// Store writes manifest bytes for a digest.
func (c *manifestCache) Store(d digest.Digest, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entryPath := c.entryPath(d)
	if err := os.MkdirAll(filepath.Dir(entryPath), 0o700); err != nil {
		return err
	}

	return atomicWrite(entryPath, raw)
}

// entryPath returns the manifest path for a digest.
func (c *manifestCache) entryPath(d digest.Digest) string {
	return filepath.Join(c.path, hashString(d.String()), "manifest.json")
}
