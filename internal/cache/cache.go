// Package cache provides disk-based caching for OCI blobs.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/meigma/blobber/core"
)

// jsonExt is the file extension for JSON metadata files.
const jsonExt = ".json"

// Cache implements core.BlobSource with disk-based caching.
// It stores complete blobs keyed by their SHA256 digest.
type Cache struct {
	path         string
	fallback     core.Registry
	logger       *slog.Logger
	verifyOnRead bool

	mu sync.RWMutex
}

// New creates a new cache at the given path.
// The fallback registry is used to fetch blobs not in the cache.
func New(path string, fallback core.Registry, logger *slog.Logger) (*Cache, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	// Create cache directory structure
	dirs := []string{
		filepath.Join(path, "blobs", "sha256"),
		filepath.Join(path, "entries", "sha256"),
		filepath.Join(path, "refs"),
		filepath.Join(path, "tags"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create cache directory %s: %w", dir, err)
		}
	}

	return &Cache{
		path:     path,
		fallback: fallback,
		logger:   logger,
	}, nil
}

// SetVerifyOnRead controls whether cache hits are re-verified by digest.
func (c *Cache) SetVerifyOnRead(enabled bool) {
	c.verifyOnRead = enabled
}

// Open returns a BlobHandle for random access to the blob.
// If the blob is cached, returns a handle to the cached file.
// Otherwise, downloads the blob to the cache first.
// The ref parameter is used for fetching missing ranges during resume.
//
// If the cached blob file is missing or corrupt, the entry is evicted
// and the blob is re-downloaded (self-healing).
func (c *Cache) Open(ctx context.Context, ref string, desc core.LayerDescriptor) (core.BlobHandle, error) {
	entry, blobPath, entryPath := c.LoadCompleteEntry(desc.Digest)
	if entry != nil {
		c.logger.Debug("cache hit", "digest", desc.Digest)
		handle, openErr := c.openCachedBlob(blobPath, entry)
		if openErr == nil {
			c.touchEntry(entryPath, entry)
			return handle, nil
		}
		c.selfHealEvict(desc.Digest, openErr)
	}

	// Cache miss - download and cache the blob
	c.logger.Debug("cache miss", "digest", desc.Digest)
	if downloadErr := c.downloadBlob(ctx, ref, desc, blobPath, entryPath); downloadErr != nil {
		return nil, fmt.Errorf("download blob: %w", downloadErr)
	}

	// Load the entry we just created
	entry, err := loadEntry(entryPath)
	if err != nil {
		return nil, fmt.Errorf("load entry after download: %w", err)
	}

	return c.openCachedBlob(blobPath, entry)
}

// OpenStream returns a streaming reader for the blob.
// For cached blobs, opens the file directly.
// For uncached blobs, downloads to cache first, then streams from the cached file.
// The ref parameter is used for fetching missing ranges during resume.
//
// If the cached blob file is missing or corrupt, the entry is evicted
// and the blob is re-downloaded (self-healing).
//
// Note: For cache misses, this blocks until the full download completes.
// Use OpenStreamThrough for true streaming with concurrent cache population.
func (c *Cache) OpenStream(ctx context.Context, ref string, desc core.LayerDescriptor) (io.ReadCloser, error) {
	entry, blobPath, entryPath := c.LoadCompleteEntry(desc.Digest)
	if entry != nil {
		c.logger.Debug("cache hit (stream)", "digest", desc.Digest)
		f, openErr := c.openCachedBlobFile(blobPath, entry)
		if openErr == nil {
			c.touchEntry(entryPath, entry)
			return f, nil
		}
		c.selfHealEvict(desc.Digest, openErr)
	}

	// Cache miss - download and cache the blob, then return file reader
	c.logger.Debug("cache miss (stream)", "digest", desc.Digest)
	if downloadErr := c.downloadBlob(ctx, ref, desc, blobPath, entryPath); downloadErr != nil {
		return nil, fmt.Errorf("download blob: %w", downloadErr)
	}

	// Open newly downloaded blob (skip size validation - just downloaded)
	//nolint:gosec // G304: blobPath is derived from digest, not user input
	f, err := os.Open(blobPath)
	if err != nil {
		return nil, fmt.Errorf("open cached blob: %w", err)
	}
	return f, nil
}

