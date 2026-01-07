package archive

import (
	"context"
	"errors"
	"io"
)

// copyWithContext copies from src to dst while honoring context cancellation.
// It checks context every 128KB to balance responsiveness with performance.
func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader, buf []byte) error {
	if len(buf) < copyBufferSize {
		buf = make([]byte, copyBufferSize)
	}
	buf = buf[:copyBufferSize]
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

const copyBufferSize = 128 * 1024
