package blobber

import (
	"context"
	"io/fs"
)

// List returns the file listing of an image without downloading content.
// The ref must be fully qualified (e.g., "ghcr.io/org/repo:tag").
// This leverages eStargz for efficient remote listing.
func (c *Client) List(ctx context.Context, ref string) ([]FileEntry, error) {
	panic("not implemented")
}

// Walk walks the file tree of an image, calling fn for each file or directory.
// The ref must be fully qualified (e.g., "ghcr.io/org/repo:tag").
// This leverages eStargz for efficient remote listing without downloading content.
// Errors returned by fn control traversal (e.g., fs.SkipDir, fs.SkipAll).
func (c *Client) Walk(ctx context.Context, ref string, fn fs.WalkDirFunc) error {
	panic("not implemented")
}
