package blobber

import (
	"context"
	"io"
)

// Open returns a reader for a specific file within an image.
// The ref must be fully qualified (e.g., "ghcr.io/org/repo:tag").
// This leverages eStargz for efficient selective retrieval.
// The caller is responsible for closing the returned ReadCloser.
func (c *Client) Open(ctx context.Context, ref string, path string) (io.ReadCloser, error) {
	panic("not implemented")
}
