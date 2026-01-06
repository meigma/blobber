// Package archive provides eStargz creation and reading operations.
package archive

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"

	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/opencontainers/go-digest"

	"github.com/gilmanlab/blobber"
)

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


// Compile-time interface implementation check.
var _ blobber.ArchiveBuilder = (*builder)(nil)

// builder implements blobber.ArchiveBuilder using estargz.
type builder struct {
	logger *slog.Logger
}

// NewBuilder creates a new ArchiveBuilder.
// If logger is nil, a no-op logger is used.
func NewBuilder(logger *slog.Logger) *builder {
	if logger == nil {
		//nolint:sloglint // DiscardHandler is intentional for no-op logging
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &builder{logger: logger}
}

// Build creates an eStargz blob from the given filesystem.
// The tar data is streamed through a pipe to avoid buffering the uncompressed
// archive. The compressed output is written to a temporary file while computing
// the blob digest and size for efficient streaming push.
func (b *builder) Build(ctx context.Context, src fs.FS, compression blobber.Compression) (*blobber.BuildResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Create temp file for compressed estargz output
	esgzFile, err := os.CreateTemp("", "blobber-esgz-*")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	esgzPath := esgzFile.Name()

	// Cleanup helper for error cases
	cleanup := func() {
		esgzFile.Close()
		os.Remove(esgzPath)
	}

	// Wrap temp file with digesting writer to compute blob digest while writing
	dw := newDigestingWriter(esgzFile)

	// Create pipe for streaming tar data
	pr, pw := io.Pipe()

	// Error channel for goroutine (buffered to prevent blocking)
	errCh := make(chan error, 1)

	// Goroutine writes tar entries to pipe
	go func() {
		tarErr := writeTarToPipe(ctx, pw, src)
		errCh <- tarErr
	}()

	// Create estargz writer using the digesting wrapper
	writer := estargz.NewWriterWithCompressor(dw, compression)

	// Use AppendTarLossLess to preserve exact tar bytes.
	if err = writer.AppendTarLossLess(pr); err != nil {
		pr.Close() // Unblock goroutine if it's still writing
		<-errCh    // Wait for goroutine to finish
		cleanup()
		return nil, fmt.Errorf("build estargz: %w", err)
	}

	var tocDigest digest.Digest
	tocDigest, err = writer.Close()
	if err != nil {
		pr.Close()
		<-errCh
		cleanup()
		return nil, fmt.Errorf("close estargz writer: %w", err)
	}
	diffID := writer.DiffID()

	// Wait for goroutine and check for errors
	if tarErr := <-errCh; tarErr != nil {
		cleanup()
		return nil, fmt.Errorf("create tar: %w", tarErr)
	}

	// Seek to beginning for reading
	if _, err := esgzFile.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, fmt.Errorf("seek temp file: %w", err)
	}

	return &blobber.BuildResult{
		Blob: &tempFileReader{
			File:   esgzFile,
			path:   esgzPath,
			logger: b.logger,
		},
		TOCDigest:  tocDigest.String(),
		DiffID:     diffID,
		BlobDigest: dw.Digest().String(),
		BlobSize:   dw.Size(),
	}, nil
}

// tempFileReader wraps a file and removes it on close.
type tempFileReader struct {
	*os.File
	path   string
	logger *slog.Logger
}

func (r *tempFileReader) Close() error {
	closeErr := r.File.Close()
	if err := os.Remove(r.path); err != nil && !os.IsNotExist(err) {
		r.logger.Warn("failed to remove temp file", "path", r.path, "error", err)
	}
	return closeErr
}

