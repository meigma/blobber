//go:build integration

package blobber_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/meigma/blobber"
)

// testTimeout is the default timeout for integration test operations.
const testTimeout = 2 * time.Minute

// registryContainer wraps the OCI registry container with connection details.
type registryContainer struct {
	testcontainers.Container
	Host string
}

// testContext returns a context with timeout for test operations.
// The timeout is cancelled when the test completes.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	t.Cleanup(cancel)
	return ctx
}

// setupRegistry starts a distribution/registry container for testing.
func setupRegistry(ctx context.Context, t *testing.T) *registryContainer {
	t.Helper()

	container, err := testcontainers.Run(ctx,
		"registry:2",
		testcontainers.WithExposedPorts("5000/tcp"),
		testcontainers.WithEnv(map[string]string{
			"REGISTRY_STORAGE_DELETE_ENABLED": "true",
		}),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v2/").
				WithPort("5000/tcp").
				WithStatusCodeMatcher(func(status int) bool {
					return status == 200
				}).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start registry container: %v", err)
	}
	testcontainers.CleanupContainer(t, container)

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "5000")
	require.NoError(t, err)

	return &registryContainer{
		Container: container,
		Host:      host + ":" + port.Port(),
	}
}

// testFS creates an in-memory filesystem for testing.
// Note: Directories must be explicitly specified with write permissions,
// otherwise fstest.MapFS synthesizes them with 0555 (no write).
func testFS() fs.FS {
	return fstest.MapFS{
		"hello.txt": &fstest.MapFile{
			Data:    []byte("Hello, World!"),
			Mode:    0644,
			ModTime: time.Now(),
		},
		"subdir": &fstest.MapFile{
			Mode:    0755 | fs.ModeDir,
			ModTime: time.Now(),
		},
		"subdir/nested.txt": &fstest.MapFile{
			Data:    []byte("Nested content"),
			Mode:    0644,
			ModTime: time.Now(),
		},
		"binary.bin": &fstest.MapFile{
			Data:    []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD},
			Mode:    0644,
			ModTime: time.Now(),
		},
	}
}

// largeTestFS creates a filesystem with many files for limit testing.
func largeTestFS(fileCount int, fileSize int) fs.FS {
	files := make(fstest.MapFS)
	content := bytes.Repeat([]byte("x"), fileSize)
	for i := 0; i < fileCount; i++ {
		files[fmt.Sprintf("file%d.txt", i)] = &fstest.MapFile{
			Data: content,
			Mode: 0644,
		}
	}
	return files
}

// assertFilesMatch verifies that extracted files match the source filesystem.
// It checks both that all expected files exist with correct content AND that
// no unexpected files were created during extraction.
func assertFilesMatch(t *testing.T, srcFS fs.FS, destDir string) {
	t.Helper()

	// Build set of expected paths from source
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

	// Verify all expected files exist and have correct content
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

		// Compare file content for regular files
		srcContent, err := fs.ReadFile(srcFS, path)
		require.NoError(t, err, "failed to read source %s", path)

		destContent, err := os.ReadFile(destPath)
		require.NoError(t, err, "failed to read dest %s", path)

		if !bytes.Equal(srcContent, destContent) {
			t.Errorf("content mismatch for %s: got %d bytes, want %d bytes", path, len(destContent), len(srcContent))
		}
	}

	// Verify no unexpected files were created
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

		// Normalize to forward slashes for comparison (fs.WalkDir always uses /)
		relPath = filepath.ToSlash(relPath)

		if !expectedPaths[relPath] {
			t.Errorf("unexpected file in extraction: %s", relPath)
		}
		return nil
	})
	require.NoError(t, err)
}

func TestIntegration_PushPull_Gzip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/gzip:v1"
	srcFS := testFS()

	// Push
	digest, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)
	assert.NotEmpty(t, digest)
	assert.True(t, strings.HasPrefix(digest, "sha256:"), "digest should start with sha256:")

	// Pull to temp directory
	destDir := t.TempDir()
	err = client.Pull(ctx, ref, destDir)
	require.NoError(t, err)

	// Verify extracted files match source
	assertFilesMatch(t, srcFS, destDir)
}

func TestIntegration_PushPull_Zstd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/zstd:v1"
	srcFS := testFS()

	// Push with zstd compression
	digest, err := client.Push(ctx, ref, srcFS,
		blobber.WithCompression(blobber.ZstdCompression()),
	)
	require.NoError(t, err)
	assert.NotEmpty(t, digest)

	// Pull and verify
	destDir := t.TempDir()
	err = client.Pull(ctx, ref, destDir)
	require.NoError(t, err)

	assertFilesMatch(t, srcFS, destDir)
}

