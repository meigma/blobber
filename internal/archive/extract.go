package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/klauspost/compress/zstd"

	"github.com/meigma/blobber/core"
	"github.com/meigma/blobber/internal/contracts"
)

// Extract extracts an eStargz blob to the destination directory.
//
// The function auto-detects the compression format (gzip or zstd) from the
// stream's magic bytes. It validates all paths using the provided validator
// and enforces the extraction limits.
//
// Note: This implementation uses best-effort TOCTOU prevention via Lstat
// checks and O_EXCL flags. Full openat(2) safety is not yet implemented.
func Extract(ctx context.Context, r io.Reader, destDir string, validator contracts.PathValidator, limits core.ExtractLimits) error {
	// Auto-detect compression
	decompReader, err := detectAndDecompress(r)
	if err != nil {
		return fmt.Errorf("%w: %v", core.ErrInvalidArchive, err)
	}
	defer decompReader.Close()

	tr := tar.NewReader(decompReader)
	state := &extractState{
		limits:        limits,
		buf:           make([]byte, copyBufferSize),
		validatedDirs: make(map[string]struct{}),
		createdDirs:   make(map[string]struct{}),
	}
	if info, err := os.Stat(destDir); err == nil && info.IsDir() {
		state.validatedDirs[destDir] = struct{}{}
		state.createdDirs[destDir] = struct{}{}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: %v", core.ErrInvalidArchive, err)
		}

		if err := processEntry(ctx, destDir, header, tr, validator, state); err != nil {
			return err
		}
	}

	return nil
}

// extractState tracks extraction progress for limit enforcement.
type extractState struct {
	limits        core.ExtractLimits
	fileCount     int
	totalSize     int64
	buf           []byte
	validatedDirs map[string]struct{}
	createdDirs   map[string]struct{}
}

// processEntry handles a single tar entry.
func processEntry(ctx context.Context, destDir string, header *tar.Header, tr *tar.Reader, validator contracts.PathValidator, state *extractState) error {
	// Skip TOC entry (eStargz stores TOC as stargz.index.json)
	if header.Name == "stargz.index.json" {
		return nil
	}

	// Validate path
	if err := validator.ValidatePath(header.Name); err != nil {
		return err
	}

	// Check limits for regular files
	if header.Typeflag == tar.TypeReg {
		if err := checkLimits(header, state); err != nil {
			return err
		}
	}

	// Extract based on type
	switch header.Typeflag {
	case tar.TypeDir:
		return extractDir(destDir, header, state)
	case tar.TypeReg:
		return extractFile(ctx, destDir, header, tr, state)
	case tar.TypeSymlink:
		if err := validator.ValidateSymlink(destDir, header.Name, header.Linkname); err != nil {
			return err
		}
		return extractSymlink(destDir, header, state)
	case tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
		return fmt.Errorf("%w: unsupported entry type %q for %s", core.ErrInvalidArchive, header.Typeflag, header.Name)
	}
	return nil
}

// checkLimits verifies extraction limits for a regular file.
func checkLimits(header *tar.Header, state *extractState) error {
	// Reject negative sizes (malformed tar header)
	if header.Size < 0 {
		return core.ErrExtractLimits
	}

	state.fileCount++
	if state.limits.MaxFiles > 0 && state.fileCount > state.limits.MaxFiles {
		return core.ErrExtractLimits
	}
	if state.limits.MaxFileSize > 0 && header.Size > state.limits.MaxFileSize {
		return core.ErrExtractLimits
	}

	// Check for overflow before adding
	if state.totalSize > math.MaxInt64-header.Size {
		return core.ErrExtractLimits
	}
	state.totalSize += header.Size
	if state.limits.MaxTotalSize > 0 && state.totalSize > state.limits.MaxTotalSize {
		return core.ErrExtractLimits
	}
	return nil
}

// detectAndDecompress auto-detects the compression format and returns a decompressor.
func detectAndDecompress(r io.Reader) (io.ReadCloser, error) {
	// Read first 4 bytes to detect format (zstd magic is 4 bytes)
	buf := make([]byte, 4)
	n, err := io.ReadFull(r, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}

	// Prepend read bytes back
	combined := io.MultiReader(bytes.NewReader(buf[:n]), r)

	// Detect format by magic bytes
	if n >= 2 && buf[0] == 0x1f && buf[1] == 0x8b {
		// gzip magic: 0x1f 0x8b
		return gzip.NewReader(combined)
	}
	if n >= 4 && buf[0] == 0x28 && buf[1] == 0xb5 && buf[2] == 0x2f && buf[3] == 0xfd {
		// zstd magic: 0x28 0xb5 0x2f 0xfd
		decoder, err := zstd.NewReader(combined)
		if err != nil {
			return nil, err
		}
		return decoder.IOReadCloser(), nil
	}

	return nil, errors.New("unknown compression format")
}

