package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/opencontainers/go-digest"
)

// cachedFile wraps an fs.File to cache its contents on full read.
//
// It tees reads to a temp file and promotes to cache on EOF when:
// - All bytes have been read (read == expected)
// - Checksum matches the expected value
//
// If the file is closed before fully read, or checksum mismatches,
// the temp file is discarded.
type cachedFile struct {
	inner    fs.File
	temp     *os.File
	cache    *fileCache
	blobDgst digest.Digest
	path     string
	expected int64
	checksum string // Expected checksum from TOC (sha256:hex format)
	read     int64
	logger   *slog.Logger
	promoted bool
	mu       sync.Mutex
}

// newCachedFile creates a cached file wrapper.
//
// If temp file creation fails, returns the inner file unwrapped (graceful degradation).
func newCachedFile(inner fs.File, cache *fileCache, blobDgst digest.Digest, path string, size int64, checksum string) fs.File {
	logger := slog.New(slog.DiscardHandler)

	// Create temp file in the cache directory for atomic rename.
	cacheDir := cache.blobDir(blobDgst)
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		logger.Warn("failed to create cache dir, skipping caching",
			"path", path,
			"error", err,
		)
		return inner
	}

	temp, err := os.CreateTemp(cacheDir, ".cache-*.tmp")
	if err != nil {
		logger.Warn("failed to create temp file, skipping caching",
			"path", path,
			"error", err,
		)
		return inner
	}

	return &cachedFile{
		inner:    inner,
		temp:     temp,
		cache:    cache,
		blobDgst: blobDgst,
		path:     path,
		expected: size,
		checksum: checksum,
		logger:   logger,
	}
}

// Read implements io.Reader, teeing to temp file.
func (f *cachedFile) Read(p []byte) (int, error) {
	n, err := f.inner.Read(p)

	if n > 0 {
		f.mu.Lock()
		// Write to temp file (best effort - don't fail the read).
		if f.temp != nil {
			if _, writeErr := f.temp.Write(p[:n]); writeErr != nil {
				f.logger.Warn("failed to write to temp file, disabling caching",
					"path", f.path,
					"error", writeErr,
				)
				f.temp.Close()
				os.Remove(f.temp.Name())
				f.temp = nil
			}
		}
		f.read += int64(n)
		f.mu.Unlock()
	}

	// On EOF, check if we should promote to cache.
	if err == io.EOF {
		f.tryPromote()
	}

	return n, err
}

// Stat implements fs.File.
func (f *cachedFile) Stat() (fs.FileInfo, error) {
	return f.inner.Stat()
}

// Close implements fs.File.
func (f *cachedFile) Close() error {
	innerErr := f.inner.Close()

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.temp == nil {
		return innerErr
	}

	// If not promoted (partial read), clean up temp file.
	if !f.promoted {
		f.temp.Close()
		os.Remove(f.temp.Name())
		f.temp = nil
	}

	return innerErr
}

// tryPromote attempts to promote the temp file to cache.
func (f *cachedFile) tryPromote() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.temp == nil || f.promoted {
		return
	}

	// Only promote if we read the expected number of bytes.
	if f.read != f.expected {
		f.logger.Debug("partial read, not caching",
			"path", f.path,
			"read", f.read,
			"expected", f.expected,
		)
		return
	}

	// Sync to ensure all data is written.
	if err := f.temp.Sync(); err != nil {
		f.logger.Warn("failed to sync temp file",
			"path", f.path,
			"error", err,
		)
		return
	}

	// Verify checksum if provided.
	if f.checksum != "" {
		actual, err := computeFileChecksum(f.temp)
		if err != nil {
			f.logger.Warn("failed to compute checksum",
				"path", f.path,
				"error", err,
			)
			return
		}

		if actual != f.checksum {
			f.logger.Warn("checksum mismatch, discarding cached file",
				"path", f.path,
				"expected", f.checksum,
				"actual", actual,
			)
			f.temp.Close()
			os.Remove(f.temp.Name())
			f.temp = nil
			return
		}
	}

	unlock, ok, err := f.cache.TryLockBlob(f.blobDgst)
	if err != nil {
		f.logger.Warn("failed to lock cache dir, skipping caching",
			"path", f.path,
			"error", err,
		)
		return
	}
	if !ok {
		f.logger.Debug("cache lock busy, skipping caching",
			"path", f.path,
		)
		return
	}
	defer func() {
		if unlockErr := unlock(); unlockErr != nil {
			f.logger.Warn("failed to unlock cache dir",
				"path", f.path,
				"error", unlockErr,
			)
		}
	}()

	// Atomic rename to final location.
	destPath := filepath.Join(f.cache.blobDir(f.blobDgst), f.path)

	// Create parent directories if needed.
	if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
		f.logger.Warn("failed to create parent dir for cached file",
			"path", f.path,
			"error", err,
		)
		return
	}

	tempPath := f.temp.Name()
	f.temp.Close()

	if err := os.Rename(tempPath, destPath); err != nil {
		f.logger.Warn("failed to rename temp to cache",
			"path", f.path,
			"error", err,
		)
		os.Remove(tempPath)
		return
	}

	f.promoted = true
	f.logger.Debug("cached file",
		"path", f.path,
		"size", f.read,
	)

	if err := f.cache.recordCachedFile(f.blobDgst, f.read); err != nil {
		f.logger.Warn("failed to update cache metadata",
			"path", f.path,
			"error", err,
		)
	}
}

// computeFileChecksum computes SHA256 checksum in OCI digest format.
func computeFileChecksum(f *os.File) (string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek to start: %w", err)
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("compute hash: %w", err)
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
