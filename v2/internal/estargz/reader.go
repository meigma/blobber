package estargz

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path"
	"slices"
	"time"

	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/containerd/stargz-snapshotter/estargz/zstdchunked"
)

// Compile-time interface check.
var _ Reader = (*reader)(nil)

// typeDir is the TOC entry type for directories.
const typeDir = "dir"

type reader struct {
	ctx    context.Context
	logger *slog.Logger
	sr     *estargz.Reader
}

// ReaderOption configures a Reader.
type ReaderOption func(*readerConfig)

type readerConfig struct {
	logger *slog.Logger
}

// WithReaderLogger sets the logger for the reader.
func WithReaderLogger(logger *slog.Logger) ReaderOption {
	return func(c *readerConfig) {
		c.logger = logger
	}
}

// NewReader creates a new Reader from a SizedReaderAt.
//
// The TOC is parsed eagerly. If parsing fails, an error is returned.
// The provided context is stored and used for all subsequent read operations.
func NewReader(ctx context.Context, src SizedReaderAt, opts ...ReaderOption) (Reader, error) {
	cfg := &readerConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.logger == nil {
		cfg.logger = slog.New(slog.DiscardHandler)
	}

	// Create a section reader for the estargz library.
	sr := io.NewSectionReader(src, 0, src.Size())

	// Parse the TOC eagerly.
	// Include zstd decompressor to support both gzip and zstd compression.
	esr, err := estargz.Open(sr, estargz.WithDecompressors(new(zstdchunked.Decompressor)))
	if err != nil {
		return nil, fmt.Errorf("parse estargz: %w", err)
	}

	cfg.logger.Debug("opened estargz reader", "size", src.Size())

	return &reader{
		ctx:    ctx,
		logger: cfg.logger,
		sr:     esr,
	}, nil
}

// Open opens the named file.
func (r *reader) Open(name string) (fs.File, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}

	normalized, err := validatePath(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	entry, ok := r.sr.Lookup(normalized)
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	if entry.Type == typeDir {
		return &dir{
			reader: r,
			name:   name, // Store original path for ReadDir calls
			entry:  entry,
		}, nil
	}

	// Open file for reading.
	fr, err := r.sr.OpenFile(normalized)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	return &file{
		ctx:    r.ctx,
		name:   name, // Store original path for Stat
		entry:  entry,
		reader: fr,
		pos:    0,
	}, nil
}

// Stat returns a FileInfo describing the named file.
func (r *reader) Stat(name string) (fs.FileInfo, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}

	normalized, err := validatePath(name)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}

	entry, ok := r.sr.Lookup(normalized)
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}

	return &fileInfo{name: path.Base(name), entry: entry}, nil
}

// ReadDir reads the named directory and returns a list of directory entries
// sorted by filename as required by fs.ReadDirFS.
func (r *reader) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}

	normalized, err := validatePath(name)
	if err != nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
	}

	// Look up the directory entry.
	var dirEntry *estargz.TOCEntry
	var ok bool

	if normalized == "" {
		// Root directory.
		dirEntry, ok = r.sr.Lookup("")
		if !ok {
			// Empty archive - return empty list.
			return nil, nil
		}
	} else {
		dirEntry, ok = r.sr.Lookup(normalized)
		if !ok {
			return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
		}
	}

	if dirEntry.Type != typeDir {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: errors.New("not a directory")}
	}

	// Collect direct children using ForeachChild.
	var entries []fs.DirEntry
	dirEntry.ForeachChild(func(baseName string, childEntry *estargz.TOCEntry) bool {
		entries = append(entries, &dirEntryImpl{
			name:  baseName,
			entry: childEntry,
		})
		return true // continue
	})

	// Sort by filename as required by fs.ReadDirFS.
	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		if a.Name() < b.Name() {
			return -1
		}
		if a.Name() > b.Name() {
			return 1
		}
		return 0
	})

	return entries, nil
}