func TestIntegration_OpenImage_List(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/test/list:v1"
	srcFS := testFS()

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Open image and list files
	img, err := client.OpenImage(ctx, ref)
	require.NoError(t, err)
	defer img.Close()

	entries, err := img.List()
	require.NoError(t, err)

	// Verify expected files are listed
	paths := make(map[string]bool)
	for _, e := range entries {
		paths[e.Path()] = true
	}

	assert.True(t, paths["hello.txt"], "should contain hello.txt")
	assert.True(t, paths["subdir/nested.txt"], "should contain subdir/nested.txt")
	assert.True(t, paths["binary.bin"], "should contain binary.bin")
}

func TestIntegration_OpenImage_Open(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/test/open:v1"
	srcFS := testFS()

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	img, err := client.OpenImage(ctx, ref)
	require.NoError(t, err)
	defer img.Close()

	// Read specific file
	rc, err := img.Open("hello.txt")
	require.NoError(t, err)
	defer rc.Close()

	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!", string(content))
}

func TestIntegration_OpenImage_Open_NestedFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/test/opennested:v1"
	srcFS := testFS()

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	img, err := client.OpenImage(ctx, ref)
	require.NoError(t, err)
	defer img.Close()

	// Read nested file
	rc, err := img.Open("subdir/nested.txt")
	require.NoError(t, err)
	defer rc.Close()

	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "Nested content", string(content))
}

func TestIntegration_OpenImage_Open_BinaryFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/test/openbinary:v1"
	srcFS := testFS()

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	img, err := client.OpenImage(ctx, ref)
	require.NoError(t, err)
	defer img.Close()

	// Read binary file
	rc, err := img.Open("binary.bin")
	require.NoError(t, err)
	defer rc.Close()

	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	expected := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD}
	assert.Equal(t, expected, content)
}

func TestIntegration_OpenImage_Walk(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/test/walk:v1"
	srcFS := testFS()

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	img, err := client.OpenImage(ctx, ref)
	require.NoError(t, err)
	defer img.Close()

	// Walk and collect paths
	var paths []string
	err = img.Walk(func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		paths = append(paths, path)
		return nil
	})
	require.NoError(t, err)
	assert.Contains(t, paths, "hello.txt")
	assert.Contains(t, paths, "subdir")
	assert.Contains(t, paths, "subdir/nested.txt")
}

func TestIntegration_OpenImage_WalkSkipDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/test/walkskip:v1"
	srcFS := testFS()

	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	img, err := client.OpenImage(ctx, ref)
	require.NoError(t, err)
	defer img.Close()

	// Walk but skip subdir
	var paths []string
	err = img.Walk(func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "subdir" {
			return fs.SkipDir
		}
		paths = append(paths, path)
		return nil
	})
	require.NoError(t, err)
	assert.Contains(t, paths, "hello.txt")
	assert.NotContains(t, paths, "subdir/nested.txt")
}

func TestIntegration_Push_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	// Use a cancelled context
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel immediately

	ref := reg.Host + "/test/cancel:v1"
	srcFS := testFS()

	_, err = client.Push(cancelCtx, ref, srcFS)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestIntegration_Pull_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	// Push first
	ref := reg.Host + "/test/pullcancel:v1"
	srcFS := testFS()
	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Try to pull with cancelled context
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	destDir := t.TempDir()
	err = client.Pull(cancelCtx, ref, destDir)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestIntegration_InvalidReference(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	tests := []struct {
		name string
		ref  string
	}{
		{"empty", ""},
		{"spaces", "invalid ref with spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcFS := testFS()
			_, err := client.Push(ctx, tt.ref, srcFS)
			require.Error(t, err)
			assert.ErrorIs(t, err, blobber.ErrInvalidRef)
		})
	}
}

func TestIntegration_Pull_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/nonexistent/image:v1"
	destDir := t.TempDir()

	err = client.Pull(ctx, ref, destDir)
	require.Error(t, err)
	assert.ErrorIs(t, err, blobber.ErrNotFound)
}

func TestIntegration_OpenImage_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/nonexistent/image:v1"
	_, err = client.OpenImage(ctx, ref)
	require.Error(t, err)
	assert.ErrorIs(t, err, blobber.ErrNotFound)
}

