// Package archive provides the Blobber archive format and index tooling.
package archive

import "time"

const (
	magic      = "BLBRIDX\x00"
	version    = 1
	headerSize = 128
)

// Layout selects the index layout.
type Layout uint8

const (
	// LayoutA uses a sorted entry table with binary search.
	LayoutA Layout = iota
	// LayoutB uses a sorted entry table plus a hash table for lookups.
	LayoutB
)

// EntryType identifies the kind of entry in the archive.
type EntryType uint8

const (
	EntryFile EntryType = iota
	EntryDir
	EntrySymlink
)

// Compression identifies the per-file compression used.
type Compression uint8

const (
	CompressionNone Compression = iota
	CompressionZstd
	CompressionGzip
)

// Entry describes a single path in the archive.
type Entry struct {
	Path        string
	Type        EntryType
	Mode        uint32
	MTime       time.Time
	LinkTarget  string
	DataOffset  uint64
	CompSize    uint64
	RawSize     uint64
	Hash        [32]byte
	Compression Compression
}

// BuildResult summarizes a build operation.
type BuildResult struct {
	IndexSize  uint64
	DataSize   uint64
	EntryCount uint32
}
