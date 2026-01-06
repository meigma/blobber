package safepath

import (
	"errors"
	"math"
	"runtime"
	"testing"

	"github.com/gilmanlab/blobber"
)

const osWindows = "windows"

func TestValidatePath(t *testing.T) {
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
			wantErr: blobber.ErrPathTraversal,
		},
		{
			name:    "parent traversal in middle",
			path:    "foo/../bar",
			wantErr: blobber.ErrPathTraversal,
		},
		{
			name:    "parent traversal at end",
			path:    "foo/bar/..",
			wantErr: blobber.ErrPathTraversal,
		},
		{
			name:    "absolute path unix",
			path:    "/etc/passwd",
			wantErr: blobber.ErrPathTraversal,
		},
		{
			name:    "null byte",
			path:    "foo\x00bar",
			wantErr: blobber.ErrPathTraversal,
		},
		{
			name:    "null byte at end",
			path:    "foo.txt\x00",
			wantErr: blobber.ErrPathTraversal,
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
			wantErr: blobber.ErrPathTraversal,
		},
		{
			name:    "backslash traversal in middle",
			path:    "foo\\..\\bar",
			wantErr: blobber.ErrPathTraversal,
		},
		{
			name:    "mixed slash traversal",
			path:    "foo/..\\bar",
			wantErr: blobber.ErrPathTraversal,
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
				wantErr: blobber.ErrPathTraversal,
			},
			{
				name:    "windows drive letter lowercase",
				path:    "c:\\temp\\file.txt",
				wantErr: blobber.ErrPathTraversal,
			},
			{
				name:    "windows UNC path",
				path:    "\\\\server\\share\\file.txt",
				wantErr: blobber.ErrPathTraversal,
			},
			{
				name:    "windows drive relative",
				path:    "C:relative\\path",
				wantErr: blobber.ErrPathTraversal,
			},
		}
		tests = append(tests, windowsTests...)
	}

	v := NewValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := v.ValidatePath(tt.path)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidatePath(%q) = %v, want %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestValidateExtraction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		destDir string
		entries []blobber.TOCEntry
		limits  blobber.ExtractLimits
		wantErr error
	}{
		{
			name:    "valid extraction no limits",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 100},
				{Name: "dir/file2.txt", Type: "reg", Size: 200},
			},
			limits:  blobber.ExtractLimits{},
			wantErr: nil,
		},
		{
			name:    "valid extraction within limits",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 100},
				{Name: "file2.txt", Type: "reg", Size: 200},
			},
			limits: blobber.ExtractLimits{
				MaxFiles:     10,
				MaxTotalSize: 1000,
				MaxFileSize:  500,
			},
			wantErr: nil,
		},
		{
			name:    "exceeds max files",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 100},
				{Name: "file2.txt", Type: "reg", Size: 100},
				{Name: "file3.txt", Type: "reg", Size: 100},
			},
			limits: blobber.ExtractLimits{
				MaxFiles: 2,
			},
			wantErr: blobber.ErrExtractLimits,
		},
		{
			name:    "exceeds max total size",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 500},
				{Name: "file2.txt", Type: "reg", Size: 600},
			},
			limits: blobber.ExtractLimits{
				MaxTotalSize: 1000,
			},
			wantErr: blobber.ErrExtractLimits,
		},
		{
			name:    "exceeds max file size",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 100},
				{Name: "large.bin", Type: "reg", Size: 1000},
			},
			limits: blobber.ExtractLimits{
				MaxFileSize: 500,
			},
			wantErr: blobber.ErrExtractLimits,
		},
		{
			name:    "path traversal in entry",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "../escape.txt", Type: "reg", Size: 100},
			},
			limits:  blobber.ExtractLimits{},
			wantErr: blobber.ErrPathTraversal,
		},
		{
			name:    "absolute path in entry",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "/etc/passwd", Type: "reg", Size: 100},
			},
			limits:  blobber.ExtractLimits{},
			wantErr: blobber.ErrPathTraversal,
		},
		{
			name:    "directories not counted in file limit",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "dir1", Type: "dir", Size: 0},
				{Name: "dir2", Type: "dir", Size: 0},
				{Name: "dir3", Type: "dir", Size: 0},
				{Name: "file1.txt", Type: "reg", Size: 100},
			},
			limits: blobber.ExtractLimits{
				MaxFiles: 2,
			},
			wantErr: nil,
		},
		{
			name:    "symlinks not counted in file limit",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "link1", Type: "symlink", LinkName: "file1.txt"},
				{Name: "link2", Type: "symlink", LinkName: "file1.txt"},
				{Name: "file1.txt", Type: "reg", Size: 100},
			},
			limits: blobber.ExtractLimits{
				MaxFiles: 2,
			},
			wantErr: nil,
		},
		{
			name:    "empty entries",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{},
			limits:  blobber.ExtractLimits{},
			wantErr: nil,
		},
		{
			name:    "negative size rejected",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: -100},
			},
			limits:  blobber.ExtractLimits{},
			wantErr: blobber.ErrExtractLimits,
		},
		{
			name:    "size overflow rejected",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: math.MaxInt64},
				{Name: "file2.txt", Type: "reg", Size: 1},
			},
			limits:  blobber.ExtractLimits{},
			wantErr: blobber.ErrExtractLimits,
		},
		{
			name:    "large sizes within int64 range",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: math.MaxInt64 / 2},
				{Name: "file2.txt", Type: "reg", Size: math.MaxInt64 / 2},
			},
			limits:  blobber.ExtractLimits{},
			wantErr: nil,
		},
		// Boundary condition tests - exactly at limit should pass
		{
			name:    "exactly at max files limit",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 100},
				{Name: "file2.txt", Type: "reg", Size: 100},
			},
			limits: blobber.ExtractLimits{
				MaxFiles: 2,
			},
			wantErr: nil,
		},
		{
			name:    "exactly at max total size limit",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 500},
				{Name: "file2.txt", Type: "reg", Size: 500},
			},
			limits: blobber.ExtractLimits{
				MaxTotalSize: 1000,
			},
			wantErr: nil,
		},
		{
			name:    "exactly at max file size limit",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 500},
			},
			limits: blobber.ExtractLimits{
				MaxFileSize: 500,
			},
			wantErr: nil,
		},
		{
			name:    "one over max files limit",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 100},
				{Name: "file2.txt", Type: "reg", Size: 100},
				{Name: "file3.txt", Type: "reg", Size: 100},
			},
			limits: blobber.ExtractLimits{
				MaxFiles: 2,
			},
			wantErr: blobber.ErrExtractLimits,
		},
		{
			name:    "one byte over max total size",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 500},
				{Name: "file2.txt", Type: "reg", Size: 501},
			},
			limits: blobber.ExtractLimits{
				MaxTotalSize: 1000,
			},
			wantErr: blobber.ErrExtractLimits,
		},
		{
			name:    "one byte over max file size",
			destDir: "/tmp/extract",
			entries: []blobber.TOCEntry{
				{Name: "file1.txt", Type: "reg", Size: 501},
			},
			limits: blobber.ExtractLimits{
				MaxFileSize: 500,
			},
			wantErr: blobber.ErrExtractLimits,
		},
	}

	v := NewValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := v.ValidateExtraction(tt.destDir, tt.entries, tt.limits)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateExtraction() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSymlink(t *testing.T) {
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
			wantErr:  blobber.ErrPathTraversal,
		},
		{
			name:     "relative symlink escapes via deep traversal",
			destDir:  "/tmp/extract",
			linkPath: "dir/link",
			target:   "../../escape.txt",
			wantErr:  blobber.ErrPathTraversal,
		},
		{
			name:     "absolute symlink within destDir",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "/subdir/file.txt",
			wantErr:  nil,
		},
		{
			name:     "absolute symlink to root stays within",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "/",
			wantErr:  nil,
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
			wantErr:  blobber.ErrPathTraversal,
		},
		{
			name:     "absolute symlink to deeply nested path",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "/a/b/c/d/file.txt",
			wantErr:  nil,
		},
		{
			name:     "invalid linkPath with traversal",
			destDir:  "/tmp/extract",
			linkPath: "../escape/link",
			target:   "file.txt",
			wantErr:  blobber.ErrPathTraversal,
		},
		{
			name:     "invalid linkPath absolute",
			destDir:  "/tmp/extract",
			linkPath: "/etc/link",
			target:   "file.txt",
			wantErr:  blobber.ErrPathTraversal,
		},
		{
			name:     "null byte in target",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "file\x00.txt",
			wantErr:  blobber.ErrPathTraversal,
		},
		{
			name:     "null byte in linkPath",
			destDir:  "/tmp/extract",
			linkPath: "link\x00name",
			target:   "file.txt",
			wantErr:  blobber.ErrPathTraversal,
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
				wantErr:  blobber.ErrPathTraversal,
			},
			{
				name:     "windows UNC target",
				destDir:  "C:\\extract",
				linkPath: "link",
				target:   "\\\\server\\share",
				wantErr:  blobber.ErrPathTraversal,
			},
			{
				name:     "windows volume in linkPath",
				destDir:  "C:\\extract",
				linkPath: "D:\\different\\link",
				target:   "file.txt",
				wantErr:  blobber.ErrPathTraversal,
			},
			{
				name:     "windows drive-relative target",
				destDir:  "C:\\extract",
				linkPath: "link",
				target:   "C:relative\\path",
				wantErr:  blobber.ErrPathTraversal,
			},
		}
		tests = append(tests, windowsTests...)
	}

	v := NewValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := v.ValidateSymlink(tt.destDir, tt.linkPath, tt.target)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateSymlink(%q, %q, %q) = %v, want %v",
					tt.destDir, tt.linkPath, tt.target, err, tt.wantErr)
			}
		})
	}
}

