package testutils

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFS creates an in-memory filesystem for testing with common file types.
// Directories are explicitly specified with write permissions.
func TestFS() fs.FS {
	return fstest.MapFS{
		"hello.txt": &fstest.MapFile{
			Data:    []byte("Hello, World!"),
			Mode:    0o644,
			ModTime: time.Now(),
		},
		"subdir": &fstest.MapFile{
			Mode:    0o755 | fs.ModeDir,
			ModTime: time.Now(),
		},
		"subdir/nested.txt": &fstest.MapFile{
			Data:    []byte("Nested content"),
			Mode:    0o644,
			ModTime: time.Now(),
		},
		"binary.bin": &fstest.MapFile{
			Data:    []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD},
			Mode:    0o644,
			ModTime: time.Now(),
		},
	}
}

// LargeTestFS creates a filesystem with many files for limit testing.
func LargeTestFS(fileCount int, fileSize int) fs.FS {
	files := make(fstest.MapFS)
	content := bytes.Repeat([]byte("x"), fileSize)
	for i := range fileCount {
		files[fmt.Sprintf("file%d.txt", i)] = &fstest.MapFile{
			Data: content,
			Mode: 0o644,
		}
	}
	return files
}

// AssertFilesMatch verifies that extracted files match the source filesystem.
// It checks both that all expected files exist with correct content AND that
// no unexpected files were created during extraction.
func AssertFilesMatch(t *testing.T, srcFS fs.FS, destDir string) {
	t.Helper()

	// Build set of expected paths from source.
	expectedPaths := make(map[string]bool)
	err := fs.WalkDir(srcFS, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		expectedPaths[path] = true
		return nil
	})
	require.NoError(t, err)

	// Verify all expected files exist and have correct content.
	for path := range expectedPaths {
		destPath := filepath.Join(destDir, path)
		srcInfo, err := fs.Stat(srcFS, path)
		require.NoError(t, err, "failed to stat source %s", path)

		destInfo, err := os.Stat(destPath)
		if err != nil {
			t.Errorf("expected path not found: %s", path)
			continue
		}

		if srcInfo.IsDir() {
			if !destInfo.IsDir() {
				t.Errorf("expected directory, got file: %s", path)
			}
			continue
		}

		// Compare file content for regular files.
		srcContent, err := fs.ReadFile(srcFS, path)
		require.NoError(t, err, "failed to read source %s", path)

		destContent, err := os.ReadFile(destPath)
		require.NoError(t, err, "failed to read dest %s", path)

		if !bytes.Equal(srcContent, destContent) {
			t.Errorf("content mismatch for %s: got %d bytes, want %d bytes",
				path, len(destContent), len(srcContent))
		}
	}

	// Verify no unexpected files were created.
	err = filepath.WalkDir(destDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == destDir {
			return nil
		}

		relPath, err := filepath.Rel(destDir, path)
		if err != nil {
			return err
		}

		// Normalize to forward slashes for comparison.
		relPath = filepath.ToSlash(relPath)

		if !expectedPaths[relPath] {
			t.Errorf("unexpected file in extraction: %s", relPath)
		}
		return nil
	})
	require.NoError(t, err)
}

// AssertFSContains verifies that an fs.FS contains the expected file with content.
func AssertFSContains(t *testing.T, fsys fs.FS, path string, expectedContent []byte) {
	t.Helper()

	content, err := fs.ReadFile(fsys, path)
	require.NoError(t, err, "failed to read %s from fs.FS", path)

	if !bytes.Equal(content, expectedContent) {
		t.Errorf("content mismatch for %s: got %d bytes, want %d bytes",
			path, len(content), len(expectedContent))
	}
}
