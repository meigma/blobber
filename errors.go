package blobber

import "errors"

// Sentinel errors for common failure conditions.
var (
	// ErrNotFound indicates the requested image or file was not found.
	ErrNotFound = errors.New("blobber: not found")

	// ErrUnauthorized indicates authentication failed.
	ErrUnauthorized = errors.New("blobber: unauthorized")

	// ErrInvalidRef indicates the image reference is malformed.
	ErrInvalidRef = errors.New("blobber: invalid reference")

	// ErrPathTraversal indicates a path traversal attack was detected.
	ErrPathTraversal = errors.New("blobber: path traversal detected")

	// ErrExtractLimits indicates extraction safety limits were exceeded.
	ErrExtractLimits = errors.New("blobber: extraction limits exceeded")

	// ErrInvalidArchive indicates the blob is not a valid eStargz archive.
	ErrInvalidArchive = errors.New("blobber: invalid eStargz archive")

	// ErrClosed indicates an operation was attempted on a closed resource.
	ErrClosed = errors.New("blobber: resource closed")
)
