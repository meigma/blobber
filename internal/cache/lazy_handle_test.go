package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilmanlab/blobber/core"
)

func TestLazyHandle_ReadAt(t *testing.T) {
	t.Parallel()

	t.Run("fetches data on demand", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("lazy loading test content that is long enough")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// Open lazy handle
		handle, err := cache.OpenLazy(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		defer handle.Close()

		// Initially not complete
		assert.False(t, handle.Complete())
		assert.Equal(t, int64(len(data)), handle.Size())

		// Read the first 10 bytes - should trigger a fetch
		buf := make([]byte, 10)
		n, err := handle.ReadAt(buf, 0)
		require.NoError(t, err)
		assert.Equal(t, 10, n)
		assert.Equal(t, data[:10], buf)

		// Verify a range request was made
		require.Len(t, reg.rangeRequests, 1)
		assert.Equal(t, int64(0), reg.rangeRequests[0].offset)
		assert.Equal(t, int64(10), reg.rangeRequests[0].length)

		// Clear range requests and read same range again
		reg.rangeRequests = nil
		n, err = handle.ReadAt(buf, 0)
		require.NoError(t, err)
		assert.Equal(t, 10, n)
		assert.Equal(t, data[:10], buf)

		// No new range request should be made (cached)
		assert.Empty(t, reg.rangeRequests)
	})

	t.Run("reads from different offsets", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("0123456789abcdefghijklmnopqrstuvwxyz")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		handle, err := cache.OpenLazy(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		defer handle.Close()

		// Read from offset 10
		buf := make([]byte, 5)
		n, err := handle.ReadAt(buf, 10)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, data[10:15], buf)

		// Read from offset 20
		n, err = handle.ReadAt(buf, 20)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, data[20:25], buf)

		// Two range requests should have been made
		assert.Len(t, reg.rangeRequests, 2)
	})

	t.Run("becomes complete after reading all data", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("complete me")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		handle, err := cache.OpenLazy(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		defer handle.Close()

		assert.False(t, handle.Complete())

		// Read entire blob
		buf := make([]byte, len(data))
		n, err := handle.ReadAt(buf, 0)
		require.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, data, buf)

		// Should now be complete
		assert.True(t, handle.Complete())
	})

	t.Run("returns cached blob when complete", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("already cached content")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		// First, cache the complete blob via normal Open
		handle1, err := cache.Open(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		handle1.Close()

		// Remove from mock registry
		delete(reg.blobs, digest)

		// OpenLazy should return the cached blob
		handle2, err := cache.OpenLazy(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		defer handle2.Close()

		// Should be complete from cache
		assert.True(t, handle2.Complete())

		// Can still read
		buf := make([]byte, len(data))
		n, err := handle2.ReadAt(buf, 0)
		require.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, data, buf)
	})
}

func TestLazyHandle_ConcurrentReads(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create larger test data
	content := bytes.Repeat([]byte("concurrent test data "), 100)
	hash := sha256.Sum256(content)
	digest := "sha256:" + hex.EncodeToString(hash[:])

	reg := newMockRegistry()
	reg.addBlob(digest, content)

	cache, err := New(dir, reg, nil)
	require.NoError(t, err)

	desc := core.LayerDescriptor{
		Digest:    digest,
		Size:      int64(len(content)),
		MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
	}

	handle, err := cache.OpenLazy(context.Background(), "test.io/repo:tag", desc)
	require.NoError(t, err)
	defer handle.Close()

	// Spawn multiple goroutines reading different offsets
	done := make(chan bool, 10)
	for i := range 10 {
		offset := int64(i * 100)
		go func(off int64) {
			buf := make([]byte, 20)
			n, readErr := handle.ReadAt(buf, off)
			assert.NoError(t, readErr)
			assert.Equal(t, 20, n)
			assert.Equal(t, content[off:off+20], buf)
			done <- true
		}(offset)
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}
}

func TestLazyHandle_PersistsProgress(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	data, digest := createTestBlob("persistence test with enough length for ranges")
	reg := newMockRegistry()
	reg.addBlob(digest, data)

	desc := core.LayerDescriptor{
		Digest:    digest,
		Size:      int64(len(data)),
		MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
	}

	// First cache instance
	cache1, err := New(dir, reg, nil)
	require.NoError(t, err)

	handle1, err := cache1.OpenLazy(context.Background(), "test.io/repo:tag", desc)
	require.NoError(t, err)

	// Read first half
	buf := make([]byte, len(data)/2)
	n, err := handle1.ReadAt(buf, 0)
	require.NoError(t, err)
	assert.Equal(t, len(data)/2, n)

	// Close handle (saves progress)
	handle1.Close()

	// Clear range requests
	reg.rangeRequests = nil

	// Second cache instance (simulating process restart)
	cache2, err := New(dir, reg, nil)
	require.NoError(t, err)

	handle2, err := cache2.OpenLazy(context.Background(), "test.io/repo:tag", desc)
	require.NoError(t, err)
	defer handle2.Close()

	// Read first half again - should not trigger fetch (cached)
	n, err = handle2.ReadAt(buf, 0)
	require.NoError(t, err)
	assert.Equal(t, len(data)/2, n)
	assert.Empty(t, reg.rangeRequests, "should not fetch already cached range")

	// Read second half - should trigger fetch
	buf2 := make([]byte, len(data)-len(data)/2)
	_, err = handle2.ReadAt(buf2, int64(len(data)/2))
	require.NoError(t, err)
	assert.Len(t, reg.rangeRequests, 1, "should fetch second half")
}

