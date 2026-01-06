package blobber

import (
	"context"
	"fmt"
	"log/slog"
)

// Client provides operations against OCI registries.
type Client struct {
	registry  Registry
	builder   ArchiveBuilder
	reader    ArchiveReader
	validator PathValidator
	logger    *slog.Logger

	// credential configuration
	credStore any // credentials.Store from ORAS
}

// NewClient creates a new blobber client.
//
// By default, credentials are resolved from Docker config (~/.docker/config.json)
// and credential helpers. Use WithCredentials or WithCredentialStore to override.
func NewClient(opts ...ClientOption) (*Client, error) {
	panic("not implemented")
}

// OpenImage opens a remote image for reading.
// The ref must be fully qualified (e.g., "ghcr.io/org/repo:tag").
//
// The returned Image caches the eStargz reader for efficient multiple file access.
// The caller must call Image.Close when done to release resources.
func (c *Client) OpenImage(ctx context.Context, ref string) (*Image, error) {
	// Pull the blob from registry
	blob, size, err := c.registry.Pull(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("pull %s: %w", ref, err)
	}
	defer blob.Close()

	// Create Image with cached reader
	img, err := NewImageFromBlob(ref, blob, size, c.validator, c.logger)
	if err != nil {
		return nil, fmt.Errorf("open image %s: %w", ref, err)
	}

	return img, nil
}
