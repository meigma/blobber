package validator

import (
	"bytes"
	"log/slog"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/v2/internal/tar"
)

func TestValidatePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		// Valid paths
		{
			name:    "simple file",
			path:    "file.txt",
			wantErr: nil,
		},
		{
			name:    "nested path",
			path:    "dir/subdir/file.txt",
			wantErr: nil,
		},
		{
			name:    "path with dots in filename",
			path:    "file.tar.gz",
			wantErr: nil,
		},
		{
			name:    "single dot directory",
			path:    "./file.txt",
			wantErr: nil,
		},

		// Empty path
		{
			name:    "empty path",
			path:    "",
			wantErr: tar.ErrPathTraversal,
		},

		// Null bytes
		{
			name:    "null byte in path",
			path:    "file\x00.txt",
			wantErr: tar.ErrPathTraversal,
		},
		{
			name:    "null byte at start",
			path:    "\x00file.txt",
			wantErr: tar.ErrPathTraversal,
		},

		// Absolute paths
		{
			name:    "absolute path unix",
			path:    "/etc/passwd",
			wantErr: tar.ErrPathTraversal,
		},

		// Path traversal
		{
			name:    "simple traversal",
			path:    "../file.txt",
			wantErr: tar.ErrPathTraversal,
		},
		{
			name:    "nested traversal",
			path:    "dir/../../../file.txt",
			wantErr: tar.ErrPathTraversal,
		},
		{
			name:    "double dot only",
			path:    "..",
			wantErr: tar.ErrPathTraversal,
		},

		// Windows traversal bypass (trailing dots/spaces)
		// These pass filepath.IsLocal but resolve as ".." on Windows.
		{
			name:    "Windows bypass trailing space",
			path:    ".. /file.txt",
			wantErr: tar.ErrPathTraversal,
		},
		{
			name:    "Windows bypass multiple trailing spaces",
			path:    "..  /file.txt",
			wantErr: tar.ErrPathTraversal,
		},
		{
			name:    "Windows bypass trailing dot",
			path:    ".../file.txt",
			wantErr: tar.ErrPathTraversal,
		},
		{
			name:    "Windows bypass mixed trailing",
			path:    ".. ./file.txt",
			wantErr: tar.ErrPathTraversal,
		},
		{
			name:    "Windows bypass nested",
			path:    "dir/.. /file.txt",
			wantErr: tar.ErrPathTraversal,
		},
		{
			name:    "Windows bypass with backslash",
			path:    "dir\\.. \\file.txt",
			wantErr: tar.ErrPathTraversal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := New()
			err := v.ValidatePath(tt.path)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePath_Windows(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific tests")
	}

	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{
			name:    "volume name",
			path:    "C:file.txt",
			wantErr: tar.ErrPathTraversal,
		},
		{
			name:    "absolute with volume",
			path:    "C:\\Windows\\System32",
			wantErr: tar.ErrPathTraversal,
		},
		{
			name:    "reserved name NUL",
			path:    "NUL",
			wantErr: tar.ErrPathTraversal,
		},
		{
			name:    "reserved name CON",
			path:    "CON",
			wantErr: tar.ErrPathTraversal,
		},
		{
			name:    "reserved name in subdir",
			path:    "dir/NUL",
			wantErr: tar.ErrPathTraversal,
		},
		{
			name:    "traversal with backslash",
			path:    "dir\\..\\..\\file.txt",
			wantErr: tar.ErrPathTraversal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := New()
			err := v.ValidatePath(tt.path)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
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
		// Valid symlinks
		{
			name:     "relative target same directory",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "file.txt",
			wantErr:  nil,
		},
		{
			name:     "relative target subdirectory",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "subdir/file.txt",
			wantErr:  nil,
		},
		{
			name:     "relative target parent within destDir",
			destDir:  "/tmp/extract",
			linkPath: "subdir/link",
			target:   "../file.txt",
			wantErr:  nil,
		},
		{
			name:     "link in subdirectory to sibling",
			destDir:  "/tmp/extract",
			linkPath: "dir1/link",
			target:   "../dir2/file.txt",
			wantErr:  nil,
		},

		// Invalid link paths
		{
			name:     "empty link path",
			destDir:  "/tmp/extract",
			linkPath: "",
			target:   "file.txt",
			wantErr:  tar.ErrPathTraversal,
		},
		{
			name:     "link path with traversal",
			destDir:  "/tmp/extract",
			linkPath: "../link",
			target:   "file.txt",
			wantErr:  tar.ErrPathTraversal,
		},

		// Null bytes
		{
			name:     "null byte in target",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "file\x00.txt",
			wantErr:  tar.ErrPathTraversal,
		},

		// Absolute targets
		{
			name:     "absolute target unix",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "/etc/passwd",
			wantErr:  tar.ErrPathTraversal,
		},
		{
			name:     "absolute target with backslash",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "\\etc\\passwd",
			wantErr:  tar.ErrPathTraversal,
		},

		// Escaping destDir
		{
			name:     "target escapes destDir",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "../outside.txt",
			wantErr:  tar.ErrPathTraversal,
		},
		{
			name:     "target escapes via deep traversal",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "../../etc/passwd",
			wantErr:  tar.ErrPathTraversal,
		},
		{
			name:     "target escapes from subdirectory",
			destDir:  "/tmp/extract",
			linkPath: "subdir/link",
			target:   "../../outside.txt",
			wantErr:  tar.ErrPathTraversal,
		},

		// Windows traversal bypass in targets
		{
			name:     "target Windows bypass trailing space",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   ".. /outside.txt",
			wantErr:  tar.ErrPathTraversal,
		},
		{
			name:     "target Windows bypass trailing dots",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   ".../outside.txt",
			wantErr:  tar.ErrPathTraversal,
		},

		// UNC and device paths
		{
			name:     "UNC path target",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "//server/share/file",
			wantErr:  tar.ErrPathTraversal,
		},
		{
			name:     "UNC path target backslash",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "\\\\server\\share\\file",
			wantErr:  tar.ErrPathTraversal,
		},
		{
			name:     "device path target",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "\\\\.\\COM1",
			wantErr:  tar.ErrPathTraversal,
		},
		{
			name:     "extended path target",
			destDir:  "/tmp/extract",
			linkPath: "link",
			target:   "\\\\?\\C:\\Windows",
			wantErr:  tar.ErrPathTraversal,
		},

		// ValidatePath delegation tests (linkPath validation)
		{
			name:     "absolute linkPath",
			destDir:  "/tmp/extract",
			linkPath: "/etc/passwd",
			target:   "file.txt",
			wantErr:  tar.ErrPathTraversal,
		},
		{
			name:     "null byte in linkPath",
			destDir:  "/tmp/extract",
			linkPath: "link\x00name",
			target:   "file.txt",
			wantErr:  tar.ErrPathTraversal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := New()
			err := v.ValidateSymlink(tt.destDir, tt.linkPath, tt.target)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSymlink_Windows(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific tests")
	}

	tests := []struct {
		name     string
		destDir  string
		linkPath string
		target   string
		wantErr  error
	}{
		{
			name:     "target with volume name",
			destDir:  "C:\\Users\\test\\extract",
			linkPath: "link",
			target:   "D:file.txt",
			wantErr:  tar.ErrPathTraversal,
		},
		{
			name:     "absolute target with volume",
			destDir:  "C:\\Users\\test\\extract",
			linkPath: "link",
			target:   "C:\\Windows\\System32\\config",
			wantErr:  tar.ErrPathTraversal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := New()
			err := v.ValidateSymlink(tt.destDir, tt.linkPath, tt.target)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("returns functional validator", func(t *testing.T) {
		t.Parallel()

		v := New()
		require.NotNil(t, v)

		// Verify the validator works.
		assert.NoError(t, v.ValidatePath("valid/path.txt"))
		assert.ErrorIs(t, v.ValidatePath("../traversal"), tar.ErrPathTraversal)
	})

	t.Run("accepts logger option", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		v := New(WithLogger(logger))
		require.NotNil(t, v)

		// Trigger a validation failure to verify logger is used.
		_ = v.ValidatePath("../traversal")
		assert.Contains(t, buf.String(), "path is not local")
	})

	t.Run("accepts nil logger", func(t *testing.T) {
		t.Parallel()

		v := New(WithLogger(nil))
		require.NotNil(t, v)

		// Verify the validator still works with nil logger option.
		assert.NoError(t, v.ValidatePath("valid/path.txt"))
	})
}

func TestIsWithinDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		dir  string
		want bool
	}{
		{
			name: "exact match",
			path: "/tmp/extract",
			dir:  "/tmp/extract",
			want: true,
		},
		{
			name: "path within dir",
			path: "/tmp/extract/file.txt",
			dir:  "/tmp/extract",
			want: true,
		},
		{
			name: "path in subdirectory",
			path: "/tmp/extract/subdir/file.txt",
			dir:  "/tmp/extract",
			want: true,
		},
		{
			name: "path outside dir",
			path: "/tmp/other/file.txt",
			dir:  "/tmp/extract",
			want: false,
		},
		{
			name: "path is parent",
			path: "/tmp",
			dir:  "/tmp/extract",
			want: false,
		},
		{
			name: "similar prefix but different dir",
			path: "/tmp/extract-other/file.txt",
			dir:  "/tmp/extract",
			want: false,
		},
		{
			name: "root directory",
			path: "/anything",
			dir:  "/",
			want: true,
		},
		{
			name: "dir with trailing separator",
			path: "/tmp/extract/file.txt",
			dir:  "/tmp/extract/",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isWithinDir(tt.path, tt.dir)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestContainsWindowsTraversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		// Not traversal
		{
			name: "normal path",
			path: "dir/file.txt",
			want: false,
		},
		{
			name: "literal double dot",
			path: "..",
			want: false, // Handled by filepath.IsLocal
		},
		{
			name: "path with double dot filename",
			path: "..foo",
			want: false,
		},
		{
			name: "four dots is traversal",
			path: "dir/..../file",
			want: true, // "...." becomes ".." on Windows
		},

		// Windows traversal bypass
		{
			name: "trailing space",
			path: ".. ",
			want: true,
		},
		{
			name: "multiple trailing spaces",
			path: "..  ",
			want: true,
		},
		{
			name: "trailing dot",
			path: "...",
			want: true,
		},
		{
			name: "mixed trailing",
			path: ".. .",
			want: true,
		},
		{
			name: "trailing space in nested path",
			path: "dir/.. /file.txt",
			want: true,
		},
		{
			name: "backslash separator",
			path: "dir\\.. \\file.txt",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := containsWindowsTraversal(tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsWindowsTraversalSegment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		segment string
		want    bool
	}{
		// Not traversal
		{
			name:    "literal double dot",
			segment: "..",
			want:    false,
		},
		{
			name:    "normal directory",
			segment: "dir",
			want:    false,
		},
		{
			name:    "double dot prefix with chars",
			segment: "..foo",
			want:    false,
		},
		{
			name:    "single dot",
			segment: ".",
			want:    false,
		},
		{
			name:    "empty",
			segment: "",
			want:    false,
		},

		// Traversal bypass
		{
			name:    "trailing space",
			segment: ".. ",
			want:    true,
		},
		{
			name:    "multiple trailing spaces",
			segment: "..   ",
			want:    true,
		},
		{
			name:    "trailing dot",
			segment: "...",
			want:    true,
		},
		{
			name:    "multiple trailing dots",
			segment: "....",
			want:    true,
		},
		{
			name:    "mixed dots and spaces",
			segment: ".. . .",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isWindowsTraversalSegment(tt.segment)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestContainsNull(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "no null byte",
			path: "normal/path.txt",
			want: false,
		},
		{
			name: "null byte in middle",
			path: "path\x00file.txt",
			want: true,
		},
		{
			name: "null byte at start",
			path: "\x00path.txt",
			want: true,
		},
		{
			name: "null byte at end",
			path: "path.txt\x00",
			want: true,
		},
		{
			name: "empty string",
			path: "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := containsNull(tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Fuzz tests

func FuzzValidatePath(f *testing.F) {
	// Seed corpus from unit tests - valid paths.
	f.Add("file.txt")
	f.Add("dir/subdir/file.txt")
	f.Add("file.tar.gz")
	f.Add("./file.txt")

	// Invalid paths.
	f.Add("")
	f.Add("file\x00.txt")
	f.Add("\x00file.txt")
	f.Add("/etc/passwd")
	f.Add("../file.txt")
	f.Add("dir/../../../file.txt")
	f.Add("..")

	// Windows traversal bypass patterns.
	f.Add(".. /file.txt")
	f.Add("..  /file.txt")
	f.Add(".../file.txt")
	f.Add(".. ./file.txt")
	f.Add("dir/.. /file.txt")
	f.Add("dir\\.. \\file.txt")

	// Edge cases.
	f.Add(".")
	f.Add("a")
	f.Add("a/b/c/d/e/f/g")
	f.Add("...foo")
	f.Add("..foo")

	f.Fuzz(func(t *testing.T, path string) {
		v := New()

		// Property: never panics (implicit - test fails on panic).

		err1 := v.ValidatePath(path)

		// Property: idempotent - same result on repeated calls.
		err2 := v.ValidatePath(path)
		if (err1 == nil) != (err2 == nil) {
			t.Errorf("ValidatePath not idempotent: first=%v, second=%v", err1, err2)
		}

		// Property: only returns nil or ErrPathTraversal.
		if err1 != nil && err1 != tar.ErrPathTraversal {
			t.Errorf("ValidatePath returned unexpected error type: %v", err1)
		}
	})
}

func FuzzValidateSymlink(f *testing.F) {
	// Seed corpus - valid symlinks.
	f.Add("/tmp/extract", "link", "file.txt")
	f.Add("/tmp/extract", "link", "subdir/file.txt")
	f.Add("/tmp/extract", "subdir/link", "../file.txt")
	f.Add("/tmp/extract", "dir1/link", "../dir2/file.txt")

	// Invalid link paths.
	f.Add("/tmp/extract", "", "file.txt")
	f.Add("/tmp/extract", "../link", "file.txt")
	f.Add("/tmp/extract", "/etc/passwd", "file.txt")
	f.Add("/tmp/extract", "link\x00name", "file.txt")

	// Invalid targets.
	f.Add("/tmp/extract", "link", "file\x00.txt")
	f.Add("/tmp/extract", "link", "/etc/passwd")
	f.Add("/tmp/extract", "link", "\\etc\\passwd")
	f.Add("/tmp/extract", "link", "../outside.txt")
	f.Add("/tmp/extract", "link", "../../etc/passwd")
	f.Add("/tmp/extract", "subdir/link", "../../outside.txt")

	// Windows traversal bypass in targets.
	f.Add("/tmp/extract", "link", ".. /outside.txt")
	f.Add("/tmp/extract", "link", ".../outside.txt")

	// UNC and device paths.
	f.Add("/tmp/extract", "link", "//server/share/file")
	f.Add("/tmp/extract", "link", "\\\\server\\share\\file")
	f.Add("/tmp/extract", "link", "\\\\.\\COM1")
	f.Add("/tmp/extract", "link", "\\\\?\\C:\\Windows")

	// Edge cases.
	f.Add("/tmp/extract", "a", "b")
	f.Add("/", "link", "file.txt")
	f.Add("/tmp/extract/", "link", "file.txt")

	f.Fuzz(func(t *testing.T, destDir, linkPath, target string) {
		v := New()

		// Property: never panics (implicit - test fails on panic).

		err1 := v.ValidateSymlink(destDir, linkPath, target)

		// Property: idempotent - same result on repeated calls.
		err2 := v.ValidateSymlink(destDir, linkPath, target)
		if (err1 == nil) != (err2 == nil) {
			t.Errorf("ValidateSymlink not idempotent: first=%v, second=%v", err1, err2)
		}

		// Property: only returns nil or ErrPathTraversal.
		if err1 != nil && err1 != tar.ErrPathTraversal {
			t.Errorf("ValidateSymlink returned unexpected error type: %v", err1)
		}

		// Property: if linkPath fails ValidatePath, ValidateSymlink must also fail.
		linkPathErr := v.ValidatePath(linkPath)
		if linkPathErr != nil && err1 == nil {
			t.Errorf("ValidatePath(%q) failed but ValidateSymlink passed", linkPath)
		}
	})
}

func FuzzContainsWindowsTraversal(f *testing.F) {
	// Normal paths - should return false.
	f.Add("dir/file.txt")
	f.Add("..")
	f.Add("..foo")
	f.Add("file.txt")
	f.Add("")

	// Traversal bypass patterns - should return true.
	f.Add(".. ")
	f.Add("..  ")
	f.Add("...")
	f.Add(".. .")
	f.Add("dir/.. /file.txt")
	f.Add("dir\\.. \\file.txt")
	f.Add("dir/..../file")

	f.Fuzz(func(t *testing.T, path string) {
		// Property: never panics (implicit).

		result1 := containsWindowsTraversal(path)

		// Property: idempotent.
		result2 := containsWindowsTraversal(path)
		if result1 != result2 {
			t.Errorf("containsWindowsTraversal not idempotent: first=%v, second=%v", result1, result2)
		}
	})
}

func FuzzIsWithinDir(f *testing.F) {
	// Path within dir.
	f.Add("/tmp/extract", "/tmp/extract")
	f.Add("/tmp/extract/file.txt", "/tmp/extract")
	f.Add("/tmp/extract/subdir/file.txt", "/tmp/extract")

	// Path outside dir.
	f.Add("/tmp/other/file.txt", "/tmp/extract")
	f.Add("/tmp", "/tmp/extract")
	f.Add("/tmp/extract-other/file.txt", "/tmp/extract")

	// Edge cases.
	f.Add("/anything", "/")
	f.Add("/tmp/extract/file.txt", "/tmp/extract/")
	f.Add("", "")
	f.Add("/", "/")

	f.Fuzz(func(t *testing.T, path, dir string) {
		// Property: never panics (implicit).

		result1 := isWithinDir(path, dir)

		// Property: idempotent.
		result2 := isWithinDir(path, dir)
		if result1 != result2 {
			t.Errorf("isWithinDir not idempotent: first=%v, second=%v", result1, result2)
		}

		// Property: path == dir implies isWithinDir is true.
		if path == dir && !result1 {
			t.Errorf("isWithinDir(%q, %q) = false, expected true for equal paths", path, dir)
		}
	})
}

func FuzzContainsNull(f *testing.F) {
	f.Add("normal/path.txt")
	f.Add("path\x00file.txt")
	f.Add("\x00path.txt")
	f.Add("path.txt\x00")
	f.Add("")
	f.Add("\x00")
	f.Add("\x00\x00")

	f.Fuzz(func(t *testing.T, path string) {
		// Property: never panics (implicit).

		result1 := containsNull(path)

		// Property: idempotent.
		result2 := containsNull(path)
		if result1 != result2 {
			t.Errorf("containsNull not idempotent: first=%v, second=%v", result1, result2)
		}

		// Property: result matches strings.ContainsRune behavior.
		expected := false
		for _, r := range path {
			if r == '\x00' {
				expected = true
				break
			}
		}
		if result1 != expected {
			t.Errorf("containsNull(%q) = %v, expected %v", path, result1, expected)
		}
	})
}

func FuzzIsWindowsTraversalSegment(f *testing.F) {
	// Not traversal.
	f.Add("..")
	f.Add("dir")
	f.Add("..foo")
	f.Add(".")
	f.Add("")

	// Traversal bypass.
	f.Add(".. ")
	f.Add("..   ")
	f.Add("...")
	f.Add("....")
	f.Add(".. . .")

	f.Fuzz(func(t *testing.T, segment string) {
		// Property: never panics (implicit).

		result1 := isWindowsTraversalSegment(segment)

		// Property: idempotent.
		result2 := isWindowsTraversalSegment(segment)
		if result1 != result2 {
			t.Errorf("isWindowsTraversalSegment not idempotent: first=%v, second=%v", result1, result2)
		}

		// Property: literal ".." should return false (handled by filepath.IsLocal).
		if segment == ".." && result1 {
			t.Error("isWindowsTraversalSegment(\"..\") should return false")
		}

		// Property: if true, segment must start with "..".
		if result1 && len(segment) < 2 {
			t.Errorf("isWindowsTraversalSegment(%q) = true but len < 2", segment)
		}
		if result1 && segment[:2] != ".." {
			t.Errorf("isWindowsTraversalSegment(%q) = true but doesn't start with \"..\"", segment)
		}
	})
}
