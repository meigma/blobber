package safepath

import (
	"math"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gilmanlab/blobber/core"
)

const osWindows = "windows"

func TestValidator_ValidatePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{
			name:    "simple file",
			path:    "foo.txt",
			wantErr: nil,
		},
		{
			name:    "nested path",
			path:    "foo/bar/baz.txt",
			wantErr: nil,
		},
		{
			name:    "dot prefix",
			path:    "./foo/bar",
			wantErr: nil,
		},
		{
			name:    "single dot component",
			path:    "foo/./bar",
			wantErr: nil,
		},
		{
			name:    "parent traversal at start",
			path:    "../foo",
			wantErr: core.ErrPathTraversal,
		},
		{
			name:    "parent traversal in middle",
			path:    "foo/../bar",
			wantErr: core.ErrPathTraversal,
		},
		{
			name:    "parent traversal at end",
			path:    "foo/bar/..",
			wantErr: core.ErrPathTraversal,
		},
		{
			name:    "absolute path unix",
			path:    "/etc/passwd",
			wantErr: core.ErrPathTraversal,
		},
		{
			name:    "null byte",
			path:    "foo\x00bar",
			wantErr: core.ErrPathTraversal,
		},
		{
			name:    "null byte at end",
			path:    "foo.txt\x00",
			wantErr: core.ErrPathTraversal,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: nil,
		},
		{
			name:    "double dot not as component",
			path:    "foo..bar",
			wantErr: nil,
		},
		{
			name:    "triple dot",
			path:    ".../foo",
			wantErr: nil,
		},
		// Backslash separator traversal (relevant for mixed-separator archives)
		{
			name:    "backslash traversal at start",
			path:    "..\\foo",
			wantErr: core.ErrPathTraversal,
		},
		{
			name:    "backslash traversal in middle",
			path:    "foo\\..\\bar",
			wantErr: core.ErrPathTraversal,
		},
		{
			name:    "mixed slash traversal",
			path:    "foo/..\\bar",
			wantErr: core.ErrPathTraversal,
		},
	}

	// Add Windows-specific tests for volume names/UNC paths.
	if runtime.GOOS == osWindows {
		windowsTests := []struct {
			name    string
			path    string
			wantErr error
		}{
			{
				name:    "windows drive letter",
				path:    "C:\\Windows\\System32",
				wantErr: core.ErrPathTraversal,
			},
			{
				name:    "windows drive letter lowercase",
				path:    "c:\\temp\\file.txt",
				wantErr: core.ErrPathTraversal,
			},
			{
				name:    "windows UNC path",
				path:    "\\\\server\\share\\file.txt",
				wantErr: core.ErrPathTraversal,
			},
			{
				name:    "windows drive relative",
				path:    "C:relative\\path",
				wantErr: core.ErrPathTraversal,
			},
		}
		tests = append(tests, windowsTests...)
	}

	v := NewValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := v.ValidatePath(tt.path)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr, "ValidatePath(%q)", tt.path)
			} else {
				assert.NoError(t, err, "ValidatePath(%q)", tt.path)
			}
		})
	}
}

