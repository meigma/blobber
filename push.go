package blobber

import (
	"context"
	"io/fs"
)

// Push uploads files from src to the given image reference.
// The ref must be fully qualified (e.g., "ghcr.io/org/repo:tag").
// Returns the digest of the pushed image.
func (c *Client) Push(ctx context.Context, ref string, src fs.FS, opts ...PushOption) (string, error) {
	panic("not implemented")
}
