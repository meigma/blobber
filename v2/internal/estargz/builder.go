package estargz

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"

	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/opencontainers/go-digest"

	"github.com/meigma/blobber/v2/internal/tar"
)

// Compile-time interface check.
var _ Builder = (*builder)(nil)

// BuildResult contains metadata from a Build operation.
type BuildResult struct {
	// BlobDigest is the SHA-256 digest of the compressed blob.
	BlobDigest digest.Digest

	// BlobSize is the size of the compressed blob in bytes.
	BlobSize int64

	// UncompressedDigest is the SHA-256 digest of the uncompressed tar content.
	UncompressedDigest digest.Digest

	// TOCDigest is the SHA-256 digest of the Table of Contents.
	TOCDigest digest.Digest
}

type builder struct {
	logger      *slog.Logger
	compression estargz.Compressor
}

// BuilderOption configures a Builder.
type BuilderOption func(*builder)

// WithBuilderLogger sets the logger for the builder.
func WithBuilderLogger(logger *slog.Logger) BuilderOption {
	return func(b *builder) {
		b.logger = logger
	}
}

// WithCompression sets the compression algorithm.
// If not specified, gzip compression is used.
func WithCompression(c estargz.Compressor) BuilderOption {
	return func(b *builder) {
		b.compression = c
	}
}

// NewBuilder creates a new Builder with the given options.
func NewBuilder(opts ...BuilderOption) Builder {
	b := &builder{}
	for _, opt := range opts {
		opt(b)
	}
	if b.logger == nil {
		b.logger = slog.New(slog.DiscardHandler)
	}
	if b.compression == nil {
		b.compression = estargz.NewGzipCompressor()
	}
	return b
}

// Build creates an eStargz archive from the given filesystem.
//
// The method walks the filesystem, creates a tar archive, and compresses it
// using the eStargz format. The compressed output is written to dst.
func (b *builder) Build(ctx context.Context, dst io.Writer, src fs.FS) (*BuildResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Derive a cancelable context to interrupt the tar goroutine on error.
	// This prevents hangs when src.Read blocks and an error occurs.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Wrap dst to compute blob digest while writing.
	dw := newDigestingWriter(dst)

	// Create pipe for streaming tar data to estargz writer.
	pr, pw := io.Pipe()

	// Error channel for the tar-writing goroutine.
	errCh := make(chan error, 1)

	go func() {
		errCh <- b.writeTarToPipe(ctx, pw, src)
	}()

	// Create estargz writer.
	writer := estargz.NewWriterWithCompressor(dw, b.compression)

	// AppendTarLossLess preserves exact tar bytes for reproducibility.
	if err := writer.AppendTarLossLess(pr); err != nil {
		cancel()
		pr.Close()
		tarErr := <-errCh
		return nil, joinTarError(fmt.Errorf("build estargz: %w", err), tarErr)
	}

	tocDigest, err := writer.Close()
	if err != nil {
		cancel()
		pr.Close()
		tarErr := <-errCh
		return nil, joinTarError(fmt.Errorf("close estargz writer: %w", err), tarErr)
	}
	diffID, err := digest.Parse(writer.DiffID())
	if err != nil {
		return nil, fmt.Errorf("parse diffID: %w", err)
	}

	// Wait for tar goroutine and check for errors.
	if tarErr := <-errCh; tarErr != nil {
		return nil, fmt.Errorf("create tar: %w", tarErr)
	}

	return &BuildResult{
		BlobDigest:         dw.Digest(),
		BlobSize:           dw.Size(),
		UncompressedDigest: diffID,
		TOCDigest:          tocDigest,
	}, nil
}

// joinTarError joins the primary error with the tar error if the tar error
// provides additional context (i.e., is not just a context cancellation).
func joinTarError(primary, tarErr error) error {
	if tarErr == nil || errors.Is(tarErr, context.Canceled) {
		return primary
	}
	return errors.Join(primary, tarErr)
}

// writeTarToPipe writes tar entries from src to the pipe writer.
// It closes the pipe when done, propagating any error.
func (b *builder) writeTarToPipe(ctx context.Context, pw *io.PipeWriter, src fs.FS) error {
	tw := tar.NewWriter(tar.WithWriterLogger(b.logger))
	err := tw.WriteTo(ctx, pw, src)
	if err != nil {
		pw.CloseWithError(err)
	} else {
		pw.Close()
	}
	return err
}

// digestingWriter computes digest and size while writing.
type digestingWriter struct {
	w        io.Writer
	digester digest.Digester
	size     int64
}

func newDigestingWriter(w io.Writer) *digestingWriter {
	return &digestingWriter{
		w:        w,
		digester: digest.SHA256.Digester(),
	}
}

func (d *digestingWriter) Write(p []byte) (int, error) {
	n, err := d.w.Write(p)
	if n > 0 {
		d.digester.Hash().Write(p[:n])
		d.size += int64(n)
	}
	return n, err
}

func (d *digestingWriter) Digest() digest.Digest { return d.digester.Digest() }
func (d *digestingWriter) Size() int64           { return d.size }
