package oci

import (
	"log/slog"

	"oras.land/oras-go/v2/registry/remote/credentials"
)

// Option configures a Client.
type Option func(*client)

// WithCredentialStore sets the credential store for authentication.
// If not set, requests are made without authentication.
func WithCredentialStore(store credentials.Store) Option {
	return func(c *client) {
		c.credStore = store
	}
}

// WithPlainHTTP enables insecure HTTP connections instead of HTTPS.
// This should only be used for local development or testing.
func WithPlainHTTP(plainHTTP bool) Option {
	return func(c *client) {
		c.plainHTTP = plainHTTP
	}
}

// WithUserAgent sets the User-Agent header for HTTP requests.
func WithUserAgent(ua string) Option {
	return func(c *client) {
		c.userAgent = ua
	}
}

// WithLogger sets the logger for debug output.
func WithLogger(logger *slog.Logger) Option {
	return func(c *client) {
		c.logger = logger
	}
}
