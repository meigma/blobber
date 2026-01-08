package blobber

import "github.com/meigma/blobber/core"

// Sentinel errors for common failure conditions.
// Re-exported from core package.
var (
	// ErrNotFound indicates the requested image or file was not found.
	ErrNotFound = core.ErrNotFound

	// ErrUnauthorized indicates authentication failed.
	ErrUnauthorized = core.ErrUnauthorized

	// ErrInvalidRef indicates the image reference is malformed.
	ErrInvalidRef = core.ErrInvalidRef

	// ErrPathTraversal indicates a path traversal attack was detected.
	ErrPathTraversal = core.ErrPathTraversal

	// ErrExtractLimits indicates extraction safety limits were exceeded.
	ErrExtractLimits = core.ErrExtractLimits

	// ErrInvalidArchive indicates the blob is not a valid eStargz archive.
	ErrInvalidArchive = core.ErrInvalidArchive

	// ErrClosed indicates an operation was attempted on a closed resource.
	ErrClosed = core.ErrClosed
)
