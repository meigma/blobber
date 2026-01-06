package blobber

import (
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
