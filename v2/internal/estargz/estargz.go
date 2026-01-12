// Package estargz provides eStargz archive creation and reading operations.
//
// eStargz (seekable tar.gz) is a compression format that enables efficient
// random access to files within a compressed archive. This package provides
// three main capabilities:
//
//   - Builder: Creates eStargz archives from filesystem sources
//   - Reader: Provides fs.FS access to archive contents via TOC-based lookup
//   - Extractor: Extracts full archives to disk using a tar.Extractor
//
// The package is compression-agnostic at the API level, accepting compression
// configuration via functional options.
package estargz

//go:generate go run github.com/matryer/moq@latest -out mocks/builder.go -pkg mocks . Builder
//go:generate go run github.com/matryer/moq@latest -out mocks/reader.go -pkg mocks . Reader
//go:generate go run github.com/matryer/moq@latest -out mocks/extractor.go -pkg mocks . Extractor

import (
	"context"
	"io"
	"io/fs"
)

// Builder creates eStargz archives from filesystem sources.
type Builder interface {
	// Build creates an eStargz archive from the given filesystem.
	// The compressed output is written to dst.
	Build(ctx context.Context, dst io.Writer, src fs.FS) (*BuildResult, error)
}

// Reader provides fs.FS access to an eStargz archive.
//
// The Reader parses the TOC eagerly at creation time and fetches file
// content lazily on Read(). Each Open() call returns an independent
// file handle with its own read position.
type Reader interface {
	fs.FS
	fs.StatFS
	fs.ReadDirFS
	io.Closer
}

// Extractor extracts eStargz archives to the filesystem.
type Extractor interface {
	// Extract decompresses an eStargz archive and extracts it to destDir.
	// The compression format (gzip or zstd) is auto-detected from magic bytes.
	Extract(ctx context.Context, r io.Reader, destDir string) error
}

// SizedReaderAt extends io.ReaderAt with size information.
//
// This interface is required for reading eStargz archives because the TOC
// (Table of Contents) is located at the end of the archive. The size is
// needed to locate the footer which points to the TOC.
//
// For local files, wrap *os.File with the size from Stat().
// For network access, implement using HTTP range requests.
type SizedReaderAt interface {
	io.ReaderAt
	Size() int64
}
