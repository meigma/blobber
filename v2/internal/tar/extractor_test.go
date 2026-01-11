package tar

import (
	"archive/tar"
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testValidator implements PathValidator for testing.
type testValidator struct{}

func (v *testValidator) ValidatePath(path string) error {
	if path == "" || strings.HasPrefix(path, "/") || strings.HasPrefix(path, "..") || strings.Contains(path, "/../") {
		return ErrPathTraversal
	}
	return nil
}

func (v *testValidator) ValidateSymlink(destDir, linkPath, target string) error {
	if err := v.ValidatePath(linkPath); err != nil {
		return err
	}
	if strings.HasPrefix(target, "/") || strings.HasPrefix(target, "..") {
		return ErrPathTraversal
	}
	return nil
}

func newTestValidator() PathValidator {
	return &testValidator{}
}

func TestNewExtractor(t *testing.T) {
	t.Parallel()

	t.Run("returns Extractor interface", func(t *testing.T) {
		t.Parallel()

		v := newTestValidator()
		e := NewExtractor(v)
		require.NotNil(t, e)
	})

	t.Run("accepts options", func(t *testing.T) {
		t.Parallel()

		v := newTestValidator()
		e := NewExtractor(v,
			WithExtractorLogger(nil),
			WithLimits(Limits{MaxFiles: 10}),
			WithEntryFilter(func(path string, isDir bool) bool { return true }),
		)
		require.NotNil(t, e)
	})
}

func TestExtract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		entries   []testTarEntry
		wantFiles map[string]string // path -> content
		wantDirs  []string
		wantLinks map[string]string // path -> target
	}{
		{
			name:      "empty archive",
			entries:   nil,
			wantFiles: nil,
		},
		{
			name: "single file",
			entries: []testTarEntry{
				{name: "hello.txt", content: "hello world", mode: 0o644},
			},
			wantFiles: map[string]string{"hello.txt": "hello world"},
		},
		{
			name: "directory and file",
			entries: []testTarEntry{
				{name: "subdir/", mode: fs.ModeDir | 0o755},
				{name: "subdir/file.txt", content: "in subdir", mode: 0o644},
			},
			wantDirs:  []string{"subdir"},
			wantFiles: map[string]string{"subdir/file.txt": "in subdir"},
		},
		{
			name: "nested structure",
			entries: []testTarEntry{
				{name: "a/", mode: fs.ModeDir | 0o755},
				{name: "a/b/", mode: fs.ModeDir | 0o755},
				{name: "a/b/c.txt", content: "deep", mode: 0o644},
				{name: "root.txt", content: "root", mode: 0o644},
			},
			wantDirs:  []string{"a", "a/b"},
			wantFiles: map[string]string{"a/b/c.txt": "deep", "root.txt": "root"},
		},
		{
			name: "file without explicit parent directory",
			entries: []testTarEntry{
				{name: "implicit/parent/file.txt", content: "content", mode: 0o644},
			},
			wantDirs:  []string{"implicit", "implicit/parent"},
			wantFiles: map[string]string{"implicit/parent/file.txt": "content"},
		},
		{
			name: "symlink",
			entries: []testTarEntry{
				{name: "target.txt", content: "target content", mode: 0o644},
				{name: "link", linkTarget: "target.txt", mode: fs.ModeSymlink},
			},
			wantFiles: map[string]string{"target.txt": "target content"},
			wantLinks: map[string]string{"link": "target.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			archive := createTestTar(t, tt.entries)
			destDir := t.TempDir()

			v := newTestValidator()
			e := NewExtractor(v)
			err := e.Extract(context.Background(), archive, destDir)
			require.NoError(t, err)

			// Verify files.
			for path, wantContent := range tt.wantFiles {
				fullPath := filepath.Join(destDir, path)
				content, err := os.ReadFile(fullPath)
				require.NoError(t, err, "reading %s", path)
				assert.Equal(t, wantContent, string(content), "content of %s", path)
			}

			// Verify directories.
			for _, dir := range tt.wantDirs {
				fullPath := filepath.Join(destDir, dir)
				info, err := os.Stat(fullPath)
				require.NoError(t, err, "stat %s", dir)
				assert.True(t, info.IsDir(), "%s should be a directory", dir)
			}

			// Verify symlinks.
			for path, wantTarget := range tt.wantLinks {
				fullPath := filepath.Join(destDir, path)
				target, err := os.Readlink(fullPath)
				require.NoError(t, err, "readlink %s", path)
				assert.Equal(t, wantTarget, target, "symlink target of %s", path)
			}
		})
	}
}

