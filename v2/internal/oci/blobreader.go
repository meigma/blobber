package oci

import (
	"context"
	"io"
	"sync/atomic"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// BlobReader provides random access to a remote blob via HTTP range requests.
//
// It implements io.ReaderAt and provides Size() for use with estargz.Reader.
// Each ReadAt call issues one HTTP range request.
//
// BlobReader is safe for concurrent ReadAt calls. Each call is independent
// and uses no shared mutable state.
type BlobReader struct {
	ctx    context.Context
	client *client
	ref    string
	desc   ocispec.Descriptor
	closed atomic.Bool
}

// newBlobReader creates a new BlobReader.
//
//nolint:gocritic // hugeParam: desc passed by value to match OCI ecosystem patterns
func newBlobReader(ctx context.Context, c *client, ref string, desc ocispec.Descriptor) *BlobReader {
	return &BlobReader{
		ctx:    ctx,
		client: c,
		ref:    ref,
		desc:   desc,
	}
}

// ReadAt implements io.ReaderAt.
// Each call issues one HTTP range request for the specified byte range.
func (r *BlobReader) ReadAt(p []byte, off int64) (n int, err error) {
	if r.closed.Load() {
		return 0, ErrClosed
	}
	if ctxErr := r.ctx.Err(); ctxErr != nil {
		return 0, ctxErr
	}
	if len(p) == 0 {
		return 0, nil
	}

	// Check for read past end of blob.
	if off >= r.desc.Size {
		return 0, io.EOF
	}

	// Clamp read length to blob size.
	length := int64(len(p))
	if off+length > r.desc.Size {
		length = r.desc.Size - off
	}

	rc, err := r.client.FetchBlobRange(r.ctx, r.ref, r.desc, off, length)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	// Read exactly the requested bytes.
	n, err = io.ReadFull(rc, p[:length])
	if err != nil && err != io.ErrUnexpectedEOF {
		return n, err
	}

	// If we read less than the buffer size due to EOF, signal it.
	if int64(n) < int64(len(p)) {
		return n, io.EOF
	}

	return n, nil
}

// Size returns the total size of the blob in bytes.
func (r *BlobReader) Size() int64 {
	return r.desc.Size
}

// Close marks the reader as closed.
// After Close, ReadAt calls will return ErrClosed.
func (r *BlobReader) Close() error {
	r.closed.Store(true)
	return nil
}
