// Package validator provides path validation for secure tar extraction.
//
// This package performs lexical validation only. The extraction code must use
// safe filesystem primitives (such as O_EXCL flags and symlink checks) to
// prevent TOCTOU races during actual file creation.
package validator

import (
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/meigma/blobber/v2/internal/tar"
)

// Compile-time interface check.
var _ tar.PathValidator = (*Validator)(nil)

// Option configures a Validator.
type Option func(*Validator)

// Validator implements tar.PathValidator with secure path validation.
type Validator struct {
	logger *slog.Logger
}

// New creates a new Validator with the given options.
func New(opts ...Option) *Validator {
	v := &Validator{}
	for _, opt := range opts {
		opt(v)
	}
	if v.logger == nil {
		v.logger = slog.New(slog.DiscardHandler)
	}
	return v
}

// WithLogger sets the logger for the Validator.
func WithLogger(logger *slog.Logger) Option {
	return func(v *Validator) {
		v.logger = logger
	}
}

// ValidatePath checks if a path is safe for extraction.
func (v *Validator) ValidatePath(path string) error {
	// Null bytes are not checked by filepath.IsLocal.
	if containsNull(path) {
		v.logger.Debug("path contains null byte", "path", path)
		return tar.ErrPathTraversal
	}
	// IsLocal rejects empty paths, absolute paths, paths with volume names,
	// paths containing ".." traversal, and Windows reserved names.
	if !filepath.IsLocal(path) {
		v.logger.Debug("path is not local", "path", path)
		return tar.ErrPathTraversal
	}
	// Windows strips trailing dots and spaces from path segments during file
	// operations. A segment like ".. " or "..  " passes IsLocal but resolves
	// as ".." on Windows, enabling traversal. Check all platforms since
	// archives may be created anywhere and extracted on Windows.
	if containsWindowsTraversal(path) {
		v.logger.Debug("path contains Windows traversal bypass", "path", path)
		return tar.ErrPathTraversal
	}
	return nil
}

// ValidateSymlink checks if a symlink target is safe.
func (v *Validator) ValidateSymlink(destDir, linkPath, target string) error {
	if err := v.ValidatePath(linkPath); err != nil {
		return err
	}

	if containsNull(target) {
		v.logger.Debug("symlink target contains null byte", "link_path", linkPath, "target", target)
		return tar.ErrPathTraversal
	}

	// Reject absolute symlink targets, including Windows UNC paths.
	if filepath.IsAbs(target) || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "\\") {
		v.logger.Debug("symlink target is absolute", "link_path", linkPath, "target", target)
		return tar.ErrPathTraversal
	}

	// Check for Windows traversal bypass in target.
	if containsWindowsTraversal(target) {
		v.logger.Debug("symlink target contains Windows traversal bypass", "link_path", linkPath, "target", target)
		return tar.ErrPathTraversal
	}

	// Reject targets with Windows volume names.
	if filepath.VolumeName(target) != "" {
		v.logger.Debug("symlink target contains volume name", "link_path", linkPath, "target", target)
		return tar.ErrPathTraversal
	}

	absDestDir, err := filepath.Abs(destDir)
	if err != nil {
		v.logger.Debug("failed to resolve dest_dir", "dest_dir", destDir, "error", err)
		return tar.ErrPathTraversal
	}

	// Resolve target relative to the link's directory within destDir.
	linkDir := filepath.Dir(filepath.Join(absDestDir, linkPath))
	targetPath := filepath.Clean(filepath.Join(linkDir, target))

	if !isWithinDir(targetPath, absDestDir) {
		v.logger.Debug("symlink target escapes dest_dir", "link_path", linkPath, "target", target, "resolved_target", targetPath, "dest_dir", absDestDir)
		return tar.ErrPathTraversal
	}
	return nil
}

// containsNull reports whether path contains a null byte.
func containsNull(path string) bool {
	return strings.ContainsRune(path, '\x00')
}

// containsWindowsTraversal checks for path segments that bypass traversal
// detection due to Windows path normalization. Windows strips trailing dots
// and spaces from path segments, so ".. " or "..." resolve as "..".
func containsWindowsTraversal(path string) bool {
	normalized := strings.ReplaceAll(path, "\\", "/")
	for segment := range strings.SplitSeq(normalized, "/") {
		if isWindowsTraversalSegment(segment) {
			return true
		}
	}
	return false
}

// isWindowsTraversalSegment reports whether a path segment would resolve to
// ".." on Windows after trailing dots and spaces are stripped.
func isWindowsTraversalSegment(segment string) bool {
	if !strings.HasPrefix(segment, "..") {
		return false
	}
	// Check if everything after ".." is trailing junk (dots/spaces).
	rest := segment[2:]
	for _, r := range rest {
		if r != '.' && r != ' ' {
			return false
		}
	}
	// Only flag as traversal bypass if there IS trailing junk.
	// Literal ".." is handled by filepath.IsLocal.
	return rest != ""
}

// isWithinDir reports whether path is lexically within or equal to dir.
func isWithinDir(path, dir string) bool {
	if path == dir {
		return true
	}
	// Normalize dir by removing trailing separator (unless it's the root).
	dir = strings.TrimSuffix(dir, string(filepath.Separator))
	if dir == "" {
		// dir was "/" or just the separator; any absolute path is within root.
		return filepath.IsAbs(path)
	}
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}
