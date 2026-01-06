package core

import (
	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/containerd/stargz-snapshotter/estargz/zstdchunked"
	"github.com/klauspost/compress/zstd"
)

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
