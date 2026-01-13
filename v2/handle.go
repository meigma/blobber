// Package blobber provides a client for pushing and pulling files to OCI registries.
package blobber

import (
	"context"
	"io"
	"io/fs"
	"os"
	"sync/atomic"

	"github.com/opencontainers/go-digest"

	"github.com/meigma/blobber/v2/internal/cache"
	"github.com/meigma/blobber/v2/internal/estargz"
)

// BlobHandle provides fs.FS access to a blob's contents.
//
// BlobHandle can be backed by either a local temp file (from Pull) or
// network requests (from Stream). Both expose the same fs.FS interface.
//
// Call Close when done to release resources. For local-backed handles,
// this deletes the temp file. For network-backed handles, this closes
// any open connections.
type BlobHandle struct {
	ctx       context.Context
	reader    estargz.Reader
	closer    func() error
	streamAll func() (io.ReadCloser, error)
	closed    atomic.Bool

	// Cache fields for on-demand file caching (used by Stream).
	fileCache  cache.FileCache
	blobDigest digest.Digest
}

// Open implements fs.FS.
func (h *BlobHandle) Open(name string) (fs.File, error) {
	if h.closed.Load() {
		return nil, fs.ErrClosed
	}

	// Check file cache first (if configured).
	if h.fileCache != nil && h.blobDigest != "" {
		if f, err := h.fileCache.Get(h.blobDigest, name); err == nil {
			// Cache hit - return cached file.
			return f, nil
		}
		// Cache miss - continue to open from reader.
	}

	// Open from reader (network or local).
	f, err := h.reader.Open(name)
	if err != nil {
		return nil, err
	}

	// Wrap with caching tee if cache configured.
	if h.fileCache != nil && h.blobDigest != "" {
		// Get file size from Stat for the wrapper.
		info, statErr := f.Stat()
		if statErr == nil && !info.IsDir() {
			// Wrap regular files for caching.
			// Note: checksum verification is skipped (empty string) for simplicity.
			// The registry already verifies blob checksums.
			f = h.fileCache.WrapFile(h.blobDigest, name, f, info.Size(), "")
		}
	}

	return f, nil
}

// Stat implements fs.StatFS.
func (h *BlobHandle) Stat(name string) (fs.FileInfo, error) {
	if h.closed.Load() {
		return nil, fs.ErrClosed
	}
	return h.reader.Stat(name)
}

// ReadDir implements fs.ReadDirFS.
func (h *BlobHandle) ReadDir(name string) ([]fs.DirEntry, error) {
	if h.closed.Load() {
		return nil, fs.ErrClosed
	}
	return h.reader.ReadDir(name)
}

// Close releases resources associated with the handle.
// It is safe to call Close multiple times.
func (h *BlobHandle) Close() error {
	if !h.closed.CompareAndSwap(false, true) {
		return nil // already closed
	}
	if err := h.reader.Close(); err != nil {
		return err
	}
	if h.closer != nil {
		return h.closer()
	}
	return nil
}

// sizedFile wraps *os.File to implement estargz.SizedReaderAt.
type sizedFile struct {
	*os.File
	size int64
}

// Size returns the file size.
func (f *sizedFile) Size() int64 {
	return f.size
}

// newLocalHandle creates a BlobHandle backed by a local temp file.
//
// The temp file is deleted when Close is called.
func newLocalHandle(ctx context.Context, tempPath string) (*BlobHandle, error) {
	f, err := os.Open(tempPath) //nolint:gosec // tempPath is controlled by the library
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	src := &sizedFile{File: f, size: info.Size()}

	reader, err := estargz.NewReader(ctx, src)
	if err != nil {
		f.Close()
		return nil, err
	}

	return &BlobHandle{
		ctx:    ctx,
		reader: reader,
		closer: func() error {
			f.Close()
			return os.Remove(tempPath)
		},
		streamAll: func() (io.ReadCloser, error) {
			return os.Open(tempPath) //nolint:gosec // tempPath is controlled by the library
		},
	}, nil
}

// newNetworkHandle creates a BlobHandle backed by network range requests.
//
// The src provides random access to the blob for TOC-based file reads.
// The fetchFull function is called by CopyTo to fetch the entire blob
// in one request rather than per-file range requests.
func newNetworkHandle(ctx context.Context, src estargz.BlobSource, fetchFull func() (io.ReadCloser, error)) (*BlobHandle, error) {
	reader, err := estargz.NewReader(ctx, src)
	if err != nil {
		return nil, err
	}

	return &BlobHandle{
		ctx:       ctx,
		reader:    reader,
		closer:    src.Close,
		streamAll: fetchFull,
	}, nil
}

// dirReader implements estargz.Reader by wrapping a directory.
//
// This provides fs.FS access to extracted files in a cache directory,
// without the overhead of reading through an estargz archive.
type dirReader struct {
	path string
	fsys fs.FS
}

// newDirReader creates a reader backed by a directory.
func newDirReader(path string) *dirReader {
	return &dirReader{
		path: path,
		fsys: os.DirFS(path),
	}
}

// Open implements fs.FS.
func (r *dirReader) Open(name string) (fs.File, error) {
	return r.fsys.Open(name)
}

// Stat implements fs.StatFS.
func (r *dirReader) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(r.fsys, name)
}

// ReadDir implements fs.ReadDirFS.
func (r *dirReader) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(r.fsys, name)
}

// Close implements io.Closer. For directory-backed readers, this is a no-op.
func (r *dirReader) Close() error {
	return nil
}

// newDirHandle creates a BlobHandle backed by a directory of extracted files.
//
// This is used when serving from the file cache. The directory is not deleted
// on Close since it's managed by the cache.
func newDirHandle(ctx context.Context, dirPath string) (*BlobHandle, error) {
	reader := newDirReader(dirPath)

	return &BlobHandle{
		ctx:    ctx,
		reader: reader,
		closer: nil, // Directory is managed by cache, not deleted on close.
		streamAll: func() (io.ReadCloser, error) {
			// For CopyTo on a dir-backed handle, we could walk and tar,
			// but it's simpler to just error - CopyTo should use the cache dir directly.
			return nil, fs.ErrInvalid
		},
	}, nil
}
