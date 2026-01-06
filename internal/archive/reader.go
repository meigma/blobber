package archive

import (
	"fmt"
	"io"

	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/containerd/stargz-snapshotter/estargz/zstdchunked"

	"github.com/gilmanlab/blobber/core"
)

// Compile-time interface implementation check.
var _ core.ArchiveReader = (*Reader)(nil)

// Reader reads eStargz blobs.
type Reader struct{}

// NewReader creates a new Reader.
func NewReader() *Reader {
	return &Reader{}
}

// ReadTOC extracts the TOC from an eStargz blob.
// The size parameter is the total blob size (needed for footer location).
func (r *Reader) ReadTOC(ra io.ReaderAt, size int64) (*core.TOC, error) {
	sr := io.NewSectionReader(ra, 0, size)

	// Support both gzip and zstd compressed archives
	esr, err := estargz.Open(sr,
		estargz.WithDecompressors(&zstdchunked.Decompressor{}),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", core.ErrInvalidArchive, err)
	}

	var entries []core.TOCEntry

	// Get root entry
	root, ok := esr.Lookup("")
	if !ok {
		// Empty archive or no root
		return &core.TOC{Entries: entries}, nil
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

	return &core.TOC{Entries: entries}, nil
}

// OpenFile returns a reader for a specific file within an eStargz blob.
// The size parameter is the total blob size (needed for estargz.Open).
//
//nolint:gocritic // hugeParam: entry passed by value to match core.ArchiveReader interface
func (r *Reader) OpenFile(ra io.ReaderAt, size int64, entry core.TOCEntry) (io.Reader, error) {
	sr := io.NewSectionReader(ra, 0, size)

	// Support both gzip and zstd compressed archives
	esr, err := estargz.Open(sr,
		estargz.WithDecompressors(&zstdchunked.Decompressor{}),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", core.ErrInvalidArchive, err)
	}

	fileRA, err := esr.OpenFile(entry.Name)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", entry.Name, err)
	}

	// Wrap io.ReaderAt as io.Reader limited to entry.Size
	return io.NewSectionReader(fileRA, 0, entry.Size), nil
}

// convertTOCEntry converts an estargz.TOCEntry to core.TOCEntry.
func convertTOCEntry(e *estargz.TOCEntry) core.TOCEntry {
	return core.TOCEntry{
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
