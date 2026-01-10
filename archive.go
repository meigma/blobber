package blobber

import "github.com/meigma/blobber/core"

// Type aliases re-exported from core package.
type (
	// BuildResult contains the output of building an eStargz blob.
	BuildResult = core.BuildResult

	// TOC represents the table of contents of an eStargz blob.
	TOC = core.TOC

	// TOCEntry represents a file in the TOC.
	TOCEntry = core.TOCEntry
)
