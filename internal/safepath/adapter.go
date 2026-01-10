package safepath

import "github.com/meigma/blobber/core"

// Adapter provides the path validation surface for blobber clients.
type Adapter struct {
	*Validator
}

// Compile-time interface implementation check.
var _ core.PathValidator = (*Adapter)(nil)

// NewAdapter creates a new adapter for path validation.
func NewAdapter() *Adapter {
	return &Adapter{Validator: NewValidator()}
}