func TestExtract_Limits(t *testing.T) {
	t.Parallel()

	t.Run("max files", func(t *testing.T) {
		t.Parallel()

		archive := createTestTar(t, []testTarEntry{
			{name: "a.txt", content: "a", mode: 0o644},
			{name: "b.txt", content: "b", mode: 0o644},
			{name: "c.txt", content: "c", mode: 0o644},
		})
		destDir := t.TempDir()

		v := newTestValidator()
		e := NewExtractor(v, WithLimits(Limits{MaxFiles: 2}))
		err := e.Extract(context.Background(), archive, destDir)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrLimitExceeded)
	})

	t.Run("max file size", func(t *testing.T) {
		t.Parallel()

		archive := createTestTar(t, []testTarEntry{
			{name: "large.txt", content: "12345678901234567890", mode: 0o644},
		})
		destDir := t.TempDir()

		v := newTestValidator()
		e := NewExtractor(v, WithLimits(Limits{MaxFileSize: 10}))
		err := e.Extract(context.Background(), archive, destDir)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrLimitExceeded)
	})

	t.Run("max total size", func(t *testing.T) {
		t.Parallel()

		archive := createTestTar(t, []testTarEntry{
			{name: "a.txt", content: "12345", mode: 0o644},
			{name: "b.txt", content: "67890", mode: 0o644},
			{name: "c.txt", content: "abcde", mode: 0o644},
		})
		destDir := t.TempDir()

		v := newTestValidator()
		e := NewExtractor(v, WithLimits(Limits{MaxTotalSize: 12}))
		err := e.Extract(context.Background(), archive, destDir)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrLimitExceeded)
	})

	t.Run("within limits succeeds", func(t *testing.T) {
		t.Parallel()

		archive := createTestTar(t, []testTarEntry{
			{name: "a.txt", content: "12345", mode: 0o644},
			{name: "b.txt", content: "67890", mode: 0o644},
		})
		destDir := t.TempDir()

		v := newTestValidator()
		e := NewExtractor(v, WithLimits(Limits{MaxFiles: 5, MaxFileSize: 100, MaxTotalSize: 100}))
		err := e.Extract(context.Background(), archive, destDir)

		require.NoError(t, err)
	})
}

func TestExtract_EntryFilter(t *testing.T) {
	t.Parallel()

	t.Run("skip by filter", func(t *testing.T) {
		t.Parallel()

		archive := createTestTar(t, []testTarEntry{
			{name: "keep.txt", content: "keep", mode: 0o644},
			{name: "skip.txt", content: "skip", mode: 0o644},
			{name: "stargz.index.json", content: "{}", mode: 0o644},
		})
		destDir := t.TempDir()

		filter := func(path string, isDir bool) bool {
			return path != "skip.txt" && path != "stargz.index.json"
		}

		v := newTestValidator()
		e := NewExtractor(v, WithEntryFilter(filter))
		err := e.Extract(context.Background(), archive, destDir)
		require.NoError(t, err)

		// keep.txt should exist.
		_, err = os.Stat(filepath.Join(destDir, "keep.txt"))
		require.NoError(t, err)

		// skip.txt should not exist.
		_, err = os.Stat(filepath.Join(destDir, "skip.txt"))
		require.True(t, os.IsNotExist(err))

		// stargz.index.json should not exist.
		_, err = os.Stat(filepath.Join(destDir, "stargz.index.json"))
		require.True(t, os.IsNotExist(err))
	})

	t.Run("filter receives correct isDir", func(t *testing.T) {
		t.Parallel()

		archive := createTestTar(t, []testTarEntry{
			{name: "dir/", mode: fs.ModeDir | 0o755},
			{name: "file.txt", content: "content", mode: 0o644},
		})
		destDir := t.TempDir()

		var calls []struct {
			path  string
			isDir bool
		}
		filter := func(path string, isDir bool) bool {
			calls = append(calls, struct {
				path  string
				isDir bool
			}{path, isDir})
			return true
		}

		v := newTestValidator()
		e := NewExtractor(v, WithEntryFilter(filter))
		err := e.Extract(context.Background(), archive, destDir)
		require.NoError(t, err)

		require.Len(t, calls, 2)
		assert.Equal(t, "dir/", calls[0].path)
		assert.True(t, calls[0].isDir)
		assert.Equal(t, "file.txt", calls[1].path)
		assert.False(t, calls[1].isDir)
	})
}

