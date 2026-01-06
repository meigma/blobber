package blobber

import (
	"io/fs"
	"log/slog"
)

// ClientOption configures a Client.
type ClientOption func(*Client) error

// PushOption configures a Push operation.
type PushOption func(*pushConfig)

// PullOption configures a Pull operation.
type PullOption func(*pullConfig)

// ExtractLimits defines safety limits for extraction.
type ExtractLimits struct {
	MaxFiles     int   // Maximum number of files (0 = no limit)
	MaxTotalSize int64 // Maximum total extracted size (0 = no limit)
	MaxFileSize  int64 // Maximum single file size (0 = no limit)
}

// pushConfig holds configuration for Push operations.
type pushConfig struct {
	annotations map[string]string
	mediaType   string
	compression Compression
}

// pullConfig holds configuration for Pull operations.
type pullConfig struct {
	overwrite bool
	fileMode  fs.FileMode
	limits    ExtractLimits
}

// WithAnnotations sets OCI annotations on the pushed image.
func WithAnnotations(annotations map[string]string) PushOption {
	panic("not implemented")
}

// WithCompression sets the compression algorithm (gzip or zstd).
func WithCompression(c Compression) PushOption {
	panic("not implemented")
}

// WithCredentials sets explicit credentials for a specific registry.
func WithCredentials(registry, username, password string) ClientOption {
	panic("not implemented")
}

// WithCredentialStore sets a custom credential store (ORAS credentials.Store).
func WithCredentialStore(store any) ClientOption {
	panic("not implemented")
}

// WithExtractLimits sets safety limits for extraction.
func WithExtractLimits(limits ExtractLimits) PullOption {
	panic("not implemented")
}

// WithFileMode sets the file mode for extracted files.
func WithFileMode(mode fs.FileMode) PullOption {
	panic("not implemented")
}

// WithInsecure allows connections to registries without TLS.
func WithInsecure(insecure bool) ClientOption {
	panic("not implemented")
}

// WithLogger sets a logger for the client. By default, logging is disabled.
func WithLogger(logger *slog.Logger) ClientOption {
	panic("not implemented")
}

// WithMediaType sets a custom media type for the layer.
func WithMediaType(mt string) PushOption {
	panic("not implemented")
}

// WithOverwrite allows overwriting existing files during extraction.
func WithOverwrite(overwrite bool) PullOption {
	panic("not implemented")
}

// WithUserAgent sets a custom User-Agent header for registry requests.
func WithUserAgent(ua string) ClientOption {
	panic("not implemented")
}