// OpenStreamThrough returns a streaming reader that writes to cache as it reads.
// Unlike OpenStream, this doesn't block on cache miss - it streams directly from
// the registry while concurrently populating the cache.
//
// For cache hits, returns the cached file directly (same as OpenStream).
// For cache misses, returns a tee reader that writes to cache as data flows through.
//
// This preserves streaming extraction performance when caching is enabled.
func (c *Cache) OpenStreamThrough(ctx context.Context, ref string, desc core.LayerDescriptor) (io.ReadCloser, error) {
	entry, blobPath, entryPath := c.LoadCompleteEntry(desc.Digest)
	if entry != nil {
		c.logger.Debug("cache hit (stream-through)", "digest", desc.Digest)
		f, openErr := c.openCachedBlobFile(blobPath, entry)
		if openErr == nil {
			c.touchEntry(entryPath, entry)
			return f, nil
		}
		c.selfHealEvict(desc.Digest, openErr)
	}

	// Cache miss - stream from registry while writing to cache
	c.logger.Debug("cache miss (stream-through)", "digest", desc.Digest)
	return c.streamThrough(ctx, ref, desc, blobPath, entryPath)
}

// streamThrough creates a tee reader that streams from registry while caching.
func (c *Cache) streamThrough(ctx context.Context, ref string, desc core.LayerDescriptor, blobPath, entryPath string) (io.ReadCloser, error) {
	// Clean up any stale .partial files and associated entry from previous
	// incomplete lazy loads. Must remove both to prevent later lazy reads
	// from treating stale entry ranges as cached (returning zeroed data).
	partialPath := blobPath + ".partial"
	if _, err := os.Stat(partialPath); err == nil {
		os.Remove(partialPath)
		os.Remove(entryPath) // Remove entry to invalidate any stale ranges
	}

	// Fetch blob from registry
	reader, err := c.fallback.FetchBlob(ctx, ref, desc)
	if err != nil {
		return nil, fmt.Errorf("fetch blob: %w", err)
	}

	// Create temp file for caching
	tmpPath := blobPath + ".tmp"
	//nolint:gosec // G304: tmpPath is derived from blobPath which is derived from digest
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		reader.Close()
		return nil, fmt.Errorf("create temp file: %w", err)
	}

	return &cachingReader{
		cache:     c,
		reader:    reader,
		file:      f,
		tmpPath:   tmpPath,
		blobPath:  blobPath,
		entryPath: entryPath,
		desc:      desc,
		ref:       ref,
		hasher:    sha256.New(), // Initialize upfront for zero-length blob verification
	}, nil
}

// cachingReader wraps a reader and writes to cache as data flows through.
type cachingReader struct {
	cache     *Cache
	reader    io.ReadCloser
	file      *os.File
	tmpPath   string
	blobPath  string
	entryPath string
	desc      core.LayerDescriptor
	ref       string
	written   int64
	hasher    hash.Hash // Initialized upfront for zero-length blob verification
	closed    bool
	err       error // sticky error from writes
}

func (cr *cachingReader) Read(p []byte) (n int, err error) {
	if cr.err != nil {
		// If we had a write error, still try to read from source
		return cr.reader.Read(p)
	}

	n, err = cr.reader.Read(p)
	if n > 0 {
		// Write to cache file
		written, writeErr := cr.file.Write(p[:n])
		if writeErr != nil {
			cr.err = writeErr
			cr.cache.logger.Debug("cache write error, continuing without caching", "error", writeErr)
		} else {
			cr.written += int64(written)
			//nolint:errcheck // hash.Write never returns an error per hash.Hash contract
			cr.hasher.Write(p[:n])
		}
	}
	return n, err
}

