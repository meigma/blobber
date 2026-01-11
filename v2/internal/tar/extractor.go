package tar

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"strings"
)

// Compile-time interface check.
var _ Extractor = (*extractor)(nil)

// Limits configures extraction limits.
type Limits struct {
	MaxFiles     int   // Maximum number of entries including files, dirs, symlinks (0 = unlimited)
	MaxFileSize  int64 // Maximum size per file in bytes (0 = unlimited)
	MaxTotalSize int64 // Maximum total extracted size in bytes (0 = unlimited)
}

// EntryFilter determines whether an entry should be extracted.
// Return true to extract the entry, false to skip it.
// The path is the entry's name from the tar header.
// The isDir flag indicates whether the entry is a directory.
type EntryFilter func(path string, isDir bool) bool

// ExtractorOption configures an extractor.
type ExtractorOption func(*extractor)

// WithExtractorLogger sets the logger for the extractor.
func WithExtractorLogger(logger *slog.Logger) ExtractorOption {
	return func(e *extractor) {
		e.logger = logger
	}
}

// WithLimits sets extraction limits.
func WithLimits(limits Limits) ExtractorOption {
	return func(e *extractor) {
		e.limits = limits
	}
}

// WithEntryFilter sets an entry filter.
// Entries for which the filter returns false are skipped.
func WithEntryFilter(filter EntryFilter) ExtractorOption {
	return func(e *extractor) {
		e.filter = filter
	}
}

type extractor struct {
	logger    *slog.Logger
	validator PathValidator
	limits    Limits
	filter    EntryFilter
}

// NewExtractor creates a new Extractor with the given validator and options.
func NewExtractor(validator PathValidator, opts ...ExtractorOption) *extractor {
	e := &extractor{
		validator: validator,
	}
	for _, opt := range opts {
		opt(e)
	}
	if e.logger == nil {
		e.logger = slog.New(slog.DiscardHandler)
	}
	return e
}

// Extract reads tar entries from r and writes them to destDir.
func (e *extractor) Extract(ctx context.Context, r io.Reader, destDir string) error {
	root, err := os.OpenRoot(destDir)
	if err != nil {
		return fmt.Errorf("open root %s: %w", destDir, err)
	}
	defer root.Close()

	tr := tar.NewReader(r)
	state := &extractState{
		limits:      e.limits,
		createdDirs: make(map[string]struct{}),
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}

		if err := e.processEntry(ctx, root, destDir, header, tr, state); err != nil {
			return err
		}
	}

	return nil
}

// extractState tracks mutable state during a single extraction operation.
type extractState struct {
	limits      Limits
	fileCount   int
	totalSize   int64
	createdDirs map[string]struct{} // tracks dirs created by this extraction
}

func (e *extractor) processEntry(ctx context.Context, root *os.Root, destDir string, header *tar.Header, tr *tar.Reader, state *extractState) error {
	path := header.Name
	isDir := header.Typeflag == tar.TypeDir

	// Apply filter if configured.
	if e.filter != nil && !e.filter(path, isDir) {
		e.logger.Debug("skipped by filter", "path", path)
		return e.discardContent(ctx, header, tr)
	}

	// Validate path lexically.
	if err := e.validator.ValidatePath(path); err != nil {
		return err
	}

	// Check entry count limit (applies to all entry types).
	if err := e.checkEntryLimit(state); err != nil {
		return err
	}

	switch header.Typeflag {
	case tar.TypeDir:
		return e.extractDir(root, header, state)

	//nolint:staticcheck // TypeRegA is deprecated but needed for old archives
	case tar.TypeReg, tar.TypeRegA:
		if err := e.checkFileLimits(header, state); err != nil {
			return err
		}
		return e.extractFile(ctx, root, header, tr, state)

	case tar.TypeSymlink:
		if err := e.validator.ValidateSymlink(destDir, path, header.Linkname); err != nil {
			return err
		}
		return e.extractSymlink(root, header, state)

	default:
		return fmt.Errorf("%w: %s has type %c", ErrUnsupportedEntry, path, header.Typeflag)
	}
}

func (e *extractor) discardContent(ctx context.Context, header *tar.Header, tr *tar.Reader) error {
	//nolint:staticcheck // TypeRegA is deprecated but needed for old archives
	if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
		return nil
	}
	if err := copyWithContext(ctx, io.Discard, tr); err != nil {
		return fmt.Errorf("discard content %s: %w", header.Name, err)
	}
	return nil
}