func TestIntegration_Pull_ExtractLimits_MaxFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	// Push archive with many files
	ref := reg.Host + "/test/limits:v1"
	srcFS := largeTestFS(20, 100) // 20 files
	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Pull with file limit lower than actual count
	destDir := t.TempDir()
	err = client.Pull(ctx, ref, destDir,
		blobber.WithExtractLimits(blobber.ExtractLimits{
			MaxFiles: 5,
		}),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, blobber.ErrExtractLimits)
}

func TestIntegration_Pull_ExtractLimits_MaxTotalSize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	// Push archive with known total size
	ref := reg.Host + "/test/sizelimit:v1"
	srcFS := largeTestFS(10, 1024) // 10 files, 1KB each = ~10KB total
	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Pull with size limit lower than total
	destDir := t.TempDir()
	err = client.Pull(ctx, ref, destDir,
		blobber.WithExtractLimits(blobber.ExtractLimits{
			MaxTotalSize: 5 * 1024, // 5KB limit
		}),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, blobber.ErrExtractLimits)
}

func TestIntegration_Image_ClosedOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/test/closed:v1"
	srcFS := testFS()
	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	img, err := client.OpenImage(ctx, ref)
	require.NoError(t, err)

	// Close the image
	err = img.Close()
	require.NoError(t, err)

	// All operations should fail with ErrClosed
	_, err = img.List()
	assert.ErrorIs(t, err, blobber.ErrClosed)

	_, err = img.Open("hello.txt")
	assert.ErrorIs(t, err, blobber.ErrClosed)

	err = img.Walk(func(path string, d fs.DirEntry, walkErr error) error {
		return nil
	})
	assert.ErrorIs(t, err, blobber.ErrClosed)
}

func TestIntegration_Image_OpenNonexistentFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/test/nofile:v1"
	srcFS := testFS()
	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	img, err := client.OpenImage(ctx, ref)
	require.NoError(t, err)
	defer img.Close()

	_, err = img.Open("nonexistent.txt")
	require.Error(t, err)
	assert.ErrorIs(t, err, blobber.ErrNotFound)
}

func TestIntegration_Push_WithAnnotations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/test/annotated:v1"
	srcFS := testFS()

	annotations := map[string]string{
		"org.opencontainers.image.title":   "Test Image",
		"org.opencontainers.image.version": "1.0.0",
	}

	digest, err := client.Push(ctx, ref, srcFS,
		blobber.WithAnnotations(annotations),
	)
	require.NoError(t, err)
	assert.NotEmpty(t, digest)

	// Pull should still work
	destDir := t.TempDir()
	err = client.Pull(ctx, ref, destDir)
	require.NoError(t, err)
}

func TestIntegration_RoundTrip_PreservesContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	// Create filesystem with various content types
	// Note: Directories must be explicitly specified with write permissions
	srcFS := fstest.MapFS{
		"text.txt": &fstest.MapFile{
			Data: []byte("Plain text content"),
			Mode: 0644,
		},
		"unicode.txt": &fstest.MapFile{
			Data: []byte("Unicode: \u4f60\u597d\u4e16\u754c \xf0\x9f\x8c\x8d"),
			Mode: 0644,
		},
		"empty.txt": &fstest.MapFile{
			Data: []byte{},
			Mode: 0644,
		},
		"binary.dat": &fstest.MapFile{
			Data: []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD},
			Mode: 0644,
		},
		"deep": &fstest.MapFile{
			Mode: 0755 | fs.ModeDir,
		},
		"deep/nested": &fstest.MapFile{
			Mode: 0755 | fs.ModeDir,
		},
		"deep/nested/path": &fstest.MapFile{
			Mode: 0755 | fs.ModeDir,
		},
		"deep/nested/path/file.txt": &fstest.MapFile{
			Data: []byte("Deeply nested"),
			Mode: 0644,
		},
	}

	ref := reg.Host + "/test/roundtrip:v1"
	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	destDir := t.TempDir()
	err = client.Pull(ctx, ref, destDir)
	require.NoError(t, err)

	assertFilesMatch(t, srcFS, destDir)
}

