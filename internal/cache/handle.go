package cache

import (
	"os"

	"github.com/meigma/blobber/core"
)

// Compile-time interface check.
var _ core.BlobHandle = (*fileHandle)(nil)

// fileHandle implements core.BlobHandle for a cached file.
type fileHandle struct {
	file     *os.File
	size     int64
	complete bool
}

// ReadAt implements io.ReaderAt.
func (h *fileHandle) ReadAt(p []byte, off int64) (n int, err error) {
	return h.file.ReadAt(p, off)
}

// Close implements io.Closer.
func (h *fileHandle) Close() error {
	return h.file.Close()
}

// Size returns the total blob size.
func (h *fileHandle) Size() int64 {
	return h.size
}

// Complete reports whether the entire blob is available.
func (h *fileHandle) Complete() bool {
	return h.complete
}
