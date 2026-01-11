// Package tar provides interfaces for creating and extracting tar archives.
//
// This package defines contracts for tar operations without any compression
// awareness. Compression handling is the responsibility of callers.
package tar

//go:generate go run github.com/matryer/moq@latest -out mocks/extractor.go -pkg mocks . Extractor
//go:generate go run github.com/matryer/moq@latest -out mocks/path_validator.go -pkg mocks . PathValidator
//go:generate go run github.com/matryer/moq@latest -out mocks/writer.go -pkg mocks . Writer

import (
	"context"
	"io"
	"io/fs"
)

// Extractor extracts tar archives to the filesystem.
type Extractor interface {
	// Extract reads tar entries from r and writes them to destDir.
	// The implementation is responsible for path validation and limits.
	Extract(ctx context.Context, r io.Reader, destDir string) error
}

// PathValidator validates paths during tar extraction.
type PathValidator interface {
	// ValidatePath checks if a path is safe for extraction.
	// Returns an error if the path contains traversal sequences,
	// null bytes, absolute paths, or other unsafe patterns.
	ValidatePath(path string) error

	// ValidateSymlink checks if a symlink target is safe.
	// The linkPath is the path of the symlink within the archive.
	// The target is the symlink's target path.
	// Returns an error if the symlink would escape destDir.
	ValidateSymlink(destDir, linkPath, target string) error
}

// Writer creates tar archives from filesystem sources.
type Writer interface {
	// WriteTo writes a tar archive containing all entries from src to dst.
	// The caller is responsible for closing dst.
	//
	// If src implements [fs.ReadLinkFS], symlinks are preserved in the archive
	// using Lstat and ReadLink. If src does not implement [fs.ReadLinkFS] and
	// a symlink is encountered, WriteTo returns an error.
	WriteTo(ctx context.Context, dst io.Writer, src fs.FS) error
}
