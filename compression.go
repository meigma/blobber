package blobber

import (
	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/containerd/stargz-snapshotter/estargz/zstdchunked"
	"github.com/klauspost/compress/zstd"
)

// Compression provides compression/decompression for eStargz blobs.
// This is a type alias for estargz.Compression, allowing custom implementations.
//
// Use GzipCompression or ZstdCompression for built-in implementations.
type Compression = estargz.Compression

// gzipCompression implements estargz.Compression for gzip.
type gzipCompression struct {
	*estargz.GzipCompressor
	estargz.GzipDecompressor
}

// GzipCompression returns gzip compression (default).
func GzipCompression() Compression {
	return &gzipCompression{
		GzipCompressor:   estargz.NewGzipCompressor(),
		GzipDecompressor: estargz.GzipDecompressor{},
	}
}

// zstdCompression implements estargz.Compression for zstd.
type zstdCompression struct {
	*zstdchunked.Compressor
	zstdchunked.Decompressor
}

// ZstdCompression returns zstd compression.
func ZstdCompression() Compression {
	return &zstdCompression{
		Compressor: &zstdchunked.Compressor{
			CompressionLevel: zstd.SpeedDefault,
		},
		Decompressor: zstdchunked.Decompressor{},
	}
}
