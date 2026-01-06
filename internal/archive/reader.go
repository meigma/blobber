package archive

import (
	"fmt"
	"io"

	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/containerd/stargz-snapshotter/estargz/zstdchunked"

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

// ReadTOC extracts the TOC from an eStargz blob.
// The size parameter is the total blob size (needed for footer location).
func (r *reader) ReadTOC(ra io.ReaderAt, size int64) (*blobber.TOC, error) {
	sr := io.NewSectionReader(ra, 0, size)

	// Support both gzip and zstd compressed archives
	esr, err := estargz.Open(sr,
		estargz.WithDecompressors(&zstdchunked.Decompressor{}),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", blobber.ErrInvalidArchive, err)
	}

	var entries []blobber.TOCEntry

	// Get root entry
	root, ok := esr.Lookup("")
	if !ok {
		// Empty archive or no root
		return &blobber.TOC{Entries: entries}, nil
	}

	// Recursively collect all entries, omitting synthetic root (Name == "")
	var collect func(e *estargz.TOCEntry)
	collect = func(e *estargz.TOCEntry) {
		// Omit root entry to match List/Walk expectations
		if e.Name != "" {
			entries = append(entries, convertTOCEntry(e))
		}
		if e.Type == "dir" {
			e.ForeachChild(func(baseName string, child *estargz.TOCEntry) bool {
				collect(child)
				return true // continue
			})
		}
	}
	collect(root)

	return &blobber.TOC{Entries: entries}, nil
}

// OpenFile returns a reader for a specific file within an eStargz blob.
// The size parameter is the total blob size (needed for estargz.Open).
//
//nolint:gocritic // hugeParam: entry passed by value to match blobber.ArchiveReader interface
func (r *reader) OpenFile(ra io.ReaderAt, size int64, entry blobber.TOCEntry) (io.Reader, error) {
	sr := io.NewSectionReader(ra, 0, size)

	// Support both gzip and zstd compressed archives
	esr, err := estargz.Open(sr,
		estargz.WithDecompressors(&zstdchunked.Decompressor{}),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", blobber.ErrInvalidArchive, err)
	}

	fileRA, err := esr.OpenFile(entry.Name)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", entry.Name, err)
	}

	// Wrap io.ReaderAt as io.Reader limited to entry.Size
	return io.NewSectionReader(fileRA, 0, entry.Size), nil
}

// convertTOCEntry converts an estargz.TOCEntry to blobber.TOCEntry.
func convertTOCEntry(e *estargz.TOCEntry) blobber.TOCEntry {
	return blobber.TOCEntry{
		Name:       e.Name,
		Type:       e.Type,
		Size:       e.Size,
		Mode:       e.Mode,
		Offset:     e.Offset,
		LinkName:   e.LinkName,
		ChunkSize:  e.ChunkSize,
		ChunkCount: 0, // Derived from chunk entries if needed, not NumLink
	}
}
