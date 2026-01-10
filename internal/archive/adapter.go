package archive

import (
	"log/slog"

	"github.com/meigma/blobber/core"
)

// BuilderAdapter provides the archive builder surface for blobber clients.
type BuilderAdapter struct {
	*Builder
}

// Compile-time interface implementation check.
var _ core.ArchiveBuilder = (*BuilderAdapter)(nil)

// NewBuilderAdapter creates a new adapter for the archive builder.
func NewBuilderAdapter(logger *slog.Logger) *BuilderAdapter {
	return &BuilderAdapter{Builder: NewBuilder(logger)}
}

// ReaderAdapter provides the archive reader surface for blobber clients.
type ReaderAdapter struct {
	*Reader
}

// Compile-time interface implementation check.
var _ core.ArchiveReader = (*ReaderAdapter)(nil)

// NewReaderAdapter creates a new adapter for the archive reader.
func NewReaderAdapter() *ReaderAdapter {
	return &ReaderAdapter{Reader: NewReader()}
}
