package blobber

import "github.com/gilmanlab/blobber/core"

// Type aliases re-exported from core package for cache operations.
type (
	// BlobHandle provides random access to a cached or remote blob.
	// Implements io.ReaderAt for estargz compatibility.
	BlobHandle = core.BlobHandle

	// LayerDescriptor captures the resolved layer metadata plus platform context.
	// Used as the cache key and for blob retrieval operations.
	LayerDescriptor = core.LayerDescriptor

	// BlobSource provides access to blobs, either from cache or network.
	BlobSource = core.BlobSource
)
