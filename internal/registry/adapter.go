package registry

import "github.com/meigma/blobber/core"

// Adapter provides a narrow registry surface for blobber clients.
type Adapter struct {
	*orasRegistry
}

// Compile-time interface implementation check.
var _ core.Registry = (*Adapter)(nil)

// NewAdapter creates a new adapter backed by the ORAS registry implementation.
func NewAdapter(opts ...Option) *Adapter {
	return &Adapter{orasRegistry: New(opts...)}
}
