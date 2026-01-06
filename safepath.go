package blobber

// PathValidator validates paths for security concerns.
// This interface is implemented by internal/safepath.
type PathValidator interface {
	// ValidatePath checks if a path is safe (no traversal, valid characters).
	// Returns ErrPathTraversal if the path is unsafe.
	ValidatePath(path string) error

	// ValidateExtraction checks if extracting the given entries to destDir is safe.
	// Returns ErrPathTraversal for unsafe paths, ErrExtractLimits if limits exceeded.
	ValidateExtraction(destDir string, entries []TOCEntry, limits ExtractLimits) error

	// ValidateSymlink checks if a symlink target is safe (stays within destDir).
	// Returns ErrPathTraversal if the symlink would escape.
	ValidateSymlink(destDir, linkPath, target string) error
}
