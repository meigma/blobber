package blobber

import (
	"log/slog"

	"oras.land/oras-go/v2/registry/remote/credentials"

	"github.com/gilmanlab/blobber/core"
	"github.com/gilmanlab/blobber/internal/registry"
)

// ClientOption configures a Client.
type ClientOption func(*Client) error

// PushOption configures a Push operation.
type PushOption func(*pushConfig)

// PullOption configures a Pull operation.
type PullOption func(*pullConfig)

// ExtractLimits defines safety limits for extraction.
// Re-exported from core package.
type ExtractLimits = core.ExtractLimits

// pushConfig holds configuration for Push operations.
type pushConfig struct {
	annotations map[string]string
	mediaType   string
	compression Compression
}

// pullConfig holds configuration for Pull operations.
type pullConfig struct {
	limits ExtractLimits
}

// WithAnnotations sets OCI annotations on the pushed image.
func WithAnnotations(annotations map[string]string) PushOption {
	return func(c *pushConfig) {
		c.annotations = annotations
	}
}

// WithCompression sets the compression algorithm (gzip or zstd).
func WithCompression(c Compression) PushOption {
	return func(cfg *pushConfig) {
		cfg.compression = c
	}
}

// WithCredentials sets explicit credentials for a specific registry.
func WithCredentials(registryHost, username, password string) ClientOption {
	return func(c *Client) error {
		c.credStore = staticCredentials(registryHost, username, password)
		return nil
	}
}

// WithCredentialStore sets a custom credential store.
func WithCredentialStore(store credentials.Store) ClientOption {
	return func(c *Client) error {
		c.credStore = store
		return nil
	}
}

// WithExtractLimits sets safety limits for extraction.
func WithExtractLimits(limits ExtractLimits) PullOption {
	return func(c *pullConfig) {
		c.limits = limits
	}
}

// WithInsecure allows connections to registries without TLS.
func WithInsecure(insecure bool) ClientOption {
	return func(c *Client) error {
		c.plainHTTP = insecure
		return nil
	}
}

// WithLogger sets a logger for the client. By default, logging is disabled.
func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) error {
		c.logger = logger
		return nil
	}
}

// WithMediaType sets a custom media type for the layer.
func WithMediaType(mt string) PushOption {
	return func(c *pushConfig) {
		c.mediaType = mt
	}
}

// WithUserAgent sets a custom User-Agent header for registry requests.
func WithUserAgent(ua string) ClientOption {
	return func(c *Client) error {
		c.userAgent = ua
		return nil
	}
}

// staticCredentials returns a credential store with a single static credential.
func staticCredentials(registryHost, username, password string) credentials.Store {
	return registry.StaticCredentials(registryHost, username, password)
}