func TestContainsNull(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{"normal", false},
		{"with\x00null", true},
		{"\x00start", true},
		{"end\x00", true},
		{"", false},
	}

	for _, tt := range tests {
		got := containsNull(tt.path)
		if got != tt.want {
			t.Errorf("containsNull(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestContainsTraversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{"normal/path", false},
		{"../escape", true},
		{"foo/../bar", true},
		{"foo/..", true},
		{"...", false},
		{"foo..bar", false},
		{"./current", false},
		{"foo/./bar", false},
		// Backslash separator tests (relevant for Windows or mixed-separator archives)
		{"..\\escape", true},
		{"foo\\..\\bar", true},
		{"foo\\..", true},
		{"normal\\path", false},
		{".\\current", false},
		// Mixed separators
		{"foo/..\\bar", true},
		{"foo\\../bar", true},
	}

	for _, tt := range tests {
		got := containsTraversal(tt.path)
		if got != tt.want {
			t.Errorf("containsTraversal(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsWithinDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		dir  string
		want bool
	}{
		{"/tmp/extract/file.txt", "/tmp/extract", true},
		{"/tmp/extract", "/tmp/extract", true},
		{"/tmp/extractmore", "/tmp/extract", false},
		{"/tmp/other", "/tmp/extract", false},
		{"/etc/passwd", "/tmp/extract", false},
		// Root directory edge case
		{"/etc/passwd", "/", true},
		{"/tmp/extract/file.txt", "/", true},
		{"/", "/", true},
	}
	if runtime.GOOS == osWindows {
		windowsTests := []struct {
			path string
			dir  string
			want bool
		}{
			{`C:\`, `C:\`, true},
			{`C:\Windows\System32`, `C:\`, true},
			{`D:\Other`, `C:\`, false},
			{`C:\Temp`, `C:\Temp`, true},
		}
		tests = append(tests, windowsTests...)
	}

	for _, tt := range tests {
		got := isWithinDir(tt.path, tt.dir)
		if got != tt.want {
			t.Errorf("isWithinDir(%q, %q) = %v, want %v", tt.path, tt.dir, got, tt.want)
		}
	}
}

func TestIsAbsolute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{"relative/path", false},
		{"/absolute/path", true},
		{"./current", false},
		{"../parent", false},
		{"", false},
	}

	for _, tt := range tests {
		got := isAbsolute(tt.path)
		if got != tt.want {
			t.Errorf("isAbsolute(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestHasVolumeName(t *testing.T) {
	t.Parallel()

	// Cross-platform tests (these should behave consistently).
	tests := []struct {
		path string
		want bool
	}{
		{"relative/path", false},
		{"./current", false},
		{"", false},
	}

	// Add platform-specific expectations.
	if runtime.GOOS == osWindows {
		windowsTests := []struct {
			path string
			want bool
		}{
			{"C:\\Windows", true},
			{"c:\\temp", true},
			{"D:", true},
			{"\\\\server\\share", true},
			{"/unix/style", false},
		}
		tests = append(tests, windowsTests...)
	} else {
		// On Unix, no paths have volume names.
		unixTests := []struct {
			path string
			want bool
		}{
			{"/absolute/path", false},
			{"C:\\fake\\windows", false},
		}
		tests = append(tests, unixTests...)
	}

	for _, tt := range tests {
		got := hasVolumeName(tt.path)
		if got != tt.want {
			t.Errorf("hasVolumeName(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
