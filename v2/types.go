package blobber

import "github.com/meigma/blobber/v2/internal/domain"

// Re-export domain types for public API.
type (
	// PushResult contains the result of a Push operation.
	PushResult = domain.PushResult

	// BlobManifest represents metadata about a pushed blob's manifest.
	BlobManifest = domain.BlobManifest

	// BlobDescriptor contains metadata about the blob itself.
	BlobDescriptor = domain.BlobDescriptor

	// Referrer represents an artifact attached to a manifest.
	Referrer = domain.Referrer
)