func TestLazyHandle_HandleErrors(t *testing.T) {
	t.Parallel()

	t.Run("returns error when range fetch fails", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("error test content")
		reg := newMockRegistry()
		// Don't add blob to registry - will cause fetch error

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		handle, err := cache.OpenLazy(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		defer handle.Close()

		buf := make([]byte, 10)
		_, err = handle.ReadAt(buf, 0)
		assert.Error(t, err)
	})

	t.Run("returns error after close", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("close test")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		handle, err := cache.OpenLazy(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)

		handle.Close()

		buf := make([]byte, 10)
		_, err = handle.ReadAt(buf, 0)
		assert.ErrorIs(t, err, core.ErrClosed)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("context cancel test content")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		ctx, cancel := context.WithCancel(context.Background())
		handle, err := cache.OpenLazy(ctx, "test.io/repo:tag", desc)
		require.NoError(t, err)
		defer handle.Close()

		// Cancel context
		cancel()

		// Read should fail with context error
		buf := make([]byte, 10)
		_, err = handle.ReadAt(buf, 0)
		assert.Error(t, err)
	})
}

func TestFindGapsInRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ranges   []Range
		offset   int64
		length   int64
		expected []Range
	}{
		{
			name:     "empty ranges - entire range is gap",
			ranges:   nil,
			offset:   0,
			length:   100,
			expected: []Range{{Offset: 0, Length: 100}},
		},
		{
			name:     "no gap - fully covered",
			ranges:   []Range{{Offset: 0, Length: 100}},
			offset:   10,
			length:   20,
			expected: nil,
		},
		{
			name:     "gap at start",
			ranges:   []Range{{Offset: 50, Length: 50}},
			offset:   0,
			length:   100,
			expected: []Range{{Offset: 0, Length: 50}},
		},
		{
			name:     "gap at end",
			ranges:   []Range{{Offset: 0, Length: 50}},
			offset:   0,
			length:   100,
			expected: []Range{{Offset: 50, Length: 50}},
		},
		{
			name:     "gap in middle",
			ranges:   []Range{{Offset: 0, Length: 30}, {Offset: 70, Length: 30}},
			offset:   0,
			length:   100,
			expected: []Range{{Offset: 30, Length: 40}},
		},
		{
			name:     "partial overlap - gap before",
			ranges:   []Range{{Offset: 50, Length: 50}},
			offset:   30,
			length:   40,
			expected: []Range{{Offset: 30, Length: 20}},
		},
		{
			name:     "partial overlap - gap after",
			ranges:   []Range{{Offset: 0, Length: 50}},
			offset:   30,
			length:   40,
			expected: []Range{{Offset: 50, Length: 20}},
		},
		{
			name:     "zero length request",
			ranges:   nil,
			offset:   0,
			length:   0,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := findGapsInRange(tt.ranges, tt.offset, tt.length)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLazyHandle_Entry(t *testing.T) {
	t.Parallel()

	t.Run("entry is marked as lazy", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("lazy entry test")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		handle, err := cache.OpenLazy(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)
		handle.Close()

		// Load entry and verify lazy flag
		entryPath := filepath.Join(dir, "entries", "sha256", extractHash(digest)+".json")
		entry, err := loadEntry(entryPath)
		require.NoError(t, err)
		assert.True(t, entry.Lazy)
	})

	t.Run("ranges are tracked in entry", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		data, digest := createTestBlob("range tracking test with enough length")
		reg := newMockRegistry()
		reg.addBlob(digest, data)

		cache, err := New(dir, reg, nil)
		require.NoError(t, err)

		desc := core.LayerDescriptor{
			Digest:    digest,
			Size:      int64(len(data)),
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		}

		handle, err := cache.OpenLazy(context.Background(), "test.io/repo:tag", desc)
		require.NoError(t, err)

		// Read first 10 bytes
		buf := make([]byte, 10)
		_, err = handle.ReadAt(buf, 0)
		require.NoError(t, err)

		handle.Close()

		// Load entry and verify ranges
		entryPath := filepath.Join(dir, "entries", "sha256", extractHash(digest)+".json")
		entry, err := loadEntry(entryPath)
		require.NoError(t, err)
		require.Len(t, entry.Ranges, 1)
		assert.Equal(t, int64(0), entry.Ranges[0].Offset)
		assert.Equal(t, int64(10), entry.Ranges[0].Length)
	})
}

func TestOpenLazy_FilePersistence(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	data, digest := createTestBlob("file persistence verification test data")
	reg := newMockRegistry()
	reg.addBlob(digest, data)

	cache, err := New(dir, reg, nil)
	require.NoError(t, err)

	desc := core.LayerDescriptor{
		Digest:    digest,
		Size:      int64(len(data)),
		MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
	}

	// Open lazy, read some data, close
	handle, err := cache.OpenLazy(context.Background(), "test.io/repo:tag", desc)
	require.NoError(t, err)

	buf := make([]byte, 20)
	_, err = handle.ReadAt(buf, 0)
	require.NoError(t, err)
	handle.Close()

	// Verify partial file exists
	partialPath := filepath.Join(dir, "blobs", "sha256", extractHash(digest)+".partial")
	_, err = os.Stat(partialPath)
	require.NoError(t, err)

	// Read all data to complete
	handle2, err := cache.OpenLazy(context.Background(), "test.io/repo:tag", desc)
	require.NoError(t, err)

	fullBuf := make([]byte, len(data))
	_, err = handle2.ReadAt(fullBuf, 0)
	require.NoError(t, err)
	handle2.Close()

	// Verify complete file exists (no .partial suffix)
	blobPath := filepath.Join(dir, "blobs", "sha256", extractHash(digest))
	content, err := os.ReadFile(blobPath)
	require.NoError(t, err)
	assert.Equal(t, data, content)

	// Partial file should be gone
	_, err = os.Stat(partialPath)
	assert.True(t, os.IsNotExist(err))
}
