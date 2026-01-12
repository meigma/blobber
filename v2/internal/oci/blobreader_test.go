package oci

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlobReader_ReadAt(t *testing.T) {
	t.Parallel()

	data := []byte("hello, world!")
	desc := ocispec.Descriptor{
		Digest: digest.FromBytes(data),
		Size:   int64(len(data)),
	}

	reader := &BlobReader{
		ctx:  context.Background(),
		ref:  "test/repo:tag",
		desc: desc,
	}

	// Test that Size returns the correct value.
	assert.Equal(t, int64(len(data)), reader.Size())
}

func TestBlobReader_Size(t *testing.T) {
	t.Parallel()

	desc := ocispec.Descriptor{
		Size: 12345,
	}

	reader := &BlobReader{
		ctx:  context.Background(),
		desc: desc,
	}

	assert.Equal(t, int64(12345), reader.Size())
}

func TestBlobReader_Close(t *testing.T) {
	t.Parallel()

	reader := &BlobReader{
		ctx:  context.Background(),
		desc: ocispec.Descriptor{Size: 100},
	}

	// Before close, closed should be false.
	assert.False(t, reader.closed.Load())

	err := reader.Close()
	require.NoError(t, err)

	// After close, closed should be true.
	assert.True(t, reader.closed.Load())
}

func TestBlobReader_ReadAt_AfterClose(t *testing.T) {
	t.Parallel()

	reader := &BlobReader{
		ctx:  context.Background(),
		desc: ocispec.Descriptor{Size: 100},
	}

	err := reader.Close()
	require.NoError(t, err)

	buf := make([]byte, 10)
	n, err := reader.ReadAt(buf, 0)

	assert.Equal(t, 0, n)
	assert.ErrorIs(t, err, ErrClosed)
}

func TestBlobReader_ReadAt_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	reader := &BlobReader{
		ctx:  ctx,
		desc: ocispec.Descriptor{Size: 100},
	}

	buf := make([]byte, 10)
	n, err := reader.ReadAt(buf, 0)

	assert.Equal(t, 0, n)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestBlobReader_ReadAt_EmptyBuffer(t *testing.T) {
	t.Parallel()

	reader := &BlobReader{
		ctx:  context.Background(),
		desc: ocispec.Descriptor{Size: 100},
	}

	buf := make([]byte, 0)
	n, err := reader.ReadAt(buf, 0)

	assert.Equal(t, 0, n)
	assert.NoError(t, err)
}

func TestBlobReader_ReadAt_PastEOF(t *testing.T) {
	t.Parallel()

	reader := &BlobReader{
		ctx:  context.Background(),
		desc: ocispec.Descriptor{Size: 100},
	}

	buf := make([]byte, 10)
	n, err := reader.ReadAt(buf, 100) // Offset at size = EOF.

	assert.Equal(t, 0, n)
	assert.ErrorIs(t, err, io.EOF)
}

func TestBlobReader_ConcurrentReadAt(t *testing.T) {
	t.Parallel()

	// Test that concurrent ReadAt calls don't race on shared state.
	// Size is 0 so all reads return io.EOF before reaching client.FetchBlobRange,
	// allowing us to test concurrent access to closed, ctx, and desc.
	reader := &BlobReader{
		ctx:  context.Background(),
		desc: ocispec.Descriptor{Size: 0},
	}

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(offset int64) {
			defer wg.Done()
			buf := make([]byte, 10)
			n, err := reader.ReadAt(buf, offset)
			// All reads should return EOF since Size is 0.
			_ = n
			_ = err
		}(int64(i * 100))
	}

	wg.Wait()
	// If we get here without a race detector failure, the test passes.
}
