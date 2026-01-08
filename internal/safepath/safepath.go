// Package safepath provides path validation for secure file extraction.
//
// This package performs lexical validation only. The extraction code must use
// safe filesystem primitives (such as openat(2) with O_NOFOLLOW) to prevent
// TOCTOU races during actual file creation.
package safepath

import (
	"math"
	"path/filepath"
	"strings"

	"github.com/meigma/blobber/core"
)

// Compile-time interface implementation check.
var _ core.PathValidator = (*Validator)(nil)

// Validator implements blobber.PathValidator.
type Validator struct{}

// NewValidator creates a new PathValidator.
func NewValidator() *Validator {
	return &Validator{}
}

// ValidatePath checks if a path is safe (no traversal, valid characters, no volume names).
func (v *Validator) ValidatePath(path string) error {
	if containsNull(path) {
		return core.ErrPathTraversal
	}
	if filepath.VolumeName(path) != "" {
		return core.ErrPathTraversal
	}
	if filepath.IsAbs(path) {
		return core.ErrPathTraversal
	}
	if containsTraversal(path) {
		return core.ErrPathTraversal
	}
	return nil
}

// ValidateExtraction checks if extracting the given entries to destDir is safe.
func (v *Validator) ValidateExtraction(destDir string, entries []core.TOCEntry, limits core.ExtractLimits) error {
	absDestDir, err := filepath.Abs(destDir)
	if err != nil {
		return core.ErrPathTraversal
	}

	var totalSize int64
	var fileCount int
	for i := range entries {
		entry := &entries[i]
		if entryErr := v.validateEntry(absDestDir, entry); entryErr != nil {
			return entryErr
		}
		if entry.Type == "reg" {
			if sizeErr := validateFileSize(entry.Size, limits.MaxFileSize); sizeErr != nil {
				return sizeErr
			}
			totalSize, err = addSize(totalSize, entry.Size)
			if err != nil {
				return err
			}
			fileCount++
		}
	}

	return validateTotals(fileCount, totalSize, limits)
}

// validateEntry checks that a single entry's path is safe.
func (v *Validator) validateEntry(absDestDir string, entry *core.TOCEntry) error {
	if err := v.ValidatePath(entry.Name); err != nil {
		return err
	}
	resolved := filepath.Join(absDestDir, entry.Name)
	if !isWithinDir(resolved, absDestDir) {
		return core.ErrPathTraversal
	}
	return nil
}

// validateFileSize checks a single file's size against limits.
func validateFileSize(size, maxFileSize int64) error {
	if size < 0 {
		return core.ErrExtractLimits
	}
	if maxFileSize > 0 && size > maxFileSize {
		return core.ErrExtractLimits
	}
	return nil
}

// addSize safely adds a size to a running total, checking for overflow.
func addSize(total, size int64) (int64, error) {
	if total > math.MaxInt64-size {
		return 0, core.ErrExtractLimits
	}
	return total + size, nil
}

// validateTotals checks aggregate limits.
func validateTotals(fileCount int, totalSize int64, limits core.ExtractLimits) error {
	if limits.MaxFiles > 0 && fileCount > limits.MaxFiles {
		return core.ErrExtractLimits
	}
	if limits.MaxTotalSize > 0 && totalSize > limits.MaxTotalSize {
		return core.ErrExtractLimits
	}
	return nil
}

// ValidateSymlink checks if a symlink target is safe (stays within destDir).
//
// Absolute symlink targets are rejected outright to prevent symlinks that
// escape the extraction directory. Relative targets are validated to ensure
// they resolve to a path within destDir.
//
// This performs lexical validation only - it does not follow existing symlinks
// on the filesystem.
func (v *Validator) ValidateSymlink(destDir, linkPath, target string) error {
	// Validate linkPath first.
	if err := v.ValidatePath(linkPath); err != nil {
		return err
	}

	// Check target for invalid characters.
	if containsNull(target) {
		return core.ErrPathTraversal
	}

	// Reject absolute symlink targets entirely.
	// These would point outside the extraction directory when dereferenced.
	if filepath.IsAbs(target) || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "\\") {
		return core.ErrPathTraversal
	}

	// Reject targets with Windows volume names (e.g., "C:foo").
	if filepath.VolumeName(target) != "" {
		return core.ErrPathTraversal
	}

	absDestDir, err := filepath.Abs(destDir)
	if err != nil {
		return core.ErrPathTraversal
	}

	// Relative target: resolve relative to the link's directory within destDir.
	linkDir := filepath.Dir(filepath.Join(absDestDir, linkPath))
	targetPath := filepath.Clean(filepath.Join(linkDir, target))

	// Check that the resolved target stays within destDir.
	if !isWithinDir(targetPath, absDestDir) {
		return core.ErrPathTraversal
	}
	return nil
}

// isWithinDir reports whether path is lexically within or equal to dir.
// Both paths should be absolute and clean for reliable results.
func isWithinDir(path, dir string) bool {
	if path == dir {
		return true
	}
	// Special case: if dir is root ("/"), any absolute path is within it.
	if dir == "/" || dir == string(filepath.Separator) {
		return filepath.IsAbs(path)
	}
	if strings.HasSuffix(dir, string(filepath.Separator)) {
		return strings.HasPrefix(path, dir)
	}
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}

// containsNull reports whether path contains a null byte.
// Null bytes in paths are a common attack vector for path injection.
func containsNull(path string) bool {
	return strings.ContainsRune(path, '\x00')
}

// containsTraversal reports whether path contains ".." directory traversal.
// It normalizes both forward and backslash separators to detect traversal
// in mixed-separator archives (common in Windows-created archives).
func containsTraversal(path string) bool {
	normalized := strings.ReplaceAll(path, "\\", "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