func TestExtract_ContextCancellation(t *testing.T) {
	t.Parallel()

	archive := createTestTar(t, []testTarEntry{
		{name: "file.txt", content: "content", mode: 0o644},
	})
	destDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	v := newTestValidator()
	e := NewExtractor(v)
	err := e.Extract(ctx, archive, destDir)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestExtract_PathValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		entries []testTarEntry
		wantErr error
	}{
		{
			name: "path traversal",
			entries: []testTarEntry{
				{name: "../escape.txt", content: "bad", mode: 0o644},
			},
			wantErr: ErrPathTraversal,
		},
		{
			name: "absolute path",
			entries: []testTarEntry{
				{name: "/etc/passwd", content: "bad", mode: 0o644},
			},
			wantErr: ErrPathTraversal,
		},
		{
			name: "symlink escape",
			entries: []testTarEntry{
				{name: "link", linkTarget: "../escape", mode: fs.ModeSymlink},
			},
			wantErr: ErrPathTraversal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			archive := createTestTar(t, tt.entries)
			destDir := t.TempDir()

			v := newTestValidator()
			e := NewExtractor(v)
			err := e.Extract(context.Background(), archive, destDir)

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestExtract_UnsupportedEntryTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		typeflag byte
	}{
		{name: "hardlink", typeflag: tar.TypeLink},
		{name: "char device", typeflag: tar.TypeChar},
		{name: "block device", typeflag: tar.TypeBlock},
		{name: "fifo", typeflag: tar.TypeFifo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			archive := createTestTarRaw(t, []tar.Header{
				{Name: "special", Typeflag: tt.typeflag, Mode: 0o644},
			})
			destDir := t.TempDir()

			v := newTestValidator()
			e := NewExtractor(v)
			err := e.Extract(context.Background(), archive, destDir)

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrUnsupportedEntry)
		})
	}
}

func TestExtract_TypeRegA(t *testing.T) {
	t.Parallel()

	// Create archive with old-style regular file (TypeRegA).
	//nolint:staticcheck // TypeRegA is deprecated but needed for testing old archives
	archive := createTestTarRawWithContent(t, []rawEntry{
		{header: tar.Header{Name: "oldstyle.txt", Typeflag: tar.TypeRegA, Size: 5, Mode: 0o644}, content: "hello"},
	})
	destDir := t.TempDir()

	v := newTestValidator()
	e := NewExtractor(v)

	err := e.Extract(context.Background(), archive, destDir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(destDir, "oldstyle.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))
}

func TestExtract_DirectoryPermissions(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions not supported on Windows")
	}

	t.Run("dir entry sets permissions", func(t *testing.T) {
		t.Parallel()

		// File creates parent with default perms, then dir entry should fix perms.
		archive := createTestTar(t, []testTarEntry{
			{name: "subdir/file.txt", content: "content", mode: 0o644},
			{name: "subdir/", mode: fs.ModeDir | 0o700}, // Restrictive perms.
		})
		destDir := t.TempDir()

		v := newTestValidator()
		e := NewExtractor(v)
		err := e.Extract(context.Background(), archive, destDir)
		require.NoError(t, err)

		info, err := os.Stat(filepath.Join(destDir, "subdir"))
		require.NoError(t, err)
		assert.Equal(t, fs.FileMode(0o700), info.Mode().Perm())
	})

	t.Run("preserves pre-existing directory permissions", func(t *testing.T) {
		t.Parallel()

		destDir := t.TempDir()

		// Create pre-existing directory with specific permissions.
		preExisting := filepath.Join(destDir, "existing")
		require.NoError(t, os.Mkdir(preExisting, 0o750))

		// Archive has dir entry with different permissions.
		archive := createTestTar(t, []testTarEntry{
			{name: "existing/", mode: fs.ModeDir | 0o700},
		})

		v := newTestValidator()
		e := NewExtractor(v)
		err := e.Extract(context.Background(), archive, destDir)
		require.NoError(t, err)

		// Pre-existing directory should retain original permissions.
		info, err := os.Stat(preExisting)
		require.NoError(t, err)
		assert.Equal(t, fs.FileMode(0o750), info.Mode().Perm())
	})

	t.Run("handles non-canonical paths", func(t *testing.T) {
		t.Parallel()

		// File creates parent, then dir entry with non-canonical path should still chmod.
		archive := createTestTarRawWithContent(t, []rawEntry{
			{header: tar.Header{Name: "a/b/file.txt", Typeflag: tar.TypeReg, Size: 1, Mode: 0o644}, content: "x"},
			{header: tar.Header{Name: "a/./b//", Typeflag: tar.TypeDir, Mode: 0o700}}, // Non-canonical.
		})
		destDir := t.TempDir()

		v := newTestValidator()
		e := NewExtractor(v)
		err := e.Extract(context.Background(), archive, destDir)
		require.NoError(t, err)

		info, err := os.Stat(filepath.Join(destDir, "a", "b"))
		require.NoError(t, err)
		assert.Equal(t, fs.FileMode(0o700), info.Mode().Perm())
	})
}

func TestExtract_MaxFilesCountsAllEntries(t *testing.T) {
	t.Parallel()

	t.Run("counts directories", func(t *testing.T) {
		t.Parallel()

		archive := createTestTar(t, []testTarEntry{
			{name: "dir1/", mode: fs.ModeDir | 0o755},
			{name: "dir2/", mode: fs.ModeDir | 0o755},
			{name: "dir3/", mode: fs.ModeDir | 0o755},
		})
		destDir := t.TempDir()

		v := newTestValidator()
		e := NewExtractor(v, WithLimits(Limits{MaxFiles: 2}))
		err := e.Extract(context.Background(), archive, destDir)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrLimitExceeded)
	})

	t.Run("counts symlinks", func(t *testing.T) {
		t.Parallel()

		archive := createTestTar(t, []testTarEntry{
			{name: "target.txt", content: "x", mode: 0o644},
			{name: "link1", linkTarget: "target.txt", mode: fs.ModeSymlink},
			{name: "link2", linkTarget: "target.txt", mode: fs.ModeSymlink},
		})
		destDir := t.TempDir()

		v := newTestValidator()
		e := NewExtractor(v, WithLimits(Limits{MaxFiles: 2}))
		err := e.Extract(context.Background(), archive, destDir)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrLimitExceeded)
	})

	t.Run("mixed entries", func(t *testing.T) {
		t.Parallel()

		archive := createTestTar(t, []testTarEntry{
			{name: "dir/", mode: fs.ModeDir | 0o755},
			{name: "file.txt", content: "x", mode: 0o644},
			{name: "link", linkTarget: "file.txt", mode: fs.ModeSymlink},
		})
		destDir := t.TempDir()

		v := newTestValidator()
		e := NewExtractor(v, WithLimits(Limits{MaxFiles: 3}))
		err := e.Extract(context.Background(), archive, destDir)

		require.NoError(t, err) // Exactly 3 entries.
	})
}

