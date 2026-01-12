package blobber

import (
	"fmt"

	"github.com/meigma/blobber/v2/internal/estargz"
)

// CopyTo extracts all files from a BlobHandle to the destination directory.
//
// The destination directory must exist. Files are extracted with their
// original permissions and directory structure preserved.
//
// For network-backed handles, this fetches the entire blob in a single
// request rather than issuing per-file range requests.
func CopyTo(h *BlobHandle, dest string) error {
	rc, err := h.streamAll()
	if err != nil {
		return fmt.Errorf("stream blob: %w", err)
	}
	defer rc.Close()

	extractor := estargz.NewExtractor()
	if err := extractor.Extract(h.ctx, rc, dest); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	return nil
}