func TestValidator_ValidateExtraction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		destDir string
		entries []core.TOCEntry
		limits  core.ExtractLimits
		wantErr error
	}{
		{
			name:    "valid extraction no limits",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 100},
				{Name: "dir/file2.txt", Type: "reg", Size: 200},
			},
			limits:  core.ExtractLimits{},
			wantErr: nil,
		},
		{
			name:    "valid extraction within limits",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 100},
				{Name: "file2.txt", Type: "reg", Size: 200},
			},
			limits: core.ExtractLimits{
				MaxFiles:     10,
				MaxTotalSize: 1000,
				MaxFileSize:  500,
			},
			wantErr: nil,
		},
		{
			name:    "exceeds max files",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 100},
				{Name: "file2.txt", Type: "reg", Size: 100},
				{Name: "file3.txt", Type: "reg", Size: 100},
			},
			limits: core.ExtractLimits{
				MaxFiles: 2,
			},
			wantErr: core.ErrExtractLimits,
		},
		{
			name:    "exceeds max total size",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 500},
				{Name: "file2.txt", Type: "reg", Size: 600},
			},
			limits: core.ExtractLimits{
				MaxTotalSize: 1000,
			},
			wantErr: core.ErrExtractLimits,
		},
		{
			name:    "exceeds max file size",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 100},
				{Name: "large.bin", Type: "reg", Size: 1000},
			},
			limits: core.ExtractLimits{
				MaxFileSize: 500,
			},
			wantErr: core.ErrExtractLimits,
		},
		{
			name:    "path traversal in entry",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "../escape.txt", Type: "reg", Size: 100},
			},
			limits:  core.ExtractLimits{},
			wantErr: core.ErrPathTraversal,
		},
		{
			name:    "absolute path in entry",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "/etc/passwd", Type: "reg", Size: 100},
			},
			limits:  core.ExtractLimits{},
			wantErr: core.ErrPathTraversal,
		},
		{
			name:    "directories not counted in file limit",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "dir1", Type: "dir", Size: 0},
				{Name: "dir2", Type: "dir", Size: 0},
				{Name: "dir3", Type: "dir", Size: 0},
				{Name: "file1.txt", Type: "reg", Size: 100},
			},
			limits: core.ExtractLimits{
				MaxFiles: 2,
			},
			wantErr: nil,
		},
		{
			name:    "symlinks not counted in file limit",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "link1", Type: "symlink", LinkName: "file1.txt"},
				{Name: "link2", Type: "symlink", LinkName: "file1.txt"},
				{Name: "file1.txt", Type: "reg", Size: 100},
			},
			limits: core.ExtractLimits{
				MaxFiles: 2,
			},
			wantErr: nil,
		},
		{
			name:    "empty entries",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{},
			limits:  core.ExtractLimits{},
			wantErr: nil,
		},
		{
			name:    "negative size rejected",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: -100},
			},
			limits:  core.ExtractLimits{},
			wantErr: core.ErrExtractLimits,
		},
		{
			name:    "size overflow rejected",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: math.MaxInt64},
				{Name: "file2.txt", Type: "reg", Size: 1},
			},
			limits:  core.ExtractLimits{},
			wantErr: core.ErrExtractLimits,
		},
		{
			name:    "large sizes within int64 range",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: math.MaxInt64 / 2},
				{Name: "file2.txt", Type: "reg", Size: math.MaxInt64 / 2},
			},
			limits:  core.ExtractLimits{},
			wantErr: nil,
		},
		// Boundary condition tests - exactly at limit should pass
		{
			name:    "exactly at max files limit",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 100},
				{Name: "file2.txt", Type: "reg", Size: 100},
			},
			limits: core.ExtractLimits{
				MaxFiles: 2,
			},
			wantErr: nil,
		},
		{
			name:    "exactly at max total size limit",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 500},
				{Name: "file2.txt", Type: "reg", Size: 500},
			},
			limits: core.ExtractLimits{
				MaxTotalSize: 1000,
			},
			wantErr: nil,
		},
		{
			name:    "exactly at max file size limit",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 500},
			},
			limits: core.ExtractLimits{
				MaxFileSize: 500,
			},
			wantErr: nil,
		},
		{
			name:    "one over max files limit",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 100},
				{Name: "file2.txt", Type: "reg", Size: 100},
				{Name: "file3.txt", Type: "reg", Size: 100},
			},
			limits: core.ExtractLimits{
				MaxFiles: 2,
			},
			wantErr: core.ErrExtractLimits,
		},
		{
			name:    "one byte over max total size",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 500},
				{Name: "file2.txt", Type: "reg", Size: 501},
			},
			limits: core.ExtractLimits{
				MaxTotalSize: 1000,
			},
			wantErr: core.ErrExtractLimits,
		},
		{
			name:    "one byte over max file size",
			destDir: "/tmp/extract",
			entries: []core.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 501},
			},
			limits: core.ExtractLimits{
				MaxFileSize: 500,
			},
			wantErr: core.ErrExtractLimits,
		},
	}

	v := NewValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := v.ValidateExtraction(tt.destDir, tt.entries, tt.limits)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateSymlink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		destDir  string
		linkPath string
		target   string
		wantErr  error
	}{
		{
			name:     "relative symlink same directory",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "file.txt",
			wantErr:  nil,
		},
		{
			name:     "relative symlink nested",
			destDir:  "/tmp/extract",
			linkPath: "dir/link",
			target:   "file.txt",
			wantErr:  nil,
		},
		{
			name:     "relative symlink to parent within destDir",
			destDir:  "/tmp/extract",
			linkPath: "dir/subdir/link",
			target:   "../file.txt",
			wantErr:  nil,
		},
		{
			name:     "relative symlink escapes destDir",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "../escape.txt",
			wantErr:  core.ErrPathTraversal,
		},
		{
			name:     "relative symlink escapes via deep traversal",
			destDir:  "/tmp/extract",
			linkPath: "dir/link",
			target:   "../../escape.txt",
			wantErr:  core.ErrPathTraversal,
		},
		{
			name:     "absolute symlink rejected",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "/subdir/file.txt",
			wantErr:  core.ErrPathTraversal,
		},
		{
			name:     "absolute symlink to root rejected",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "/",
			wantErr:  core.ErrPathTraversal,
		},
		{
			name:     "symlink to destDir itself",
			destDir:  "/tmp/extract",
			linkPath: "dir/link",
			target:   "..",
			wantErr:  nil,
		},
		{
			name:     "absolute symlink with traversal escapes",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "/../../../etc/passwd",
			wantErr:  core.ErrPathTraversal,
		},
		{
			name:     "absolute symlink to deeply nested path rejected",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "/a/b/c/d/file.txt",
			wantErr:  core.ErrPathTraversal,
		},
		{
			name:     "backslash absolute symlink rejected",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "\\etc\\passwd",
			wantErr:  core.ErrPathTraversal,
		},
		{
			name:     "invalid linkPath with traversal",
			destDir:  "/tmp/extract",
			linkPath: "../escape/link",
			target:   "file.txt",
			wantErr:  core.ErrPathTraversal,
		},
		{
			name:     "invalid linkPath absolute",
			destDir:  "/tmp/extract",
			linkPath: "/etc/link",
			target:   "file.txt",
			wantErr:  core.ErrPathTraversal,
		},
		{
			name:     "null byte in target",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "file\x00.txt",
			wantErr:  core.ErrPathTraversal,
		},
		{
			name:     "null byte in linkPath",
			destDir:  "/tmp/extract",
			linkPath: "link\x00name",
			target:   "file.txt",
			wantErr:  core.ErrPathTraversal,
		},
	}

	// Add Windows-specific tests for volume name targets.
	if runtime.GOOS == osWindows {
		windowsTests := []struct {
			name     string
			destDir  string
			linkPath string
			target   string
			wantErr  error
		}{
			{
				name:     "windows volume name in target",
				destDir:  "C:\\extract",
				linkPath: "link",
				target:   "C:\\Windows\\System32",
				wantErr:  core.ErrPathTraversal,
			},
			{
				name:     "windows UNC target",
				destDir:  "C:\\extract",
				linkPath: "link",
				target:   "\\\\server\\share",
				wantErr:  core.ErrPathTraversal,
			},
			{
				name:     "windows volume in linkPath",
				destDir:  "C:\\extract",
				linkPath: "D:\\different\\link",
				target:   "file.txt",
				wantErr:  core.ErrPathTraversal,
			},
			{
				name:     "windows drive-relative target",
				destDir:  "C:\\extract",
				linkPath: "link",
				target:   "C:relative\\path",
				wantErr:  core.ErrPathTraversal,
			},
		}
		tests = append(tests, windowsTests...)
	}

	v := NewValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := v.ValidateSymlink(tt.destDir, tt.linkPath, tt.target)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr, "ValidateSymlink(%q, %q, %q)", tt.destDir, tt.linkPath, tt.target)
			} else {
				assert.NoError(t, err, "ValidateSymlink(%q, %q, %q)", tt.destDir, tt.linkPath, tt.target)
			}
		})
	}
}

