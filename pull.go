package blobber

import (
	"context"
)

// Pull downloads all files from the image to the destination directory.
// The ref must be fully qualified (e.g., "ghcr.io/org/repo:tag").
func (c *Client) Pull(ctx context.Context, ref string, destDir string, opts ...PullOption) error {
	panic("not implemented")
}