// extractDir creates a directory from a tar header.
//
//nolint:gosec // G305: Path validated by caller via PathValidator
func extractDir(destDir string, header *tar.Header, state *extractState) error {
	fullPath := filepath.Join(destDir, header.Name)

	if err := ensureParentDir(parentDir(fullPath), destDir, state); err != nil {
		return err
	}

	//nolint:gosec // G115: Mode from trusted tar header, G301: dir perms from archive
	return mkdirAllCached(fullPath, fs.FileMode(header.Mode), state)
}

// extractFile extracts a regular file from a tar stream.
//
//nolint:gosec // G305: Path validated by caller via PathValidator
func extractFile(ctx context.Context, destDir string, header *tar.Header, tr *tar.Reader, state *extractState) error {
	fullPath := filepath.Join(destDir, header.Name)

	parent := parentDir(fullPath)
	if err := ensureParentDir(parent, destDir, state); err != nil {
		return err
	}

	// Open with O_EXCL to fail if exists (prevents race with symlink creation)
	//nolint:gosec // G304: Path validated by caller, G115: mode from tar header
	f, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fs.FileMode(header.Mode))
	if err != nil {
		return err
	}

	// Copy content with context cancellation support
	copyErr := copyWithContext(ctx, f, tr, state.buf)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// extractSymlink creates a symlink from a tar header.
//
//nolint:gosec // G305: Path validated by caller via PathValidator
func extractSymlink(destDir string, header *tar.Header, state *extractState) error {
	fullPath := filepath.Join(destDir, header.Name)

	parent := parentDir(fullPath)
	if err := ensureParentDir(parent, destDir, state); err != nil {
		return err
	}

	// Create symlink in temp location then rename atomically.
	// This avoids a race between Remove and Symlink where an attacker
	// could place their own symlink in the gap.
	tmpLink := fullPath + ".tmp"
	_ = os.Remove(tmpLink) // Clean up any stale temp file

	if err := os.Symlink(header.Linkname, tmpLink); err != nil {
		return err
	}

	if err := os.Rename(tmpLink, fullPath); err != nil {
		_ = os.Remove(tmpLink) // Clean up on failure
		return err
	}

	return nil
}

// validateNotSymlink checks that a path is not a symlink.
// This is a best-effort TOCTOU check - not fully race-safe.
func validateNotSymlink(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil // Will be created
	}
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return core.ErrPathTraversal
	}
	return nil
}

func ensureParentDir(parent, destDir string, state *extractState) error {
	// Only validate symlinks for paths within the destination directory.
	// Pre-existing symlinks in the filesystem path leading to destDir (e.g., /tmp -> private/tmp)
	// are safe and should not be rejected.
	if isWithinOrEqual(parent, destDir) {
		if err := validateNotSymlinkCached(parent, state); err != nil {
			return err
		}
	}
	return mkdirAllCached(parent, 0o750, state)
}

// isWithinOrEqual reports whether path is lexically within or equal to dir.
func isWithinOrEqual(path, dir string) bool {
	if path == dir {
		return true
	}
	// Ensure dir ends with separator for proper prefix matching
	if !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	return strings.HasPrefix(path, dir)
}

func validateNotSymlinkCached(path string, state *extractState) error {
	if _, ok := state.validatedDirs[path]; ok {
		return nil
	}
	if err := validateNotSymlink(path); err != nil {
		return err
	}
	state.validatedDirs[path] = struct{}{}
	return nil
}

func mkdirAllCached(path string, mode fs.FileMode, state *extractState) error {
	if _, ok := state.createdDirs[path]; ok {
		return nil
	}
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	state.createdDirs[path] = struct{}{}
	state.validatedDirs[path] = struct{}{}
	return nil
}

func parentDir(path string) string {
	if runtime.GOOS == osWindows {
		return filepath.Dir(path)
	}
	idx := strings.LastIndexByte(path, os.PathSeparator)
	if idx == -1 {
		return "."
	}
	if idx == 0 {
		return string(os.PathSeparator)
	}
	return path[:idx]
}