func Test_containsNull(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"normal path", "normal", false},
		{"null in middle", "with\x00null", true},
		{"null at start", "\x00start", true},
		{"null at end", "end\x00", true},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := containsNull(tt.path)
			assert.Equal(t, tt.want, got, "containsNull(%q)", tt.path)
		})
	}
}

func Test_containsTraversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"normal path", "normal/path", false},
		{"parent at start", "../escape", true},
		{"parent in middle", "foo/../bar", true},
		{"parent at end", "foo/..", true},
		{"triple dots", "...", false},
		{"double dots in name", "foo..bar", false},
		{"current dir prefix", "./current", false},
		{"current dir in middle", "foo/./bar", false},
		// Backslash separator tests (relevant for Windows or mixed-separator archives)
		{"backslash parent at start", "..\\escape", true},
		{"backslash parent in middle", "foo\\..\\bar", true},
		{"backslash parent at end", "foo\\..", true},
		{"backslash normal path", "normal\\path", false},
		{"backslash current dir", ".\\current", false},
		// Mixed separators
		{"mixed forward-back parent", "foo/..\\bar", true},
		{"mixed back-forward parent", "foo\\../bar", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := containsTraversal(tt.path)
			assert.Equal(t, tt.want, got, "containsTraversal(%q)", tt.path)
		})
	}
}

func Test_isWithinDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		dir  string
		want bool
	}{
		{"file within dir", "/tmp/extract/file.txt", "/tmp/extract", true},
		{"dir equals path", "/tmp/extract", "/tmp/extract", true},
		{"prefix match but different dir", "/tmp/extractmore", "/tmp/extract", false},
		{"completely different dir", "/tmp/other", "/tmp/extract", false},
		{"unrelated path", "/etc/passwd", "/tmp/extract", false},
		// Root directory edge case
		{"any path within root", "/etc/passwd", "/", true},
		{"nested path within root", "/tmp/extract/file.txt", "/", true},
		{"root equals root", "/", "/", true},
	}
	if runtime.GOOS == osWindows {
		windowsTests := []struct {
			name string
			path string
			dir  string
			want bool
		}{
			{"windows root equals root", `C:\`, `C:\`, true},
			{"windows nested within root", `C:\Windows\System32`, `C:\`, true},
			{"windows different drive", `D:\Other`, `C:\`, false},
			{"windows dir equals path", `C:\Temp`, `C:\Temp`, true},
		}
		tests = append(tests, windowsTests...)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isWithinDir(tt.path, tt.dir)
			assert.Equal(t, tt.want, got, "isWithinDir(%q, %q)", tt.path, tt.dir)
		})
	}
}