// writeTarToPipe writes tar entries from src to the pipe writer.
// It closes the pipe when done, propagating any error.
func writeTarToPipe(ctx context.Context, pw *io.PipeWriter, src fs.FS) error {
	var tarErr error
	defer func() {
		if tarErr != nil {
			pw.CloseWithError(tarErr)
		} else {
			pw.Close()
		}
	}()

	tw := tar.NewWriter(pw)

	tarErr = fs.WalkDir(src, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return addEntryToTar(ctx, tw, src, path, d)
	})
	if tarErr != nil {
		return tarErr
	}

	tarErr = tw.Close()
	return tarErr
}

// readLinkFS is the interface for filesystems that support reading symlink targets.
// This mirrors fs.ReadLinkFS which was added in Go 1.23.
type readLinkFS interface {
	fs.FS
	ReadLink(name string) (string, error)
}

// lstatFS is the interface for filesystems that support Lstat (stat without following symlinks).
type lstatFS interface {
	fs.FS
	Lstat(name string) (fs.FileInfo, error)
}

// addEntryToTar adds a single filesystem entry to the tar writer.
func addEntryToTar(ctx context.Context, tw *tar.Writer, src fs.FS, path string, d fs.DirEntry) error {
	// Prefer Lstat when available to avoid following symlinks.
	if lfs, ok := src.(lstatFS); ok {
		info, err := lfs.Lstat(path)
		if err != nil {
			return fmt.Errorf("lstat %s: %w", path, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return addSymlinkToTar(tw, src, path, info)
		}
		return addFileInfoEntry(ctx, tw, src, path, info)
	}

	// Handle symlinks specially to avoid following the link
	if d.Type()&fs.ModeSymlink != 0 {
		return addSymlinkToTar(tw, src, path, nil)
	}

	// For non-symlinks, d.Info() is safe to use
	info, err := d.Info()
	if err != nil {
		return err
	}

	return addFileInfoEntry(ctx, tw, src, path, info)
}

// addSymlinkToTar adds a symlink entry to the tar writer.
// It uses Lstat to get the symlink's own metadata (not following the link)
// and ReadLink to get the target path.
func addSymlinkToTar(tw *tar.Writer, src fs.FS, path string, info fs.FileInfo) error {
	if info == nil {
		var err error
		// Get symlink metadata without following
		info, err = lstat(src, path)
		if err != nil {
			return fmt.Errorf("lstat %s: %w", path, err)
		}
	}

	// Get symlink target
	target, err := readLink(src, path)
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, target)
	if err != nil {
		return err
	}
	header.Name = path

	return tw.WriteHeader(header)
}

func addFileInfoEntry(ctx context.Context, tw *tar.Writer, src fs.FS, path string, info fs.FileInfo) error {
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = path

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	// Copy file content for regular files
	if info.Mode().IsRegular() {
		if err := copyFileToTar(ctx, src, path, tw); err != nil {
			return err
		}
	}
	return nil
}

// lstat returns FileInfo for a path without following symlinks.
// Returns an error if the filesystem doesn't support Lstat.
func lstat(src fs.FS, path string) (fs.FileInfo, error) {
	if lfs, ok := src.(lstatFS); ok {
		return lfs.Lstat(path)
	}
	return nil, errors.New("filesystem does not support Lstat")
}

// readLink returns the target of a symlink.
// Returns an error if the filesystem doesn't support ReadLink.
func readLink(src fs.FS, path string) (string, error) {
	rlfs, ok := src.(readLinkFS)
	if !ok {
		return "", fmt.Errorf("symlink %s: filesystem does not support ReadLink", path)
	}

	target, err := rlfs.ReadLink(path)
	if err != nil {
		return "", fmt.Errorf("readlink %s: %w", path, err)
	}
	return target, nil
}

// copyFileToTar copies a file from the filesystem to the tar writer.
// It handles closing the file immediately to avoid FD leaks in loops.
// Context cancellation is checked every 32KB during the copy.
func copyFileToTar(ctx context.Context, src fs.FS, path string, tw *tar.Writer) error {
	f, err := src.Open(path)
	if err != nil {
		return err
	}
	copyErr := copyWithContext(ctx, tw, f)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