func TestExtract_FilterWithContextCancellation(t *testing.T) {
	t.Parallel()

	// Create archive with a file that will be filtered.
	archive := createTestTarRawWithContent(t, []rawEntry{
		{header: tar.Header{Name: "filtered.bin", Typeflag: tar.TypeReg, Size: 1024, Mode: 0o644}, content: strings.Repeat("x", 1024)},
	})
	destDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())

	// Filter cancels context when called, then returns false.
	// This ensures discardContent runs with a canceled context.
	filter := func(path string, isDir bool) bool {
		cancel()
		return false
	}

	v := newTestValidator()
	e := NewExtractor(v, WithEntryFilter(filter))
	err := e.Extract(ctx, archive, destDir)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// rawEntry for creating test archives with content.
type rawEntry struct {
	header  tar.Header
	content string
}

// createTestTarRawWithContent creates a tar archive from raw headers with content.
func createTestTarRawWithContent(t *testing.T, entries []rawEntry) *bytes.Reader {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for i := range entries {
		require.NoError(t, tw.WriteHeader(&entries[i].header))
		if entries[i].content != "" {
			_, err := tw.Write([]byte(entries[i].content))
			require.NoError(t, err)
		}
	}

	require.NoError(t, tw.Close())
	return bytes.NewReader(buf.Bytes())
}

// testTarEntry describes an entry for test tar creation.
type testTarEntry struct {
	name       string
	content    string
	mode       fs.FileMode
	linkTarget string
}

// createTestTar creates a tar archive from test entries.
func createTestTar(t *testing.T, entries []testTarEntry) *bytes.Reader {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for _, e := range entries {
		header := &tar.Header{
			Name: e.name,
			Mode: int64(e.mode.Perm()),
		}

		switch {
		case e.mode.IsDir():
			header.Typeflag = tar.TypeDir
		case e.mode&fs.ModeSymlink != 0:
			header.Typeflag = tar.TypeSymlink
			header.Linkname = e.linkTarget
		default:
			header.Typeflag = tar.TypeReg
			header.Size = int64(len(e.content))
		}

		require.NoError(t, tw.WriteHeader(header))

		if header.Typeflag == tar.TypeReg && e.content != "" {
			_, err := tw.Write([]byte(e.content))
			require.NoError(t, err)
		}
	}

	require.NoError(t, tw.Close())
	return bytes.NewReader(buf.Bytes())
}

// createTestTarRaw creates a tar archive from raw headers.
func createTestTarRaw(t *testing.T, headers []tar.Header) *bytes.Reader {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for i := range headers {
		require.NoError(t, tw.WriteHeader(&headers[i]))
	}

	require.NoError(t, tw.Close())
	return bytes.NewReader(buf.Bytes())
}
