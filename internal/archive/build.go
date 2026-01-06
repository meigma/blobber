// Package archive provides eStargz creation and reading operations.
package archive

import (
	"context"
	"io"
	"io/fs"

	"github.com/gilmanlab/blobber"
)

// Compile-time interface implementation check.
var _ blobber.ArchiveBuilder = (*builder)(nil)

// builder implements blobber.ArchiveBuilder using estargz.
type builder struct{}

// NewBuilder creates a new ArchiveBuilder.
func NewBuilder() *builder {
	return &builder{}
}

// Build creates an eStargz blob from the given filesystem.
func (b *builder) Build(ctx context.Context, src fs.FS, compression blobber.Compression) (io.ReadCloser, string, error) {
	panic("not implemented")
}

func tarFromFS(src fs.FS) (io.Reader, error) {
	panic("not implemented")
}