func (e *extractor) checkEntryLimit(state *extractState) error {
	state.fileCount++
	if e.limits.MaxFiles > 0 && state.fileCount > e.limits.MaxFiles {
		return fmt.Errorf("%w: exceeded max entries (%d)", ErrLimitExceeded, e.limits.MaxFiles)
	}
	return nil
}

func (e *extractor) checkFileLimits(header *tar.Header, state *extractState) error {
	if header.Size < 0 {
		return fmt.Errorf("%w: negative file size", ErrLimitExceeded)
	}

	if e.limits.MaxFileSize > 0 && header.Size > e.limits.MaxFileSize {
		return fmt.Errorf("%w: %s size %d exceeds max %d",
			ErrLimitExceeded, header.Name, header.Size, e.limits.MaxFileSize)
	}

	if state.totalSize > math.MaxInt64-header.Size {
		return fmt.Errorf("%w: total size overflow", ErrLimitExceeded)
	}
	state.totalSize += header.Size

	if e.limits.MaxTotalSize > 0 && state.totalSize > e.limits.MaxTotalSize {
		return fmt.Errorf("%w: total size %d exceeds max %d",
			ErrLimitExceeded, state.totalSize, e.limits.MaxTotalSize)
	}

	return nil
}

func (e *extractor) extractDir(root *os.Root, header *tar.Header, state *extractState) error {
	path := normalizePath(header.Name)
	//nolint:gosec // G115: Mode from tar header is trusted after validation
	mode := fs.FileMode(header.Mode)

	if err := mkdirAll(root, path, mode, state); err != nil {
		return fmt.Errorf("mkdir %s: %w", path, err)
	}

	// Only chmod directories created by this extraction.
	// Skip pre-existing directories to avoid changing their permissions.
	if _, created := state.createdDirs[path]; created {
		if err := root.Chmod(path, mode); err != nil {
			return fmt.Errorf("chmod %s: %w", path, err)
		}
	}

	e.logger.Debug("created directory", "path", path)
	return nil
}

func (e *extractor) extractFile(ctx context.Context, root *os.Root, header *tar.Header, tr *tar.Reader, state *extractState) error {
	path := header.Name

	// Ensure parent directory exists.
	if dir := parentPath(path); dir != "" {
		if err := mkdirAll(root, dir, 0o755, state); err != nil {
			return fmt.Errorf("mkdir parent %s: %w", dir, err)
		}
	}

	// Open with O_EXCL to fail if file exists.
	//nolint:gosec // G115: Mode from tar header is trusted after validation
	f, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fs.FileMode(header.Mode))
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}

	copyErr := copyWithContext(ctx, f, tr)
	closeErr := f.Close()

	if copyErr != nil {
		return fmt.Errorf("write %s: %w", path, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", path, closeErr)
	}

	e.logger.Debug("extracted file", "path", path, "size", header.Size)
	return nil
}

func (e *extractor) extractSymlink(root *os.Root, header *tar.Header, state *extractState) error {
	path := header.Name
	target := header.Linkname

	// Ensure parent directory exists.
	if dir := parentPath(path); dir != "" {
		if err := mkdirAll(root, dir, 0o755, state); err != nil {
			return fmt.Errorf("mkdir parent %s: %w", dir, err)
		}
	}

	if err := root.Symlink(target, path); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", path, target, err)
	}

	e.logger.Debug("created symlink", "path", path, "target", target)
	return nil
}

// mkdirAll creates a directory and all its parents within root.
// Created directories are tracked in state.createdDirs to distinguish them
// from pre-existing directories. This allows extractDir to apply tar header
// permissions only to directories created during extraction, preserving the
// permissions of pre-existing directories.
func mkdirAll(root *os.Root, path string, perm fs.FileMode, state *extractState) error {
	parts := strings.Split(path, "/")
	var current string

	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}

		if current == "" {
			current = part
		} else {
			current = current + "/" + part
		}

		if _, err := root.Stat(current); err == nil {
			continue // already exists
		}

		if err := root.Mkdir(current, perm); err != nil {
			if os.IsExist(err) {
				continue // lost race, another process created it
			}
			return err
		}
		state.createdDirs[current] = struct{}{}
	}

	return nil
}

// parentPath returns the parent directory of path, or empty string for root-level paths.
func parentPath(path string) string {
	idx := strings.LastIndexByte(path, '/')
	if idx <= 0 {
		return ""
	}
	return path[:idx]
}

// normalizePath returns a canonical form of path by removing trailing slashes
// and collapsing empty/dot segments. This matches the normalization done by mkdirAll.
func normalizePath(path string) string {
	parts := strings.Split(path, "/")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		result = append(result, part)
	}
	return strings.Join(result, "/")
}
