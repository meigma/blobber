package archive

import (
	"io"

	"github.com/gilmanlab/blobber"
)

// Compile-time interface implementation check.
var _ blobber.ArchiveReader = (*reader)(nil)

// reader implements blobber.ArchiveReader using estargz.
type reader struct{}

// NewReader creates a new ArchiveReader.
func NewReader() *reader {
	return &reader{}
}

// OpenFile returns a reader for a specific file within an eStargz blob.
func (r *reader) OpenFile(ra io.ReaderAt, entry blobber.TOCEntry) (io.Reader, error) {
	panic("not implemented")
}

// ReadTOC extracts the TOC from an eStargz blob.
func (r *reader) ReadTOC(ra io.ReaderAt, size int64) (*blobber.TOC, error) {
	panic("not implemented")
}
