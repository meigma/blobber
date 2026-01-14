# Archive Package API Sketch

This document outlines a minimal public API for the `archive` module. The API is
intended to expose the custom Blobber archive format so external tools can read
and build blobs without pulling in higher-level Blobber APIs.

This is a sketch only; the final API should remain small and stable.

## Goals

- Read and validate index blobs (spec A or B)
- Lookup entries by path and list entries
- Build index + data blobs from a filesystem or stream inputs
- Expose integrity verification (index checksum, per-file hash)
- Keep the API focused on full-file reads only

## Proposed Package Layout

- `archive/index`:
  - Parsing and querying index blobs
- `archive/build`:
  - Building index and data blobs
- `archive/format`:
  - Shared types, enums, constants

This can be collapsed into a single package if desired.

## Core Types

```go
// EntryType identifies the kind of entry in the archive.
type EntryType uint8

const (
    EntryFile EntryType = 0
    EntryDir  EntryType = 1
    EntrySymlink EntryType = 2
)

// Compression identifies the per-file compression used.
type Compression uint8

const (
    CompressionNone Compression = 0
    CompressionZstd Compression = 1
    CompressionGzip Compression = 2
)

// Entry describes a single path in the archive.
type Entry struct {
    Path        string
    Type        EntryType
    Mode        uint32
    MTime       int64
    LinkTarget  string

    DataOffset  uint64
    CompSize    uint64
    RawSize     uint64
    Hash        [32]byte
    Compression Compression
}
```

## Index Reader API

```go
// Index represents a parsed index blob.
type Index struct {
    // Version and format metadata
}

// OpenIndex parses an index blob from a ReaderAt.
func OpenIndex(r io.ReaderAt, size int64) (*Index, error)

// Lookup returns the entry for a path if it exists.
func (idx *Index) Lookup(path string) (Entry, bool, error)

// Entries returns all entries in sorted path order.
func (idx *Index) Entries() []Entry
```

### Reader invariants

- Paths are normalized and validated on input (no absolute paths, no "..", no NUL).
- Index checksum is verified during OpenIndex.
- Lookup is case-sensitive.

## Builder API

```go
// Builder constructs index + data blobs.
type Builder struct {
    // Tracks entries and emits blob data
}

// NewBuilder configures a builder for spec A or B.
func NewBuilder(opts ...BuilderOption) *Builder

// AddFile adds a regular file entry and streams its payload to the data writer.
func (b *Builder) AddFile(path string, info fs.FileInfo, r io.Reader) error

// AddDir adds a directory entry.
func (b *Builder) AddDir(path string, info fs.FileInfo) error

// AddSymlink adds a symlink entry with its target.
func (b *Builder) AddSymlink(path, target string, info fs.FileInfo) error

// Finalize writes the index blob to dst and returns stats.
func (b *Builder) Finalize(dst io.Writer) (*BuildResult, error)
```

### Builder behavior

- The builder normalizes and validates paths.
- For files, it computes SHA-256 of uncompressed content.
- Compression is per-file and opt-in.
- Data blob is written as files are added; index is emitted at Finalize.

## BuildResult

```go
// BuildResult summarizes the index and data blobs.
type BuildResult struct {
    IndexSize uint64
    DataSize  uint64
    EntryCount uint32
}
```

## Open Questions

- Whether to expose streaming access to data blob reads (e.g., a helper that
  wraps a ranged ReaderAt and verifies per-file hashes).
- Whether to expose optional hash algorithms beyond SHA-256.
- Whether to expose a strict mode for rejecting invalid or unsorted index blobs.
