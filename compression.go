package blobber

import (
	"github.com/containerd/stargz-snapshotter/estargz"
)

// Compression provides compression/decompression for eStargz blobs.
// This is a type alias for estargz.Compression, allowing custom implementations.
//
// Use GzipCompression or ZstdCompression for built-in implementations.
type Compression = estargz.Compression

// GzipCompression returns gzip compression (default).
func GzipCompression() Compression {
	panic("not implemented")
}

// ZstdCompression returns zstd compression.
func ZstdCompression() Compression {
	panic("not implemented")
}
