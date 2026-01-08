package blobber

import "github.com/meigma/blobber/core"

// FileEntry represents a file in a remote image.
// Implements fs.DirEntry for use with Walk.
// Re-exported from core package.
type FileEntry = core.FileEntry