func (cr *cachingReader) Close() error {
	if cr.closed {
		return nil
	}
	cr.closed = true

	if cr.err == nil && cr.written < cr.desc.Size {
		cr.cache.logger.Debug("stream-through draining remainder",
			"expected", cr.desc.Size,
			"written", cr.written)
		if drainErr := cr.drainToCache(); drainErr != nil {
			cr.err = drainErr
		}
	}

	readerErr := cr.reader.Close()

	// If we had errors or incomplete data, clean up
	if cr.err != nil || cr.written != cr.desc.Size {
		cr.cache.logger.Debug("stream-through incomplete, not caching",
			"expected", cr.desc.Size,
			"written", cr.written,
			"write_error", cr.err,
			"read_error", readerErr)
		cr.file.Close()
		os.Remove(cr.tmpPath)
		return readerErr
	}

	// Verify digest (always runs, including for zero-length blobs)
	computedHash := "sha256:" + hex.EncodeToString(cr.hasher.Sum(nil))
	if computedHash != cr.desc.Digest {
		cr.file.Close()
		os.Remove(cr.tmpPath)
		cr.cache.logger.Debug("stream-through digest mismatch, not caching",
			"expected", cr.desc.Digest, "got", computedHash)
		return readerErr
	}

	// Finalize cache entry
	if err := cr.file.Sync(); err != nil {
		cr.file.Close()
		os.Remove(cr.tmpPath)
		return readerErr
	}
	cr.file.Close()

	// Atomic rename
	if err := os.Rename(cr.tmpPath, cr.blobPath); err != nil {
		os.Remove(cr.tmpPath)
		cr.cache.logger.Debug("failed to rename cached blob", "error", err)
		return readerErr
	}

	// Create entry metadata
	newEntry := &Entry{
		Version:   1,
		Digest:    cr.desc.Digest,
		Size:      cr.desc.Size,
		MediaType: cr.desc.MediaType,
		Complete:  true,
		Verified:  true,
		Ref:       cr.ref,
	}
	if err := saveEntry(cr.entryPath, newEntry); err != nil {
		cr.cache.logger.Warn("failed to save cache entry after stream-through", "error", err)
	}

	cr.cache.logger.Debug("stream-through cached blob", "digest", cr.desc.Digest, "size", cr.written)
	return readerErr
}

func (cr *cachingReader) drainToCache() error {
	remaining := cr.desc.Size - cr.written
	if remaining <= 0 {
		return nil
	}

	buf := make([]byte, 128*1024)
	lr := &io.LimitedReader{R: cr.reader, N: remaining}
	for lr.N > 0 {
		n, err := lr.Read(buf)
		if n > 0 {
			written, writeErr := cr.file.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
			cr.written += int64(written)
			//nolint:errcheck // hash.Write never returns an error per hash.Hash contract
			cr.hasher.Write(buf[:written])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
	}
	if lr.N > 0 {
		return io.ErrUnexpectedEOF
	}
	return nil
}

// Evict removes a blob from the cache, including any partial download.
func (c *Cache) Evict(digest string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.evictLocked(digest)
}

// touchEntry updates the LastAccessed time for a cache entry.
// This is called on cache hits to maintain accurate LRU ordering.
func (c *Cache) touchEntry(entryPath string, entry *Entry) {
	entry.LastAccessed = time.Now()
	if err := saveEntry(entryPath, entry); err != nil {
		c.logger.Debug("failed to touch entry", "error", err)
	}
}

// Clear removes all cached blobs and reference index entries.
func (c *Cache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove and recreate the cache directories
	dirs := []string{
		filepath.Join(c.path, "blobs", "sha256"),
		filepath.Join(c.path, "entries", "sha256"),
		filepath.Join(c.path, "refs"),
		filepath.Join(c.path, "tags"),
	}

	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove %s: %w", dir, err)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("recreate %s: %w", dir, err)
		}
	}

	return nil
}

// blobPath returns the path for a cached blob file.
func (c *Cache) blobPath(digest string) string {
	hashStr := extractHash(digest)
	return filepath.Join(c.path, "blobs", "sha256", hashStr)
}

// entryPath returns the path for a cache entry metadata file.
func (c *Cache) entryPath(digest string) string {
	hashStr := extractHash(digest)
	return filepath.Join(c.path, "entries", "sha256", hashStr+jsonExt)
}

// getPaths returns the blob and entry paths for a digest.
func (c *Cache) getPaths(digest string) (blobPath, entryPath string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.blobPath(digest), c.entryPath(digest)
}

