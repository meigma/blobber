package blobber

import (
	"context"
	"fmt"
	"log/slog"

	"oras.land/oras-go/v2/registry/remote/credentials"

	"github.com/gilmanlab/blobber/internal/archive"
	"github.com/gilmanlab/blobber/internal/registry"
	"github.com/gilmanlab/blobber/internal/safepath"
)

// Client provides operations against OCI registries.
type Client struct {
	registry  Registry
	builder   ArchiveBuilder
	reader    ArchiveReader
	validator PathValidator
	logger    *slog.Logger

	// configuration passed to registry
	credStore any // credentials.Store from ORAS
	plainHTTP bool
	userAgent string
}

// NewClient creates a new blobber client.
//
// By default, credentials are resolved from Docker config (~/.docker/config.json)
// and credential helpers. Use WithCredentials or WithCredentialStore to override.
func NewClient(opts ...ClientOption) (*Client, error) {
	c := &Client{
		logger: slog.New(slog.DiscardHandler),
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	// Set up credential store if not provided
	if c.credStore == nil {
		store, err := registry.DefaultCredentialStore()
		if err != nil {
			return nil, fmt.Errorf("create credential store: %w", err)
		}
		c.credStore = store
	}

	// Wire up default implementations
	var regOpts []registry.Option
	if store, ok := c.credStore.(credentials.Store); ok {
		regOpts = append(regOpts, registry.WithCredentialStore(store))
	}
	if c.plainHTTP {
		regOpts = append(regOpts, registry.WithPlainHTTP(true))
	}
	if c.userAgent != "" {
		regOpts = append(regOpts, registry.WithUserAgent(c.userAgent))
	}

	c.registry = registry.New(regOpts...)
	c.builder = archive.NewBuilder(c.logger)
	c.reader = archive.NewReader()
	c.validator = safepath.NewValidator()

	return c, nil
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
