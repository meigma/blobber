// Package progress provides utilities for tracking I/O progress.
package progress

import "io"

// Callback is called to report progress during I/O operations.
type Callback func(bytesTransferred, totalBytes int64)

// Reader wraps an io.Reader to track bytes read and report progress.
type Reader struct {
	reader   io.Reader
	callback Callback
	total    int64
	read     int64
}

// NewReader creates a progress-tracking reader.
// The total parameter should be the expected size (-1 if unknown).
// The callback is called after each Read with cumulative bytes and total.
func NewReader(r io.Reader, total int64, callback Callback) *Reader {
	return &Reader{
		reader:   r,
		callback: callback,
		total:    total,
	}
}

// Read implements io.Reader and reports progress after each read.
func (r *Reader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	if n > 0 {
		r.read += int64(n)
		if r.callback != nil {
			r.callback(r.read, r.total)
		}
	}
	return n, err
}

// Close closes the underlying reader if it implements io.Closer.
func (r *Reader) Close() error {
	if closer, ok := r.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
