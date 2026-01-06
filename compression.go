package blobber

import "github.com/gilmanlab/blobber/core"

// Compression provides compression/decompression for eStargz blobs.
// This is a type alias for estargz.Compression, allowing custom implementations.
//
// Use GzipCompression or ZstdCompression for built-in implementations.
type Compression = core.Compression

// GzipCompression returns gzip compression (default).
func GzipCompression() Compression {
	return core.GzipCompression()
}

// ZstdCompression returns zstd compression.
func ZstdCompression() Compression {
	return core.ZstdCompression()
}
