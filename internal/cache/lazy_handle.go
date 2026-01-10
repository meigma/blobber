package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/meigma/blobber/core"
	"github.com/meigma/blobber/internal/contracts"
)

// Compile-time interface check.
var _ contracts.BlobHandle = (*lazyHandle)(nil)

// lazyHandle implements contracts.BlobHandle with on-demand range fetching.
// It fetches only the byte ranges that are actually read, caching them
// to disk for future access. This enables efficient selective file access
// from eStargz archives without downloading the entire blob.
type lazyHandle struct {
	cache    *Cache
	ref      string
	desc     core.LayerDescriptor
	ctx      context.Context
	cancelFn context.CancelFunc

	mu        sync.RWMutex
	file      *os.File // The cached file (may be sparse/partial)
	entry     *Entry   // Tracked ranges and metadata
	entryPath string   // Path to entry metadata file
	closed    bool
}

// newLazyHandle creates a lazy handle for on-demand blob fetching.
// The file should be opened or created with read/write access.
func newLazyHandle(
	ctx context.Context,
	c *Cache,
	ref string,
	desc core.LayerDescriptor,
	file *os.File,
	entry *Entry,
	entryPath string,
) *lazyHandle {
	ctx, cancel := context.WithCancel(ctx)
	return &lazyHandle{
		cache:     c,
		ref:       ref,
		desc:      desc,
		ctx:       ctx,
		cancelFn:  cancel,
		file:      file,
		entry:     entry,
		entryPath: entryPath,
	}
}

// ReadAt implements io.ReaderAt with on-demand fetching.
// If the requested range is not cached, it fetches from the registry.
func (h *lazyHandle) ReadAt(p []byte, off int64) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return 0, core.ErrClosed
	}

	length := int64(len(p))

	// Check if we have this range cached
	if h.entry.Complete || containsRange(h.entry.Ranges, off, length) {
		return h.file.ReadAt(p, off)
	}

	// Need to fetch missing data
	if err := h.fetchMissingRanges(off, length); err != nil {
		return 0, fmt.Errorf("fetch range: %w", err)
	}

	return h.file.ReadAt(p, off)
}

// fetchMissingRanges fetches any byte ranges not already cached.
// Caller must hold h.mu lock.
func (h *lazyHandle) fetchMissingRanges(off, length int64) error {
	// Calculate what we need to fetch
	requestEnd := off + length
	if requestEnd > h.desc.Size {
		requestEnd = h.desc.Size
	}

	// Find gaps in the cached ranges that overlap with our request
	gaps := findGapsInRange(h.entry.Ranges, off, requestEnd-off)
	if len(gaps) == 0 {
		return nil // Everything is cached
	}

	// Fetch each gap
	for _, gap := range gaps {
		if err := h.ctx.Err(); err != nil {
			h.saveProgress()
			return err
		}

		// Fetch the range from registry
		reader, err := h.cache.fallback.FetchBlobRange(h.ctx, h.ref, h.desc, gap.Offset, gap.Length)
		if err != nil {
			h.saveProgress()
			return fmt.Errorf("fetch blob range at %d: %w", gap.Offset, err)
		}

		// Write to file at the correct offset
		written, writeErr := writeRangeToFile(h.file, reader, gap.Offset, gap.Length)
		reader.Close()
		if writeErr != nil {
			h.saveProgress()
			return fmt.Errorf("write range at %d: %w", gap.Offset, writeErr)
		}

		// Update tracked ranges
		h.entry.Ranges = addRange(h.entry.Ranges, Range{Offset: gap.Offset, Length: written})
	}

	// Check if we now have the complete blob
	if isComplete(h.entry.Ranges, h.desc.Size) {
		if err := h.verifyAndComplete(); err != nil {
			// Verification failed - keep as partial
			h.cache.logger.Debug("lazy handle verification failed", "error", err)
		}
	}

	// Save progress
	h.saveProgress()
	return nil
}

// verifyAndComplete verifies the full blob digest and marks as complete.
// Caller must hold h.mu lock.
func (h *lazyHandle) verifyAndComplete() error {
	// Sync to ensure all data is written
	if err := h.file.Sync(); err != nil {
		return fmt.Errorf("sync file: %w", err)
	}

	// Verify the complete blob
	if _, err := h.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek for verification: %w", err)
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, h.file); err != nil {
		return fmt.Errorf("hash for verification: %w", err)
	}

	computedHash := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if computedHash != h.desc.Digest {
		return fmt.Errorf("digest mismatch: expected %s, got %s", h.desc.Digest, computedHash)
	}

	// Mark as complete and verified
	h.entry.Complete = true
	h.entry.Verified = true
	h.entry.Ranges = nil // Clear ranges for complete blobs

	// Rename partial file to final
	blobPath := h.cache.blobPath(h.desc.Digest)
	partialPath := blobPath + ".partial"
	if h.file.Name() == partialPath {
		// Close and reopen after rename
		h.file.Close()
		if err := os.Rename(partialPath, blobPath); err != nil {
			return fmt.Errorf("rename to final: %w", err)
		}
		//nolint:gosec // G304: blobPath is derived from digest, not user input
		f, err := os.Open(blobPath)
		if err != nil {
			return fmt.Errorf("reopen after rename: %w", err)
		}
		h.file = f
	}

	h.cache.logger.Debug("lazy handle completed and verified", "digest", h.desc.Digest)
	return nil
}

// saveProgress saves the current entry state to disk.
// Caller must hold h.mu lock.
func (h *lazyHandle) saveProgress() {
	if err := saveEntry(h.entryPath, h.entry); err != nil {
		h.cache.logger.Warn("failed to save lazy handle progress", "error", err)
	}
}

// Close implements io.Closer.
func (h *lazyHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return nil
	}
	h.closed = true

	// Cancel any ongoing fetches
	h.cancelFn()

	// Save final state
	h.saveProgress()

	return h.file.Close()
}

// Size returns the total blob size.
func (h *lazyHandle) Size() int64 {
	return h.desc.Size
}

// Complete reports whether the entire blob is available locally.
func (h *lazyHandle) Complete() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.entry.Complete && h.entry.Verified
}

// findGapsInRange returns the gaps within a specific range.
// This is similar to findGaps but limited to a specific range of interest.
func findGapsInRange(ranges []Range, offset, length int64) []Range {
	if length <= 0 {
		return nil
	}

	end := offset + length

	// Create a temporary total size for findGaps
	gaps := findGaps(ranges, end)

	// Filter to only gaps that overlap with our requested range
	var result []Range
	for _, gap := range gaps {
		gapEnd := gap.Offset + gap.Length

		// Check if this gap overlaps with our range
		if gap.Offset >= end || gapEnd <= offset {
			continue // No overlap
		}

		// Clip to our range
		clippedStart := gap.Offset
		if clippedStart < offset {
			clippedStart = offset
		}
		clippedEnd := gapEnd
		if clippedEnd > end {
			clippedEnd = end
		}

		if clippedEnd > clippedStart {
			result = append(result, Range{
				Offset: clippedStart,
				Length: clippedEnd - clippedStart,
			})
		}
	}

	return result
}
