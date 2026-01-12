// Package blobber provides a client for pushing and pulling files to OCI registries.
package blobber

import (
	"context"
	"io"
	"io/fs"
	"os"
	"sync/atomic"

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
}

// Open implements fs.FS.
func (h *BlobHandle) Open(name string) (fs.File, error) {
	if h.closed.Load() {
		return nil, fs.ErrClosed
	}
	return h.reader.Open(name)
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