func TestIntegration_MultipleTagsSameRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	// Push different content to different tags
	fs1 := fstest.MapFS{
		"version.txt": &fstest.MapFile{Data: []byte("v1"), Mode: 0644},
	}
	fs2 := fstest.MapFS{
		"version.txt": &fstest.MapFile{Data: []byte("v2"), Mode: 0644},
	}

	ref1 := reg.Host + "/test/multitag:v1"
	ref2 := reg.Host + "/test/multitag:v2"

	digest1, err := client.Push(ctx, ref1, fs1)
	require.NoError(t, err)

	digest2, err := client.Push(ctx, ref2, fs2)
	require.NoError(t, err)

	assert.NotEqual(t, digest1, digest2, "different content should have different digests")

	// Verify each tag has correct content
	img1, err := client.OpenImage(ctx, ref1)
	require.NoError(t, err)
	defer img1.Close()

	rc1, err := img1.Open("version.txt")
	require.NoError(t, err)
	content1, err := io.ReadAll(rc1)
	require.NoError(t, err)
	require.NoError(t, rc1.Close())
	assert.Equal(t, "v1", string(content1))

	img2, err := client.OpenImage(ctx, ref2)
	require.NoError(t, err)
	defer img2.Close()

	rc2, err := img2.Open("version.txt")
	require.NoError(t, err)
	content2, err := io.ReadAll(rc2)
	require.NoError(t, err)
	require.NoError(t, rc2.Close())
	assert.Equal(t, "v2", string(content2))
}

func TestIntegration_PullByDigest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/test/bydigest:v1"
	srcFS := testFS()

	// Push and get digest
	digest, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Pull by digest reference
	digestRef := reg.Host + "/test/bydigest@" + digest
	destDir := t.TempDir()
	err = client.Pull(ctx, digestRef, destDir)
	require.NoError(t, err)

	assertFilesMatch(t, srcFS, destDir)
}

func TestIntegration_OpenImageByDigest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/test/openbydigest:v1"
	srcFS := testFS()

	// Push and get digest
	digest, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Open by digest reference
	digestRef := reg.Host + "/test/openbydigest@" + digest
	img, err := client.OpenImage(ctx, digestRef)
	require.NoError(t, err)
	defer img.Close()

	// Verify we can read files
	rc, err := img.Open("hello.txt")
	require.NoError(t, err)
	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	assert.Equal(t, "Hello, World!", string(content))
}

func TestIntegration_EmptyFilesystem(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/test/empty:v1"
	srcFS := fstest.MapFS{}

	// Push empty filesystem
	digest, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)
	assert.NotEmpty(t, digest)

	// Pull and verify empty
	destDir := t.TempDir()
	err = client.Pull(ctx, ref, destDir)
	require.NoError(t, err)

	// Directory should be empty (only the root)
	entries, err := os.ReadDir(destDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestIntegration_LargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	// Create a ~5 MiB file (5,242,880 bytes)
	largeContent := bytes.Repeat([]byte("ABCDEFGHIJ"), 524288) // 10 * 524288 = 5,242,880 bytes
	srcFS := fstest.MapFS{
		"large.bin": &fstest.MapFile{
			Data: largeContent,
			Mode: 0644,
		},
	}

	ref := reg.Host + "/test/largefile:v1"
	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	// Pull and verify
	destDir := t.TempDir()
	err = client.Pull(ctx, ref, destDir)
	require.NoError(t, err)

	// Verify content
	pulledContent, err := os.ReadFile(filepath.Join(destDir, "large.bin"))
	require.NoError(t, err)
	assert.Equal(t, largeContent, pulledContent)
}

func TestIntegration_ClientWithUserAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(
		blobber.WithInsecure(true),
		blobber.WithUserAgent("blobber-test/1.0"),
	)
	require.NoError(t, err)

	ref := reg.Host + "/test/useragent:v1"
	srcFS := testFS()

	// Push should work with custom user agent
	digest, err := client.Push(ctx, ref, srcFS)
	require.NoError(t, err)
	assert.NotEmpty(t, digest)
}

func TestIntegration_DoubleClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := testContext(t)
	reg := setupRegistry(ctx, t)

	client, err := blobber.NewClient(blobber.WithInsecure(true))
	require.NoError(t, err)

	ref := reg.Host + "/test/doubleclose:v1"
	srcFS := testFS()
	_, err = client.Push(ctx, ref, srcFS)
	require.NoError(t, err)

	img, err := client.OpenImage(ctx, ref)
	require.NoError(t, err)

	// First close should succeed
	err = img.Close()
	require.NoError(t, err)

	// Second close should be safe (not panic or error)
	err = img.Close()
	assert.NoError(t, err)
}