// LoadCompleteEntry loads an entry if it's complete and verified.
// Returns (entry, blobPath, entryPath) where entry is nil for cache misses.
// This is exported for TTL validation to check if a cached descriptor's blob exists.
func (c *Cache) LoadCompleteEntry(digest string) (entry *Entry, blobPath, entryPath string) {
	blobPath, entryPath = c.getPaths(digest)
	var err error
	entry, err = loadEntry(entryPath)
	if err == nil && entry.Complete && entry.Verified {
		return entry, blobPath, entryPath
	}
	return nil, blobPath, entryPath
}

// selfHealEvict handles corrupt cache entries by evicting them.
func (c *Cache) selfHealEvict(digest string, err error) {
	c.logger.Debug("cache hit but blob missing/corrupt, self-healing", "digest", digest, "error", err)
	if evictErr := c.Evict(digest); evictErr != nil {
		c.logger.Debug("failed to evict corrupt entry", "error", evictErr)
	}
}

// openCachedBlob opens a cached blob file as a BlobHandle.
// Returns an error if the file is missing or has unexpected size (truncated/expanded).
func (c *Cache) openCachedBlob(blobPath string, entry *Entry) (core.BlobHandle, error) {
	if err := ensureCacheFile(blobPath); err != nil {
		return nil, fmt.Errorf("open cached blob: %w", err)
	}
	//nolint:gosec // G304: blobPath is derived from digest, not user input
	f, err := os.Open(blobPath)
	if err != nil {
		return nil, fmt.Errorf("open cached blob: %w", err)
	}

	// Validate file size matches entry to detect truncated/expanded blobs
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat cached blob: %w", err)
	}
	if info.Size() != entry.Size {
		f.Close()
		return nil, fmt.Errorf("cached blob size mismatch: expected %d, got %d", entry.Size, info.Size())
	}

	if c.verifyOnRead {
		if verifyErr := c.verifyFileDigest(f, entry.Digest); verifyErr != nil {
			f.Close()
			return nil, verifyErr
		}
	}

	return &fileHandle{
		file:     f,
		size:     entry.Size,
		complete: entry.Complete,
	}, nil
}

// openCachedBlobFile opens a cached blob file for streaming with size validation.
// Returns an error if the file is missing or has unexpected size (truncated/expanded).
// Unlike openCachedBlob, this returns the raw *os.File for streaming reads.
func (c *Cache) openCachedBlobFile(blobPath string, entry *Entry) (*os.File, error) {
	if err := ensureCacheFile(blobPath); err != nil {
		return nil, fmt.Errorf("open cached blob: %w", err)
	}
	//nolint:gosec // G304: blobPath is derived from digest, not user input
	f, err := os.Open(blobPath)
	if err != nil {
		return nil, fmt.Errorf("open cached blob: %w", err)
	}

	// Validate file size matches entry to detect truncated/expanded blobs
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat cached blob: %w", err)
	}
	if info.Size() != entry.Size {
		f.Close()
		return nil, fmt.Errorf("cached blob size mismatch: expected %d, got %d", entry.Size, info.Size())
	}

	if c.verifyOnRead {
		if verifyErr := c.verifyFileDigest(f, entry.Digest); verifyErr != nil {
			f.Close()
			return nil, verifyErr
		}
	}

	return f, nil
}

func (c *Cache) verifyFileDigest(f *os.File, expected string) error {
	if expected == "" {
		return errors.New("missing expected digest")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek cached blob: %w", err)
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("hash cached blob: %w", err)
	}

	computed := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if computed != expected {
		return fmt.Errorf("cached blob digest mismatch: expected %s, got %s", expected, computed)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek cached blob: %w", err)
	}

	return nil
}

