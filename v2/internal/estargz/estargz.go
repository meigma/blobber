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

import "io"

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
