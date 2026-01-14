package archive

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"sort"

	"github.com/klauspost/compress/zstd"
)

// BuilderOption configures a Builder.
type BuilderOption func(*Builder)

// WithLayout sets the index layout.
func WithLayout(layout Layout) BuilderOption {
	return func(b *Builder) {
		b.layout = layout
	}
}

// WithCompression sets the default compression for added files.
func WithCompression(c Compression) BuilderOption {
	return func(b *Builder) {
		b.compression = c
	}
}

// WithDataWriter sets the data blob writer.
func WithDataWriter(w io.Writer) BuilderOption {
	return func(b *Builder) {
		b.dataWriter = w
	}
}

// Builder constructs index and data blobs.
//
// The zero value is ready to use but discards data unless a data writer is set.
type Builder struct {
	layout      Layout
	compression Compression
	dataWriter  io.Writer
	counter     *countingWriter
	entries     []buildEntry
	paths       map[string]struct{}
	finalized   bool
}

// NewBuilder creates a Builder with the given options.
func NewBuilder(opts ...BuilderOption) *Builder {
	b := &Builder{
		layout:      LayoutA,
		compression: CompressionNone,
		paths:       make(map[string]struct{}),
	}
	for _, opt := range opts {
		opt(b)
	}
	if b.dataWriter == nil {
		b.dataWriter = io.Discard
	}
	b.counter = &countingWriter{w: b.dataWriter}
	return b
}

// AddFile adds a regular file entry and writes its payload to the data blob.
func (b *Builder) AddFile(path string, info fs.FileInfo, r io.Reader) error {
	if b.finalized {
		return ErrInvalidIndex
	}
	if info == nil {
		return fmt.Errorf("file info: %w", ErrInvalidIndex)
	}
	if info.IsDir() {
		return fmt.Errorf("file info: %w", ErrInvalidIndex)
	}
	clean, err := normalizePath(path)
	if err != nil {
		return err
	}
	if _, ok := b.paths[clean]; ok {
		return ErrDuplicatePath
	}

	start := b.counter.count
	hasher := sha256.New()
	tee := io.TeeReader(r, hasher)

	var out io.Writer = b.counter
	var closer io.Closer

	switch b.compression {
	case CompressionNone:
		// No wrapper needed.
	case CompressionGzip:
		gz := gzip.NewWriter(b.counter)
		out = gz
		closer = gz
	case CompressionZstd:
		zw, err := zstd.NewWriter(b.counter)
		if err != nil {
			return fmt.Errorf("zstd writer: %w", err)
		}
		out = zw
		closer = zw
	default:
		return ErrUnsupportedCompression
	}

	rawSize, err := io.Copy(out, tee)
	if closeErr := closeWriter(closer); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}

	var sum [32]byte
	copy(sum[:], hasher.Sum(nil))

	entry := buildEntry{
		path:        clean,
		typeFlag:    EntryFile,
		mode:        uint32(info.Mode().Perm()),
		mtime:       info.ModTime().Unix(),
		dataOffset:  start,
		compSize:    b.counter.count - start,
		rawSize:     uint64(rawSize),
		hash:        sum,
		compression: b.compression,
	}

	b.entries = append(b.entries, entry)
	b.paths[clean] = struct{}{}
	return nil
}

// AddDir adds a directory entry.
func (b *Builder) AddDir(path string, info fs.FileInfo) error {
	if b.finalized {
		return ErrInvalidIndex
	}
	if info == nil {
		return fmt.Errorf("dir info: %w", ErrInvalidIndex)
	}
	clean, err := normalizePath(path)
	if err != nil {
		return err
	}
	if _, ok := b.paths[clean]; ok {
		return ErrDuplicatePath
	}

	entry := buildEntry{
		path:     clean,
		typeFlag: EntryDir,
		mode:     uint32(info.Mode().Perm()),
		mtime:    info.ModTime().Unix(),
	}

	b.entries = append(b.entries, entry)
	b.paths[clean] = struct{}{}
	return nil
}

// AddSymlink adds a symlink entry with its target.
func (b *Builder) AddSymlink(path, target string, info fs.FileInfo) error {
	if b.finalized {
		return ErrInvalidIndex
	}
	if info == nil {
		return fmt.Errorf("symlink info: %w", ErrInvalidIndex)
	}
	clean, err := normalizePath(path)
	if err != nil {
		return err
	}
	if _, ok := b.paths[clean]; ok {
		return ErrDuplicatePath
	}
	if target == "" {
		return ErrInvalidPath
	}

	entry := buildEntry{
		path:       clean,
		linkTarget: target,
		typeFlag:   EntrySymlink,
		mode:       uint32(info.Mode().Perm()),
		mtime:      info.ModTime().Unix(),
	}

	b.entries = append(b.entries, entry)
	b.paths[clean] = struct{}{}
	return nil
}

// Finalize writes the index blob to dst and returns build statistics.
func (b *Builder) Finalize(dst io.Writer) (*BuildResult, error) {
	if b.finalized {
		return nil, ErrInvalidIndex
	}
	b.finalized = true

	entries := make([]buildEntry, len(b.entries))
	copy(entries, b.entries)
	if err := sortEntries(entries); err != nil {
		return nil, err
	}

	stringsBuf := &bytes.Buffer{}
	for i := range entries {
		entries[i].pathOff = uint32(stringsBuf.Len())
		stringsBuf.WriteString(entries[i].path)
		entries[i].pathLen = uint32(len(entries[i].path))

		if entries[i].typeFlag == EntrySymlink {
			entries[i].linkOff = uint32(stringsBuf.Len())
			stringsBuf.WriteString(entries[i].linkTarget)
			entries[i].linkLen = uint32(len(entries[i].linkTarget))
		}

		if b.layout == LayoutB {
			entries[i].pathHash = fnv1a64([]byte(entries[i].path))
		}
	}

	entriesBytes, err := marshalEntries(entries, b.layout)
	if err != nil {
		return nil, err
	}

	var hashBytes []byte
	var hashSlots uint32
	if b.layout == LayoutB {
		hashBytes, hashSlots, err = buildHashTable(entries)
		if err != nil {
			return nil, err
		}
	}

	indexBytes, err := buildIndex(entriesBytes, stringsBuf.Bytes(), hashBytes, hashSlots, b.layout)
	if err != nil {
		return nil, err
	}

	if _, err := dst.Write(indexBytes); err != nil {
		return nil, err
	}

	return &BuildResult{
		IndexSize:  uint64(len(indexBytes)),
		DataSize:   b.counter.count,
		EntryCount: uint32(len(entries)),
	}, nil
}

type buildEntry struct {
	path        string
	linkTarget  string
	typeFlag    EntryType
	mode        uint32
	mtime       int64
	dataOffset  uint64
	compSize    uint64
	rawSize     uint64
	hash        [32]byte
	compression Compression
	pathOff     uint32
	pathLen     uint32
	linkOff     uint32
	linkLen     uint32
	pathHash    uint64
}

func sortEntries(entries []buildEntry) error {
	if len(entries) == 0 {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].path < entries[j].path
	})

	for i := 1; i < len(entries); i++ {
		if entries[i].path == entries[i-1].path {
			return ErrDuplicatePath
		}
	}

	return nil
}

func closeWriter(w io.Closer) error {
	if w == nil {
		return nil
	}
	return w.Close()
}

type countingWriter struct {
	w     io.Writer
	count uint64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.count += uint64(n)
	return n, err
}
