package tar

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
)

// Compile-time interface check.
var _ Writer = (*writer)(nil)

// WriterOption configures a writer.
type WriterOption func(*writer)

// WithWriterLogger sets the logger for the writer.
func WithWriterLogger(logger *slog.Logger) WriterOption {
	return func(w *writer) {
		w.logger = logger
	}
}

type writer struct {
	logger *slog.Logger
}

// NewWriter creates a new Writer with the given options.
func NewWriter(opts ...WriterOption) Writer {
	w := &writer{}
	for _, opt := range opts {
		opt(w)
	}
	if w.logger == nil {
		w.logger = slog.New(slog.DiscardHandler)
	}
	return w
}

// WriteTo writes a tar archive containing all entries from src to dst.
func (w *writer) WriteTo(ctx context.Context, dst io.Writer, src fs.FS) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	tw := tar.NewWriter(dst)

	err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		// Skip root directory entry.
		if path == "." {
			return nil
		}
		return w.addEntry(ctx, tw, src, path, d)
	})
	if err != nil {
		return err
	}

	return tw.Close()
}

// addEntry adds a single filesystem entry to the tar writer.
func (w *writer) addEntry(ctx context.Context, tw *tar.Writer, src fs.FS, path string, d fs.DirEntry) error {
	// Use ReadLinkFS when available to get symlink info without following.
	if rlfs, ok := src.(fs.ReadLinkFS); ok {
		info, err := rlfs.Lstat(path)
		if err != nil {
			return fmt.Errorf("lstat %s: %w", path, err)
		}
		return w.addEntryWithInfo(ctx, tw, rlfs, path, info)
	}

	// Fallback for filesystems without symlink support.
	if d.Type()&fs.ModeSymlink != 0 {
		return fmt.Errorf("symlink %s: filesystem does not support ReadLinkFS", path)
	}
	info, err := d.Info()
	if err != nil {
		return fmt.Errorf("info %s: %w", path, err)
	}
	return w.addEntryWithInfo(ctx, tw, src, path, info)
}

// addEntryWithInfo adds an entry using FileInfo.
func (w *writer) addEntryWithInfo(ctx context.Context, tw *tar.Writer, src fs.FS, path string, info fs.FileInfo) error {
	mode := info.Mode()

	if mode&fs.ModeSymlink != 0 {
		rlfs, ok := src.(fs.ReadLinkFS)
		if !ok {
			return fmt.Errorf("symlink %s: filesystem does not support ReadLinkFS", path)
		}
		return w.addSymlink(tw, rlfs, path, info)
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("header %s: %w", path, err)
	}
	header.Name = path

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write header %s: %w", path, err)
	}

	if mode.IsRegular() {
		if err := w.copyFileContent(ctx, tw, src, path); err != nil {
			return err
		}
	}

	w.logger.Debug("added entry", "path", path, "type", header.Typeflag, "size", header.Size)
	return nil
}

// addSymlink adds a symlink entry to the tar writer.
func (w *writer) addSymlink(tw *tar.Writer, src fs.ReadLinkFS, path string, info fs.FileInfo) error {
	target, err := src.ReadLink(path)
	if err != nil {
		return fmt.Errorf("readlink %s: %w", path, err)
	}

	header, err := tar.FileInfoHeader(info, target)
	if err != nil {
		return fmt.Errorf("header %s: %w", path, err)
	}
	header.Name = path

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write header %s: %w", path, err)
	}

	w.logger.Debug("added symlink", "path", path, "target", target)
	return nil
}

// copyFileContent copies file content from src to the tar writer.
func (w *writer) copyFileContent(ctx context.Context, tw *tar.Writer, src fs.FS, path string) error {
	f, err := src.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	copyErr := copyWithContext(ctx, tw, f)
	closeErr := f.Close()

	if copyErr != nil {
		return fmt.Errorf("copy %s: %w", path, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", path, closeErr)
	}
	return nil
}

// copyWithContext copies from src to dst while honoring context cancellation.
// It checks context every 128KB to balance responsiveness with performance.
func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) error {
	buf := make([]byte, 128*1024)
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
