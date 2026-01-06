// Package safepath provides path validation for secure file extraction.
package safepath

import (
	"github.com/gilmanlab/blobber"
)

// Compile-time interface implementation check.
var _ blobber.PathValidator = (*validator)(nil)

// validator implements blobber.PathValidator.
type validator struct{}

// NewValidator creates a new PathValidator.
func NewValidator() *validator {
	return &validator{}
}

// ValidateExtraction checks if extracting the given entries to destDir is safe.
func (v *validator) ValidateExtraction(destDir string, entries []blobber.TOCEntry, limits blobber.ExtractLimits) error {
	panic("not implemented")
}

// ValidatePath checks if a path is safe (no traversal, valid characters).
func (v *validator) ValidatePath(path string) error {
	panic("not implemented")
}

// ValidateSymlink checks if a symlink target is safe (stays within destDir).
func (v *validator) ValidateSymlink(destDir, linkPath, target string) error {
	panic("not implemented")
}

func containsNull(path string) bool {
	panic("not implemented")
}

func containsTraversal(path string) bool {
	panic("not implemented")
}

func isAbsolute(path string) bool {
	panic("not implemented")
}