// downloadBlob downloads a blob from the registry and caches it.
// If a partial download exists, it will attempt to resume.
func (c *Cache) downloadBlob(ctx context.Context, ref string, desc core.LayerDescriptor, blobPath, entryPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring lock
	entry, err := loadEntry(entryPath)
	if err == nil && entry.Complete && entry.Verified {
		return nil // Another goroutine completed the download
	}

	// Check for existing partial download
	partialPath := blobPath + ".partial"
	if entry != nil && !entry.Complete && len(entry.Ranges) > 0 {
		// Try to resume partial download
		if resumeErr := c.resumeDownload(ctx, ref, desc, partialPath, entryPath, entry); resumeErr != nil {
			c.logger.Debug("resume failed, starting fresh", "error", resumeErr)
			// Fall through to full download
		} else {
			return nil // Resume successful
		}
	}

	// Full download
	return c.fullDownload(ctx, ref, desc, blobPath, entryPath)
}

// fullDownload downloads the entire blob fresh.
func (c *Cache) fullDownload(ctx context.Context, ref string, desc core.LayerDescriptor, blobPath, entryPath string) error {
	// Remove any existing partial file
	partialPath := blobPath + ".partial"
	os.Remove(partialPath)

	// Fetch blob from registry
	reader, err := c.fallback.FetchBlob(ctx, ref, desc)
	if err != nil {
		return fmt.Errorf("fetch blob: %w", err)
	}
	defer reader.Close()

	// Write to temp file first
	tmpPath := blobPath + ".tmp"
	//nolint:gosec // G304: tmpPath is derived from blobPath which is derived from digest
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	// Hash while writing
	hasher := sha256.New()
	tee := io.TeeReader(reader, hasher)

	written, err := io.Copy(f, tee)
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write blob: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync blob: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close blob: %w", err)
	}

	// Verify digest
	computedHash := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if computedHash != desc.Digest {
		os.Remove(tmpPath)
		return fmt.Errorf("digest mismatch: expected %s, got %s", desc.Digest, computedHash)
	}

	// Verify size
	if written != desc.Size {
		os.Remove(tmpPath)
		return fmt.Errorf("size mismatch: expected %d, got %d", desc.Size, written)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, blobPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename blob: %w", err)
	}

	// Create entry metadata
	newEntry := &Entry{
		Version:   1,
		Digest:    desc.Digest,
		Size:      desc.Size,
		MediaType: desc.MediaType,
		Complete:  true,
		Verified:  true,
		Ref:       ref,
	}

	if err := saveEntry(entryPath, newEntry); err != nil {
		// Blob is saved, but entry failed - log but don't fail
		c.logger.Warn("failed to save cache entry", "error", err)
	}

	c.logger.Debug("cached blob", "digest", desc.Digest, "size", written)
	return nil
}

