package blobber

import "github.com/gilmanlab/blobber/core"

// Type aliases re-exported from core package.
type (
	// BuildResult contains the output of building an eStargz blob.
	BuildResult = core.BuildResult

	// ArchiveBuilder creates eStargz blobs from files.
	ArchiveBuilder = core.ArchiveBuilder

	// ArchiveReader reads eStargz blobs.
	ArchiveReader = core.ArchiveReader

	// Extractor extracts eStargz archives to the filesystem.
	Extractor = core.Extractor

	// TOC represents the table of contents of an eStargz blob.
	TOC = core.TOC

	// TOCEntry represents a file in the TOC.
	TOCEntry = core.TOCEntry
)
