package tar

import "errors"

// Sentinel errors for tar operations.
var (
	// ErrPathTraversal indicates a path traversal attempt was detected.
	ErrPathTraversal = errors.New("path traversal detected")

	// ErrLimitExceeded indicates an extraction limit was exceeded.
	ErrLimitExceeded = errors.New("extraction limit exceeded")

	// ErrUnsupportedEntry indicates an unsupported tar entry type.
	ErrUnsupportedEntry = errors.New("unsupported archive entry type")
)
