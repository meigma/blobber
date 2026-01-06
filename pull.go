package blobber

import (
	"context"
	"fmt"

	"github.com/gilmanlab/blobber/internal/archive"
)

// Pull downloads all files from the image to the destination directory.
// The ref must be fully qualified (e.g., "ghcr.io/org/repo:tag").
func (c *Client) Pull(ctx context.Context, ref, destDir string, opts ...PullOption) error {
	// Apply options
	cfg := &pullConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Pull blob from registry
	blob, _, err := c.registry.Pull(ctx, ref)
	if err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}
	defer blob.Close()

	// Extract to destination
	if err := archive.Extract(ctx, blob, destDir, c.validator, cfg.limits); err != nil {
		return fmt.Errorf("extract %s: %w", ref, err)
	}

	return nil
}
