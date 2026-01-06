package blobber

import (
	"context"
	"io"
)

// Registry handles OCI registry operations.
// This interface is implemented by internal/registry.
type Registry interface {
	// Push uploads a blob and creates a manifest.
	// Returns the manifest digest.
	Push(ctx context.Context, ref string, layer io.Reader, opts RegistryPushOptions) (string, error)

	// Pull returns a reader for the image's layer blob and its size.
	Pull(ctx context.Context, ref string) (io.ReadCloser, int64, error)

	// PullRange fetches a byte range from the layer blob.
	// Used for selective file retrieval from eStargz.
	PullRange(ctx context.Context, ref string, offset, length int64) (io.ReadCloser, error)
}

// RegistryPushOptions contains metadata for push operations.
type RegistryPushOptions struct {
	MediaType   string
	Annotations map[string]string
	TOCDigest   string
}