// Close closes the reader.
func (r *reader) Close() error {
	// The estargz.Reader doesn't have a Close method,
	// but we might need cleanup in the future.
	return nil
}

// validatePath checks if name is valid according to fs.ValidPath semantics
// and returns the normalized path for internal lookup.
// Returns an error for invalid paths.
func validatePath(name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", fs.ErrInvalid
	}
	// "." is valid and maps to root.
	if name == "." {
		return "", nil
	}
	return name, nil
}

// file implements fs.File for regular files.
type file struct {
	ctx    context.Context
	name   string
	entry  *estargz.TOCEntry
	reader io.ReaderAt
	pos    int64
}

func (f *file) Stat() (fs.FileInfo, error) {
	return &fileInfo{name: path.Base(f.name), entry: f.entry}, nil
}

func (f *file) Read(p []byte) (int, error) {
	if err := f.ctx.Err(); err != nil {
		return 0, err
	}

	if f.pos >= f.entry.Size {
		return 0, io.EOF
	}

	n, err := f.reader.ReadAt(p, f.pos)
	f.pos += int64(n)

	// Convert EOF at end of file.
	if err == io.EOF && f.pos < f.entry.Size {
		err = nil
	}

	return n, err
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = f.pos + offset
	case io.SeekEnd:
		newPos = f.entry.Size + offset
	default:
		return 0, errors.New("invalid whence")
	}

	if newPos < 0 {
		return 0, errors.New("negative position")
	}

	f.pos = newPos
	return f.pos, nil
}

func (f *file) Close() error {
	return nil
}

// dir implements fs.File for directories.
type dir struct {
	reader  *reader
	name    string
	entry   *estargz.TOCEntry
	entries []fs.DirEntry
	offset  int
}

func (d *dir) Stat() (fs.FileInfo, error) {
	return &fileInfo{name: path.Base(d.name), entry: d.entry}, nil
}

func (d *dir) Read(p []byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: errors.New("is a directory")}
}

func (d *dir) ReadDir(n int) ([]fs.DirEntry, error) {
	// Check context on every call.
	if err := d.reader.ctx.Err(); err != nil {
		return nil, err
	}

	// Lazy load entries on first call.
	if d.entries == nil {
		entries, err := d.reader.ReadDir(d.name)
		if err != nil {
			return nil, err
		}
		d.entries = entries
	}

	if n <= 0 {
		// Return all remaining entries.
		entries := d.entries[d.offset:]
		d.offset = len(d.entries)
		return entries, nil
	}

	// Return up to n entries.
	remaining := len(d.entries) - d.offset
	if remaining == 0 {
		return nil, io.EOF
	}
	if n > remaining {
		n = remaining
	}

	entries := d.entries[d.offset : d.offset+n]
	d.offset += n
	return entries, nil
}

func (d *dir) Close() error {
	return nil
}

// fileInfo implements fs.FileInfo.
type fileInfo struct {
	name  string
	entry *estargz.TOCEntry
}

func (fi *fileInfo) Name() string       { return fi.name }
func (fi *fileInfo) Size() int64        { return fi.entry.Size }
func (fi *fileInfo) Mode() fs.FileMode  { return fi.entry.Stat().Mode() }
func (fi *fileInfo) ModTime() time.Time { return fi.entry.ModTime() }
func (fi *fileInfo) IsDir() bool        { return fi.entry.Type == typeDir }
func (fi *fileInfo) Sys() any           { return fi.entry }

// dirEntryImpl implements fs.DirEntry.
type dirEntryImpl struct {
	name  string
	entry *estargz.TOCEntry
}

func (de *dirEntryImpl) Name() string      { return de.name }
func (de *dirEntryImpl) IsDir() bool       { return de.entry.Type == typeDir }
func (de *dirEntryImpl) Type() fs.FileMode { return de.entry.Stat().Mode().Type() }
func (de *dirEntryImpl) Info() (fs.FileInfo, error) {
	return &fileInfo{name: de.name, entry: de.entry}, nil
}
