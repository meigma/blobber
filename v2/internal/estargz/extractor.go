package estargz

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/klauspost/compress/zstd"

	"github.com/meigma/blobber/v2/internal/tar"
	"github.com/meigma/blobber/v2/internal/tar/validator"
)

// tocFilename is the eStargz Table of Contents entry name.
const tocFilename = "stargz.index.json"

// Compile-time interface check.
var _ Extractor = (*extractor)(nil)

// isTOCEntry reports whether the given path is a TOC entry.
// Handles both "stargz.index.json" and "./stargz.index.json" (common tar prefix).
func isTOCEntry(path string) bool {
	return path == tocFilename || path == "./"+tocFilename
}

type extractor struct {
	logger       *slog.Logger
	tarExtractor tar.Extractor
}

// ExtractorOption configures an Extractor.
type ExtractorOption func(*extractor)

// WithExtractorLogger sets the logger for the extractor.
func WithExtractorLogger(logger *slog.Logger) ExtractorOption {
	return func(e *extractor) {
		e.logger = logger
	}
}

// WithTarExtractor sets the tar extractor to use.
// If not provided, a default extractor is created with standard path validation.
func WithTarExtractor(te tar.Extractor) ExtractorOption {
	return func(e *extractor) {
		e.tarExtractor = te
	}
}

// NewExtractor creates a new Extractor with the given options.
func NewExtractor(opts ...ExtractorOption) Extractor {
	e := &extractor{}
	for _, opt := range opts {
		opt(e)
	}
	if e.logger == nil {
		e.logger = slog.New(slog.DiscardHandler)
	}
	if e.tarExtractor == nil {
		e.tarExtractor = tar.NewExtractor(validator.New())
	}
	return e
}

// Extract decompresses an eStargz archive and extracts it to destDir.
//
// The compression format (gzip or zstd) is auto-detected from magic bytes.
// The TOC entry (stargz.index.json) is automatically excluded from extraction.
func (e *extractor) Extract(ctx context.Context, r io.Reader, destDir string) error {
	decompressed, err := detectAndDecompress(r)
	if err != nil {
		return fmt.Errorf("decompress: %w", err)
	}
	defer decompressed.Close()

	filter := func(path string, isDir bool) bool {
		if isTOCEntry(path) {
			e.logger.Debug("skipping TOC entry", "path", path)
			return false
		}
		return true
	}

	if err := e.tarExtractor.Extract(ctx, decompressed, destDir, filter); err != nil {
		return fmt.Errorf("extract tar: %w", err)
	}

	return nil
}

// detectAndDecompress auto-detects the compression format and returns a decompressor.
func detectAndDecompress(r io.Reader) (io.ReadCloser, error) {
	// Read first 4 bytes to detect format (zstd magic is 4 bytes).
	buf := make([]byte, 4)
	n, err := io.ReadFull(r, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}

	// Prepend read bytes back.
	combined := io.MultiReader(bytes.NewReader(buf[:n]), r)

	// Detect format by magic bytes.
	if n >= 2 && buf[0] == 0x1f && buf[1] == 0x8b {
		// gzip magic: 0x1f 0x8b
		return gzip.NewReader(combined)
	}
	if n >= 4 && buf[0] == 0x28 && buf[1] == 0xb5 && buf[2] == 0x2f && buf[3] == 0xfd {
		// zstd magic: 0x28 0xb5 0x2f 0xfd
		decoder, err := zstd.NewReader(combined)
		if err != nil {
			return nil, err
		}
		return decoder.IOReadCloser(), nil
	}

	// Distinguish between truncated input and unrecognized format.
	if n < 2 {
		return nil, fmt.Errorf("input too short (%d bytes): need at least 2 bytes to detect compression format", n)
	}
	return nil, errors.New("unknown compression format")
}