// resumeDownload attempts to resume a partial download.
func (c *Cache) resumeDownload(ctx context.Context, ref string, desc core.LayerDescriptor, partialPath, entryPath string, entry *Entry) error {
	// Open the partial file
	if err := ensureCacheFile(partialPath); err != nil {
		return fmt.Errorf("open partial file: %w", err)
	}
	//nolint:gosec // G304: partialPath is derived from digest, not user input
	f, err := os.OpenFile(partialPath, os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open partial file: %w", err)
	}
	defer f.Close()

	// Calculate missing ranges
	gaps := findGaps(entry.Ranges, desc.Size)
	if len(gaps) == 0 {
		// No gaps - this shouldn't happen if Complete is false
		return errors.New("no gaps but entry not complete")
	}

	c.logger.Debug("resuming download", "digest", desc.Digest, "gaps", len(gaps))

	// Download each missing range
	updatedRanges := entry.Ranges
	for _, gap := range gaps {
		if err := ctx.Err(); err != nil {
			// Save progress before returning
			c.savePartialProgress(entryPath, entry, updatedRanges, ref, desc)
			return err
		}

		// Fetch the range
		rangeReader, fetchErr := c.fallback.FetchBlobRange(ctx, ref, desc, gap.Offset, gap.Length)
		if fetchErr != nil {
			// If range requests aren't supported, fall back to full download
			if errors.Is(fetchErr, core.ErrRangeNotSupported) {
				c.logger.Debug("range requests not supported, falling back to full download")
				return fetchErr
			}
			// Save progress and return error
			c.savePartialProgress(entryPath, entry, updatedRanges, ref, desc)
			return fmt.Errorf("fetch range: %w", fetchErr)
		}

		// Write to the correct offset in the file
		written, writeErr := writeRangeToFile(f, rangeReader, gap.Offset, gap.Length)
		rangeReader.Close()
		if writeErr != nil {
			c.savePartialProgress(entryPath, entry, updatedRanges, ref, desc)
			return fmt.Errorf("write range: %w", writeErr)
		}

		// Track the downloaded range
		updatedRanges = addRange(updatedRanges, Range{Offset: gap.Offset, Length: written})
	}

	// Sync the file
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync partial file: %w", err)
	}

	// Verify the complete blob
	if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
		return fmt.Errorf("seek for verification: %w", seekErr)
	}

	hasher := sha256.New()
	if _, hashErr := io.Copy(hasher, f); hashErr != nil {
		return fmt.Errorf("hash for verification: %w", hashErr)
	}

	computedHash := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if computedHash != desc.Digest {
		// Digest mismatch - remove partial and force full download
		os.Remove(partialPath)
		os.Remove(entryPath)
		return fmt.Errorf("digest mismatch after resume: expected %s, got %s", desc.Digest, computedHash)
	}

	// Rename partial to final
	blobPath := partialPath[:len(partialPath)-len(".partial")]
	if err := os.Rename(partialPath, blobPath); err != nil {
		return fmt.Errorf("rename completed blob: %w", err)
	}

	// Update entry as complete
	entry.Complete = true
	entry.Verified = true
	entry.Ranges = nil // Clear ranges for complete blobs
	entry.Ref = ref
	if err := saveEntry(entryPath, entry); err != nil {
		c.logger.Warn("failed to save completed entry", "error", err)
	}

	c.logger.Debug("resumed download complete", "digest", desc.Digest)
	return nil
}

// writeRangeToFile writes data from reader to file at the specified offset.
// Returns the number of bytes written.
// Uses LimitReader to prevent writing past expectedLength if the registry
// returns more data than expected (e.g., 206 without Content-Range header).
func writeRangeToFile(f *os.File, reader io.Reader, offset, expectedLength int64) (int64, error) {
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, fmt.Errorf("seek to offset: %w", err)
	}

	// Limit reads to expectedLength to prevent corruption if registry
	// returns more data than expected (e.g., 206 without Content-Range).
	limitedReader := io.LimitReader(reader, expectedLength)
	written, err := io.Copy(f, limitedReader)
	if err != nil {
		return written, fmt.Errorf("copy data: %w", err)
	}

	// Validate length - must match exactly
	if written != expectedLength {
		return written, fmt.Errorf("length mismatch: expected %d, got %d", expectedLength, written)
	}

	return written, nil
}

// savePartialProgress saves the current download progress to the entry file.
func (c *Cache) savePartialProgress(entryPath string, entry *Entry, ranges []Range, ref string, desc core.LayerDescriptor) {
	entry.Ranges = ranges
	entry.Ref = ref
	entry.Complete = false
	entry.Verified = false
	entry.Size = desc.Size
	entry.Digest = desc.Digest
	entry.MediaType = desc.MediaType
	if err := saveEntry(entryPath, entry); err != nil {
		c.logger.Warn("failed to save partial progress", "error", err)
	}
}

// OpenLazy returns a BlobHandle that fetches data on-demand.
// Unlike Open, this doesn't download the entire blob upfront.
// Bytes are fetched via range requests as they are read.
//
// This is ideal for eStargz archives where only the TOC (footer)
// and specific file chunks need to be accessed.
//
// If the blob is already complete in cache, returns a regular fileHandle.
// Otherwise, returns a lazyHandle that fetches ranges incrementally.
//
// If the cached blob file is missing or corrupt, the entry is evicted
// and lazy loading starts fresh (self-healing).
func (c *Cache) OpenLazy(ctx context.Context, ref string, desc core.LayerDescriptor) (core.BlobHandle, error) {
	if c.verifyOnRead {
		return nil, errors.New("cache verify on read is incompatible with lazy loading")
	}
	entry, blobPath, entryPath := c.LoadCompleteEntry(desc.Digest)
	if entry != nil {
		c.logger.Debug("lazy cache hit (complete)", "digest", desc.Digest)
		handle, openErr := c.openCachedBlob(blobPath, entry)
		if openErr == nil {
			c.touchEntry(entryPath, entry)
			return handle, nil
		}
		c.selfHealEvict(desc.Digest, openErr)
	}

	// Need lazy loading - create or open partial file
	c.logger.Debug("lazy cache miss/partial", "digest", desc.Digest)
	return c.openLazyHandle(ctx, ref, desc, blobPath, entryPath)
}

