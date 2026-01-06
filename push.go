package blobber

import (
	"context"
	"fmt"
	"io/fs"
)

// Push uploads files from src to the given image reference.
// The ref must be fully qualified (e.g., "ghcr.io/org/repo:tag").
// Returns the digest of the pushed image.
func (c *Client) Push(ctx context.Context, ref string, src fs.FS, opts ...PushOption) (string, error) {
	// Apply options
	cfg := &pushConfig{
		compression: GzipCompression(), // default
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// Build eStargz blob
	result, err := c.builder.Build(ctx, src, cfg.compression)
	if err != nil {
		return "", fmt.Errorf("build archive: %w", err)
	}
	defer result.Blob.Close()

	// Push to registry
	regOpts := RegistryPushOptions{
		MediaType:   cfg.mediaType,
		Annotations: cfg.annotations,
		TOCDigest:   result.TOCDigest,
		DiffID:      result.DiffID,
		BlobDigest:  result.BlobDigest,
		BlobSize:    result.BlobSize,
	}

	digest, err := c.registry.Push(ctx, ref, result.Blob, &regOpts)
	if err != nil {
		return "", fmt.Errorf("push %s: %w", ref, err)
	}

	return digest, nil
}
