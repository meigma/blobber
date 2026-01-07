package blobber

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"sort"
	"sync"

	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/containerd/stargz-snapshotter/estargz/zstdchunked"
)

// Compile-time interface check.
var _ io.Closer = (*Image)(nil)

// Image represents an opened remote image for reading.
// It caches the eStargz reader for efficient multiple file access.
// Image is safe for concurrent use.
//
// The caller must call Close when done to release resources.
type Image struct {
	mu     sync.RWMutex
	closed bool

	ref        string
	blobFile   *os.File        // temp file with blob data (non-cached path)
	blobHandle BlobHandle      // cached blob handle (cached path)
	blobSize   int64           // size of the blob
	esr        *estargz.Reader // cached estargz reader

	validator PathValidator
	logger    *slog.Logger
}

// NewImageFromBlob creates a new Image from a blob reader.
// The blob is written to a temp file for random access.
//
// This is a low-level constructor primarily for testing. Most users should
// use Client.OpenImage instead.
func NewImageFromBlob(ref string, blob io.Reader, size int64, validator PathValidator, logger *slog.Logger) (*Image, error) {
	// Create temp file for blob storage
	f, err := os.CreateTemp("", "blobber-image-*")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := f.Name()

	// Copy blob to temp file
	written, err := io.Copy(f, blob)
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("copy blob to temp file: %w", err)
	}

	// Verify size if provided
	if size > 0 && written != size {
		f.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("blob size mismatch: expected %d, got %d", size, written)
	}

	// Seek to beginning for reading
	if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
		f.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("seek temp file: %w", seekErr)
	}

	// Create estargz reader with zstd support
	sr := io.NewSectionReader(f, 0, written)
	esr, err := estargz.Open(sr, estargz.WithDecompressors(&zstdchunked.Decompressor{}))
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
	}

	return &Image{
		ref:       ref,
		blobFile:  f,
		blobSize:  written,
		esr:       esr,
		validator: validator,
		logger:    logger,
	}, nil
}

// NewImageFromHandle creates a new Image from a BlobHandle.
// Used when opening images from cache, where the blob is already on disk.
//
// This is a low-level constructor primarily for cache integration. Most users
// should use Client.OpenImage instead.
func NewImageFromHandle(ref string, handle BlobHandle, validator PathValidator, logger *slog.Logger) (*Image, error) {
	size := handle.Size()

	// Create estargz reader directly from the handle (which implements io.ReaderAt)
	sr := io.NewSectionReader(handle, 0, size)
	esr, err := estargz.Open(sr, estargz.WithDecompressors(&zstdchunked.Decompressor{}))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
	}

	return &Image{
		ref:        ref,
		blobHandle: handle,
		blobSize:   size,
		esr:        esr,
		validator:  validator,
		logger:     logger,
	}, nil
}

// Close releases resources associated with the image.
// After Close, all other methods will return an error.
func (img *Image) Close() error {
	img.mu.Lock()
	defer img.mu.Unlock()

	if img.closed {
		return nil
	}
	img.closed = true

	// Handle cached path (blobHandle)
	if img.blobHandle != nil {
		return img.blobHandle.Close()
	}

	// Handle non-cached path (blobFile)
	if img.blobFile != nil {
		path := img.blobFile.Name()
		closeErr := img.blobFile.Close()
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			img.logger.Warn("failed to remove temp file", "path", path, "error", err)
		}
		return closeErr
	}

	return nil
}

// List returns the file listing of the image.
func (img *Image) List() ([]FileEntry, error) {
	img.mu.RLock()
	defer img.mu.RUnlock()

	if img.closed {
		return nil, ErrClosed
	}

	var entries []FileEntry
	root, ok := img.esr.Lookup("")
	if !ok {
		return entries, nil
	}

	var collect func(e *estargz.TOCEntry)
	collect = func(e *estargz.TOCEntry) {
		if e.Name != "" {
			entries = append(entries, tocEntryToFileEntry(e))
		}
		if e.Type == "dir" {
			e.ForeachChild(func(_ string, child *estargz.TOCEntry) bool {
				collect(child)
				return true
			})
		}
	}
	collect(root)

	// Sort by path for consistent output
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].FilePath < entries[j].FilePath
	})

	return entries, nil
}

// Open returns a reader for a specific file within the image.
// The caller is responsible for closing the returned ReadCloser.
func (img *Image) Open(path string) (io.ReadCloser, error) {
	img.mu.RLock()
	defer img.mu.RUnlock()

	if img.closed {
		return nil, ErrClosed
	}

	// Validate path
	if err := img.validator.ValidatePath(path); err != nil {
		return nil, err
	}

	// Look up file in TOC
	entry, ok := img.esr.Lookup(path)
	if !ok {
		return nil, fmt.Errorf("%s: %w", path, ErrNotFound)
	}

	if entry.Type != "reg" {
		return nil, fmt.Errorf("%s: not a regular file", path)
	}

	// Open file from estargz
	ra, err := img.esr.OpenFile(entry.Name)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	// Wrap as ReadCloser (SectionReader is limited to entry.Size)
	return io.NopCloser(io.NewSectionReader(ra, 0, entry.Size)), nil
}

// Walk walks the file tree of the image, calling fn for each file or directory.
// Errors returned by fn control traversal (e.g., fs.SkipDir, fs.SkipAll).
func (img *Image) Walk(fn fs.WalkDirFunc) error {
	img.mu.RLock()
	defer img.mu.RUnlock()

	if img.closed {
		return ErrClosed
	}

	root, ok := img.esr.Lookup("")
	if !ok {
		return nil
	}

	return img.walkEntry(root, fn)
}

// walkEntry recursively walks a TOC entry tree.
func (img *Image) walkEntry(e *estargz.TOCEntry, fn fs.WalkDirFunc) error {
	// Skip root entry itself
	if e.Name != "" {
		entry := tocEntryToFileEntry(e)
		if err := fn(e.Name, entry, nil); err != nil {
			if errors.Is(err, fs.SkipDir) {
				return nil
			}
			if errors.Is(err, fs.SkipAll) {
				return fs.SkipAll
			}
			return err
		}
	}

	if e.Type == "dir" {
		var walkErr error
		e.ForeachChild(func(_ string, child *estargz.TOCEntry) bool {
			if err := img.walkEntry(child, fn); err != nil {
				if errors.Is(err, fs.SkipAll) {
					walkErr = fs.SkipAll
					return false
				}
				walkErr = err
				return false
			}
			return true
		})
		if walkErr != nil {
			return walkErr
		}
	}

	return nil
}

// tocEntryToFileEntry converts an estargz.TOCEntry to a FileEntry.
func tocEntryToFileEntry(e *estargz.TOCEntry) FileEntry {
	return FileEntry{
		FilePath: e.Name,
		FileSize: e.Size,
		//nolint:gosec // G115: Mode from trusted estargz TOC entry
		FileMode: fs.FileMode(e.Mode),
	}
}