// openLazyHandle creates a lazy handle for on-demand fetching.
func (c *Cache) openLazyHandle(
	ctx context.Context,
	ref string,
	desc core.LayerDescriptor,
	blobPath, entryPath string,
) (core.BlobHandle, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring lock
	entry, err := loadEntry(entryPath)
	if err == nil && entry.Complete && entry.Verified {
		// Another goroutine completed the download
		return c.openCachedBlob(blobPath, entry)
	}

	// Use existing entry if valid, otherwise create new one
	if entry == nil {
		entry = &Entry{
			Version:   1,
			Digest:    desc.Digest,
			Size:      desc.Size,
			MediaType: desc.MediaType,
			Complete:  false,
			Verified:  false,
			Ref:       ref,
		}
	} else {
		entry.Ref = ref
	}

	// Open or create the partial file
	partialPath := blobPath + ".partial"
	if checkErr := ensureCacheFileIfExists(partialPath); checkErr != nil {
		return nil, fmt.Errorf("open partial file: %w", checkErr)
	}
	//nolint:gosec // G304: partialPath is derived from digest, not user input
	f, err := os.OpenFile(partialPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open partial file: %w", err)
	}

	// Ensure file is exactly the right size (sparse file).
	// Truncate both undersized and oversized files to prevent
	// corruption from interrupted downloads or bad range data.
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat partial file: %w", err)
	}
	if info.Size() != desc.Size {
		// If file is larger than expected, clear ranges since the extra data is invalid
		if info.Size() > desc.Size && entry != nil {
			entry.Ranges = nil
			c.logger.Debug("truncating oversized partial file, clearing ranges",
				"digest", desc.Digest, "fileSize", info.Size(), "expectedSize", desc.Size)
		}
		if truncErr := f.Truncate(desc.Size); truncErr != nil {
			f.Close()
			return nil, fmt.Errorf("truncate partial file: %w", truncErr)
		}
	}

	// Save initial entry
	if saveErr := saveEntry(entryPath, entry); saveErr != nil {
		c.logger.Warn("failed to save lazy entry", "error", saveErr)
	}

	c.logger.Debug("created lazy handle", "digest", desc.Digest, "ranges", len(entry.Ranges))
	return newLazyHandle(ctx, c, ref, desc, f, entry, entryPath), nil
}

// Prefetch downloads the complete blob in the background if not already complete.
// This is useful for prefetching remaining data after a partial cache hit.
// The function returns immediately; the download happens asynchronously.
// Cancel the context to stop the prefetch.
func (c *Cache) Prefetch(ctx context.Context, ref string, desc core.LayerDescriptor) {
	go func() {
		c.mu.RLock()
		blobPath := c.blobPath(desc.Digest)
		entryPath := c.entryPath(desc.Digest)
		c.mu.RUnlock()

		// Check if already complete
		entry, err := loadEntry(entryPath)
		if err == nil && entry.Complete && entry.Verified {
			return // Already complete, nothing to prefetch
		}

		// Download in background
		c.logger.Debug("starting background prefetch", "digest", desc.Digest)
		if err := c.downloadBlob(ctx, ref, desc, blobPath, entryPath); err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				c.logger.Debug("background prefetch failed", "digest", desc.Digest, "error", err)
			}
			return
		}
		c.logger.Debug("background prefetch complete", "digest", desc.Digest)
	}()
}

// extractHash extracts the hash portion from a digest string.
// e.g., "sha256:abc123..." -> "abc123..."
func extractHash(digest string) string {
	if len(digest) > 7 && digest[:7] == "sha256:" {
		return digest[7:]
	}
	return digest
}
