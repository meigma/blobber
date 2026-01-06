// Package registry provides OCI registry operations using ORAS.
package registry

import (
	"context"
	"io"

	"github.com/gilmanlab/blobber"
)

// Compile-time interface implementation check.
var _ blobber.Registry = (*orasRegistry)(nil)

// Option configures an orasRegistry.
type Option func(*orasRegistry)

// orasRegistry implements blobber.Registry using ORAS.
type orasRegistry struct {
	plainHTTP bool
	userAgent string
	credStore any // credentials.Store
}

// New creates a new Registry backed by ORAS.
func New(opts ...Option) *orasRegistry {
	panic("not implemented")
}

// Pull returns a reader for the image's layer blob.
func (r *orasRegistry) Pull(ctx context.Context, ref string) (io.ReadCloser, int64, error) {
	panic("not implemented")
}

// PullRange fetches a byte range from the layer blob.
func (r *orasRegistry) PullRange(ctx context.Context, ref string, offset, length int64) (io.ReadCloser, error) {
	panic("not implemented")
}

// Push uploads a blob and creates a manifest.
func (r *orasRegistry) Push(ctx context.Context, ref string, layer io.Reader, opts blobber.RegistryPushOptions) (string, error) {
	panic("not implemented")
}

// WithCredentialStore sets the credential store.
func WithCredentialStore(store any) Option {
	return func(r *orasRegistry) {
		r.credStore = store
	}
}

// WithPlainHTTP enables insecure HTTP connections.
func WithPlainHTTP(plainHTTP bool) Option {
	return func(r *orasRegistry) {
		r.plainHTTP = plainHTTP
	}
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(r *orasRegistry) {
		r.userAgent = ua
	}
}
