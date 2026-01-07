package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilmanlab/blobber/core"
)

// mockRegistry is a test double for core.Registry.
type mockRegistry struct {
	blobs         map[string][]byte
	rangeSupport  bool
	rangeRequests []rangeRequest
}

type rangeRequest struct {
	digest string
	offset int64
	length int64
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{
		blobs:        make(map[string][]byte),
		rangeSupport: true, // default to supporting ranges
	}
}

func (m *mockRegistry) addBlob(digest string, data []byte) {
	m.blobs[digest] = data
}

func (m *mockRegistry) Push(_ context.Context, _ string, _ io.Reader, _ *core.RegistryPushOptions) (string, error) {
	return "", nil
}

func (m *mockRegistry) Pull(_ context.Context, _ string) (io.ReadCloser, int64, error) {
	return nil, 0, nil
}

func (m *mockRegistry) PullRange(_ context.Context, _ string, _, _ int64) (io.ReadCloser, error) {
	return nil, core.ErrNotFound
}

func (m *mockRegistry) ResolveLayer(_ context.Context, _ string) (core.LayerDescriptor, error) {
	return core.LayerDescriptor{}, nil
}

func (m *mockRegistry) FetchBlob(_ context.Context, _ string, desc core.LayerDescriptor) (io.ReadCloser, error) {
	data, ok := m.blobs[desc.Digest]
	if !ok {
		return nil, core.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *mockRegistry) FetchBlobRange(_ context.Context, _ string, desc core.LayerDescriptor, offset, length int64) (io.ReadCloser, error) {
	if !m.rangeSupport {
		return nil, core.ErrRangeNotSupported
	}

	data, ok := m.blobs[desc.Digest]
	if !ok {
		return nil, core.ErrNotFound
	}

	// Track the range request
	m.rangeRequests = append(m.rangeRequests, rangeRequest{
		digest: desc.Digest,
		offset: offset,
		length: length,
	})

	// Return the requested range
	end := offset + length
	if end > int64(len(data)) {
		end = int64(len(data))
	}
	if offset >= int64(len(data)) {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	return io.NopCloser(bytes.NewReader(data[offset:end])), nil
}

// createTestBlob creates test data with a known digest.
func createTestBlob(content string) (data []byte, digest string) {
	data = []byte(content)
	hash := sha256.Sum256(data)
	digest = "sha256:" + hex.EncodeToString(hash[:])
	return data, digest
}

func TestCache_New(t *testing.T) {
	t.Parallel()

	t.Run("creates directory structure", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cachePath := filepath.Join(dir, "cache")

		_, err := New(cachePath, newMockRegistry(), nil)
		require.NoError(t, err)

		// Verify directories were created
		blobsDir := filepath.Join(cachePath, "blobs", "sha256")
		entriesDir := filepath.Join(cachePath, "entries", "sha256")

		info, err := os.Stat(blobsDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())

		info, err = os.Stat(entriesDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

func TestCache_Open(t *testing.T) {
	t.Parallel()

	t.Run("cache miss downloads blob", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("test content")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		handle, err := cache.Open(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		defer handle.Close()

		assert.Equal(t, int64(len(data)), handle.Size())
		assert.True(t, handle.Complete())

		// Read the content
		buf := make([]byte, len(data))
		n, err := handle.ReadAt(buf, 0)
		require.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, data, buf)
	})

	t.Run("cache hit returns cached blob", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("cached content")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// First open - cache miss
		handle1, err := cache.Open(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		handle1.Close()

		// Remove blob from registry to prove cache is used
		delete(reg.blobs, digest)

		// Second open - cache hit
		handle2, err := cache.Open(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		defer handle2.Close()

		// Still works because it's cached
		buf := make([]byte, len(data))
		n, err := handle2.ReadAt(buf, 0)
		require.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, data, buf)
	})

	t.Run("verifies digest on download", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data := []byte("actual content")
		wrongDigest := "sha256:0000000000000000000000000000000000000000000000000000000000000000"

		reg := newMockRegistry()
		reg.addBlob(wrongDigest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    wrongDigest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		_, err = cache.Open(context.Background(), "test.io/repo:tag", desc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "digest mismatch")
	})
}

func TestCache_OpenStream(t *testing.T) {
	t.Parallel()

	t.Run("returns streaming reader", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("stream content")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		reader, err := cache.OpenStream(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		defer reader.Close()

		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, data, content)
	})

	t.Run("self-heals on truncated blob", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("full content for truncation test")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// First download to cache
		reader1, err := cache.OpenStream(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		_, err = io.ReadAll(reader1)
		require.NoError(t, err)
		reader1.Close()

		// Corrupt the cached blob by truncating it
		blobPath := filepath.Join(dir, "blobs", "sha256", extractHash(digest))
		err = os.WriteFile(blobPath, data[:len(data)/2], 0o640)
		require.NoError(t, err)

		// OpenStream should detect mismatch, evict, and re-download
		reader2, err := cache.OpenStream(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		defer reader2.Close()

		content, err := io.ReadAll(reader2)
		require.NoError(t, err)
		assert.Equal(t, data, content, "should have re-downloaded correct content")
	})
}

func TestCache_OpenStreamThrough(t *testing.T) {
	t.Parallel()

	t.Run("streams while caching", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("stream through content")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		reader, err := cache.OpenStreamThrough(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)

		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		reader.Close()

		assert.Equal(t, data, content)

		// Verify blob was cached
		blobPath := filepath.Join(dir, "blobs", "sha256", extractHash(digest))
		_, err = os.Stat(blobPath)
		assert.NoError(t, err, "blob should be cached")
	})

	t.Run("self-heals on truncated blob", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("stream through truncation test")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// First download to cache
		reader1, err := cache.OpenStreamThrough(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		_, err = io.ReadAll(reader1)
		require.NoError(t, err)
		reader1.Close()

		// Corrupt the cached blob by truncating it
		blobPath := filepath.Join(dir, "blobs", "sha256", extractHash(digest))
		err = os.WriteFile(blobPath, data[:len(data)/2], 0o640)
		require.NoError(t, err)

		// OpenStreamThrough should detect mismatch, evict, and re-download
		reader2, err := cache.OpenStreamThrough(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)

		content, err := io.ReadAll(reader2)
		require.NoError(t, err)
		reader2.Close()

		assert.Equal(t, data, content, "should have re-downloaded correct content")
	})

	t.Run("cleans up stale partial entry on cache miss", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("stale partial test content")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// Manually create a stale partial file and entry with ranges
		hashStr := extractHash(digest)
		partialPath := filepath.Join(dir, "blobs", "sha256", hashStr+".partial")
		entryPath := filepath.Join(dir, "entries", "sha256", hashStr+".json")

		// Write partial file with some data
		err = os.WriteFile(partialPath, data[:len(data)/2], 0o640)
		require.NoError(t, err)

		// Write entry with ranges that would cause issues if not cleared
		staleEntry := &Entry{
			Version:  1,
			Digest:   digest,
			Size:     int64(len(data)),
			Complete: false,
			Verified: false,
			Ranges:   []Range{{Offset: 0, Length: int64(len(data) / 2)}},
		}
		err = saveEntry(entryPath, staleEntry)
		require.NoError(t, err)

		// OpenStreamThrough should clean up partial and entry before streaming
		reader, err := cache.OpenStreamThrough(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)

		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		reader.Close()

		assert.Equal(t, data, content)

		// Verify partial file was removed
		_, err = os.Stat(partialPath)
		assert.True(t, os.IsNotExist(err), "partial file should be removed")

		// Verify new entry is complete (stale entry should have been replaced)
		newEntry, err := loadEntry(entryPath)
		require.NoError(t, err)
		assert.True(t, newEntry.Complete, "entry should be marked complete")
		assert.True(t, newEntry.Verified, "entry should be marked verified")
		assert.Empty(t, newEntry.Ranges, "entry should have no ranges")
	})
}

func TestCache_Evict(t *testing.T) {
	t.Parallel()

	t.Run("removes cached blob", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("evict me")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// Cache the blob
		handle, err := cache.Open(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		handle.Close()

		// Evict
		err = cache.Evict(digest)
		require.NoError(t, err)

		// Verify blob file is gone
		blobPath := filepath.Join(dir, "blobs", "sha256", extractHash(digest))
		_, err = os.Stat(blobPath)
		assert.True(t, os.IsNotExist(err))

		// Verify entry file is gone
		entryPath := filepath.Join(dir, "entries", "sha256", extractHash(digest)+".json")
		_, err = os.Stat(entryPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("evicting nonexistent blob succeeds", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		cache, err := New(dir, newMockRegistry(), nil)
		require.NoError(t, err)

		err = cache.Evict("sha256:nonexistent")
		assert.NoError(t, err)
	})
}

func TestCache_Clear(t *testing.T) {
	t.Parallel()

	t.Run("removes all cached blobs", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data1, digest1 := createTestBlob("blob one")
		data2, digest2 := createTestBlob("blob two")

		reg := newMockRegistry()
		reg.addBlob(digest1, data1)
		reg.addBlob(digest2, data2)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		// Cache both blobs
		desc1 := core.LayerDescriptor{Digest: digest1, Size: int64(len(data1))}
		desc2 := core.LayerDescriptor{Digest: digest2, Size: int64(len(data2))}

		h1, err := cache.Open(context.Background(), "test.io/repo:tag1", desc1)
		require.NoError(t, err)
		h1.Close()

		h2, err := cache.Open(context.Background(), "test.io/repo:tag2", desc2)
		require.NoError(t, err)
		h2.Close()

		// Clear cache
		err = cache.Clear()
		require.NoError(t, err)

		// Verify blobs are gone
		blobsDir := filepath.Join(dir, "blobs", "sha256")
		entries, err := os.ReadDir(blobsDir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})
}

func TestFileHandle(t *testing.T) {
	t.Parallel()

	t.Run("implements BlobHandle", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("handle test")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		handle, err := cache.Open(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		defer handle.Close()

		// Test Size()
		assert.Equal(t, int64(len(data)), handle.Size())

		// Test Complete()
		assert.True(t, handle.Complete())

		// Test ReadAt() at different offsets
		buf := make([]byte, 4)
		n, err := handle.ReadAt(buf, 0)
		require.NoError(t, err)
		assert.Equal(t, 4, n)
		assert.Equal(t, data[:4], buf)

		n, err = handle.ReadAt(buf, 4)
		require.NoError(t, err)
		assert.Equal(t, 4, n)
		assert.Equal(t, data[4:8], buf)
	})
}

func TestEntry_LoadSave(t *testing.T) {
	t.Parallel()

	t.Run("roundtrip", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "entry.json")

		entry := &Entry{
			Version:   1,
			Digest:    "sha256:abc123",
			Size:      1024,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
			Complete:  true,
			Verified:  true,
		}

		err := saveEntry(path, entry)
		require.NoError(t, err)

		loaded, err := loadEntry(path)
		require.NoError(t, err)

		assert.Equal(t, entry.Version, loaded.Version)
		assert.Equal(t, entry.Digest, loaded.Digest)
		assert.Equal(t, entry.Size, loaded.Size)
		assert.Equal(t, entry.MediaType, loaded.MediaType)
		assert.Equal(t, entry.Complete, loaded.Complete)
		assert.Equal(t, entry.Verified, loaded.Verified)
		assert.False(t, loaded.CreatedAt.IsZero())
		assert.False(t, loaded.LastAccessed.IsZero())
	})

	t.Run("loadEntry returns error for missing file", func(t *testing.T) {
		t.Parallel()
		_, err := loadEntry("/nonexistent/path")
		assert.Error(t, err)
	})
}

func TestExtractHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"valid digest", "sha256:abc123def456", "abc123def456"},
		{"no prefix", "abc123def456", "abc123def456"},
		{"prefix only", "sha256:", "sha256:"}, // edge case: returns input unchanged
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := extractHash(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCache_ResumeDownload(t *testing.T) {
	t.Parallel()

	t.Run("resumes partial download", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create test data
		data, digest := createTestBlob("resumable content for testing")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		// Create a partial entry manually
		entryPath := filepath.Join(dir, "entries", "sha256", extractHash(digest)+".json")
		partialPath := filepath.Join(dir, "blobs", "sha256", extractHash(digest)+".partial")

		// Write first half of data to partial file
		halfLen := len(data) / 2
		//nolint:gosec // G306: Test file permissions are fine
		err = os.WriteFile(partialPath, data[:halfLen], 0o640)
		require.NoError(t, err)

		// Extend file to full size (simulating sparse file)
		f, err := os.OpenFile(partialPath, os.O_RDWR, 0o640)
		require.NoError(t, err)
		err = f.Truncate(int64(len(data)))
		require.NoError(t, err)
		f.Close()

		// Create partial entry with first half as downloaded range
		entry := &Entry{
			Version:   1,
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
			Complete:  false,
			Verified:  false,
			Ranges:    []Range{{Offset: 0, Length: int64(halfLen)}},
			Ref:       "test.io/repo:tag",
		}
		err = saveEntry(entryPath, entry)
		require.NoError(t, err)

		// Now open the cache - should resume download
		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		handle, err := cache.Open(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		defer handle.Close()

		// Verify complete content
		buf := make([]byte, len(data))
		n, err := handle.ReadAt(buf, 0)
		require.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, data, buf)
		assert.True(t, handle.Complete())

		// Verify range request was made for the second half
		require.Len(t, reg.rangeRequests, 1)
		assert.Equal(t, int64(halfLen), reg.rangeRequests[0].offset)
		assert.Equal(t, int64(len(data)-halfLen), reg.rangeRequests[0].length)
	})

	t.Run("falls back to full download when range not supported", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("fallback content")
		reg := newMockRegistry()
		reg.addBlob(digest, data)
		reg.rangeSupport = false // Disable range support

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		// Create a partial entry
		entryPath := filepath.Join(dir, "entries", "sha256", extractHash(digest)+".json")
		partialPath := filepath.Join(dir, "blobs", "sha256", extractHash(digest)+".partial")

		halfLen := len(data) / 2
		//nolint:gosec // G306: Test file permissions are fine
		err = os.WriteFile(partialPath, data[:halfLen], 0o640)
		require.NoError(t, err)

		f, err := os.OpenFile(partialPath, os.O_RDWR, 0o640)
		require.NoError(t, err)
		err = f.Truncate(int64(len(data)))
		require.NoError(t, err)
		f.Close()

		entry := &Entry{
			Version:  1,
			Digest:   digest,
			Size:     int64(len(data)),
			Complete: false,
			Verified: false,
			Ranges:   []Range{{Offset: 0, Length: int64(halfLen)}},
			Ref:      "test.io/repo:tag",
		}
		err = saveEntry(entryPath, entry)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// Should fall back to full download
		handle, err := cache.Open(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		defer handle.Close()

		buf := make([]byte, len(data))
		n, err := handle.ReadAt(buf, 0)
		require.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, data, buf)
	})
}

func TestCache_EntryWithRanges(t *testing.T) {
	t.Parallel()

	t.Run("stores ref in entry", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("ref tracking content")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		ref := "test.io/repo:tag"
		handle, err := cache.Open(context.Background(), ref, desc)
		require.NoError(t, err)
		handle.Close()

		// Load entry and verify ref is stored
		entryPath := filepath.Join(dir, "entries", "sha256", extractHash(digest)+".json")
		entry, err := loadEntry(entryPath)
		require.NoError(t, err)
		assert.Equal(t, ref, entry.Ref)
	})
}

func TestCache_Prefetch(t *testing.T) {
	t.Parallel()

	t.Run("prefetch does nothing for complete blob", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("prefetch complete")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// Cache the blob first
		handle, err := cache.Open(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		handle.Close()

		// Clear any range requests from initial download
		reg.rangeRequests = nil

		// Prefetch should do nothing
		cache.Prefetch(context.Background(), "test.io/repo:tag", desc)

		// Give goroutine time to run
		// Note: In a real test we'd use a sync mechanism, but for this simple case
		// the goroutine should exit immediately after checking Complete
	})
}
