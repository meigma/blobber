package estargz

import (
	"errors"
	"io"
	"sync"

	stargz "github.com/containerd/stargz-snapshotter/estargz"
	"github.com/containerd/stargz-snapshotter/estargz/zstdchunked"
	"github.com/opencontainers/go-digest"
)

// TOCEntry describes the TOC byte range within a blob.
type TOCEntry struct {
	Offset int64
	Size   int64
}

// TOCCache stores and retrieves TOC bytes for a blob digest.
type TOCCache interface {
	// Load returns cached TOC bytes and their range.
	Load(digest.Digest) (TOCEntry, []byte, bool, error)

	// Store writes TOC bytes for the given range.
	Store(digest.Digest, TOCEntry, []byte) error

	// LoadFooter returns cached footer bytes.
	LoadFooter(digest.Digest) ([]byte, bool, error)

	// StoreFooter writes footer bytes.
	StoreFooter(digest.Digest, []byte) error
}

// WrapWithTOCCache wraps a BlobSource to serve cached TOC bytes when available.
func WrapWithTOCCache(src BlobSource, blobDigest digest.Digest, cache TOCCache) BlobSource {
	if cache == nil {
		return src
	}
	return &tocCachedSource{
		src:        src,
		blobDigest: blobDigest,
		cache:      cache,
	}
}

type tocCachedSource struct {
	src        BlobSource
	blobDigest digest.Digest
	cache      TOCCache

	tocOnce   sync.Once
	tocEntry  TOCEntry
	tocBytes  []byte
	tocCached bool
	tocReady  bool

	footerOnce   sync.Once
	footerMu     sync.Mutex
	footerEntry  TOCEntry
	footerBytes  []byte
	footerCached bool
	footerReady  bool
}

func (s *tocCachedSource) ReadAt(p []byte, off int64) (int, error) {
	footerEntry, footerBytes, footerCached, footerOK := s.footerInfo()
	if footerOK && isFooterRequest(off, len(p), footerEntry) {
		if footerCached {
			copy(p, footerBytes)
			if len(footerBytes) < len(p) {
				return len(footerBytes), io.EOF
			}
			return len(footerBytes), nil
		}

		n, err := s.src.ReadAt(p, off)
		if err == nil && int64(n) == footerEntry.Size {
			footerCopy := append([]byte(nil), p[:n]...)
			s.setFooterBytes(footerCopy, true)
			_ = s.cache.StoreFooter(s.blobDigest, footerCopy)
		}
		return n, err
	}

	entry, tocBytes, cached, ok := s.tocInfo()
	if ok && cached && isTOCRequest(off, len(p), entry) {
		copy(p, tocBytes)
		if len(tocBytes) < len(p) {
			return len(tocBytes), io.EOF
		}
		return len(tocBytes), nil
	}

	n, err := s.src.ReadAt(p, off)
	if ok && !cached && isTOCRequest(off, len(p), entry) && err == nil && int64(n) == entry.Size {
		_ = s.cache.Store(s.blobDigest, entry, append([]byte(nil), p[:n]...))
	}
	return n, err
}

func (s *tocCachedSource) Size() int64 {
	return s.src.Size()
}

func (s *tocCachedSource) Close() error {
	return s.src.Close()
}

func (s *tocCachedSource) tocInfo() (TOCEntry, []byte, bool, bool) {
	s.tocOnce.Do(func() {
		entry, tocBytes, ok, err := s.cache.Load(s.blobDigest)
		if err == nil && ok && int64(len(tocBytes)) == entry.Size && entry.Size > 0 {
			s.tocEntry = entry
			s.tocBytes = tocBytes
			s.tocCached = true
			s.tocReady = true
			return
		}

		if entry, ok = s.tocEntryFromFooter(); ok {
			s.tocEntry = entry
			s.tocReady = true
		}
	})

	return s.tocEntry, s.tocBytes, s.tocCached, s.tocReady
}

func isTOCRequest(off int64, size int, entry TOCEntry) bool {
	return off == entry.Offset && int64(size) == entry.Size
}

func isFooterRequest(off int64, size int, entry TOCEntry) bool {
	return off == entry.Offset && int64(size) == entry.Size
}

func (s *tocCachedSource) tocEntryFromFooter() (TOCEntry, bool) {
	footerEntry, footerBytes, cached, ok := s.footerInfo()
	if !ok {
		return TOCEntry{}, false
	}

	if !cached || int64(len(footerBytes)) != footerEntry.Size {
		footerBytes, ok = s.readFooterBytes(footerEntry)
		if !ok {
			return TOCEntry{}, false
		}
	}

	return tocInfoFromFooterBytes(s.src.Size(), footerBytes)
}

func (s *tocCachedSource) footerInfo() (TOCEntry, []byte, bool, bool) {
	s.footerOnce.Do(func() {
		entry, ok := footerRange(s.src)
		if !ok {
			return
		}
		s.footerEntry = entry
		s.footerReady = true

		footerBytes, ok, err := s.cache.LoadFooter(s.blobDigest)
		if err == nil && ok && int64(len(footerBytes)) == entry.Size && entry.Size > 0 {
			s.setFooterBytes(footerBytes, true)
		}
	})

	s.footerMu.Lock()
	defer s.footerMu.Unlock()

	return s.footerEntry, s.footerBytes, s.footerCached, s.footerReady
}

func (s *tocCachedSource) readFooterBytes(entry TOCEntry) ([]byte, bool) {
	if entry.Size <= 0 {
		return nil, false
	}

	buf := make([]byte, entry.Size)
	n, err := s.src.ReadAt(buf, entry.Offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false
	}
	if int64(n) != entry.Size {
		return nil, false
	}

	buf = append([]byte(nil), buf[:n]...)
	s.setFooterBytes(buf, true)
	_ = s.cache.StoreFooter(s.blobDigest, buf)

	return buf, true
}

func (s *tocCachedSource) setFooterBytes(b []byte, cached bool) {
	s.footerMu.Lock()
	defer s.footerMu.Unlock()

	if len(b) == 0 {
		return
	}

	s.footerBytes = b
	s.footerCached = cached
}

func footerRange(src SizedReaderAt) (TOCEntry, bool) {
	decompressors := footerDecompressors()
	footerSize := maxFooterSize(src.Size(), decompressors...)
	if footerSize == 0 {
		return TOCEntry{}, false
	}
	offset := src.Size() - footerSize
	if offset < 0 {
		return TOCEntry{}, false
	}
	return TOCEntry{Offset: offset, Size: footerSize}, true
}

func tocInfoFromFooter(src SizedReaderAt) (TOCEntry, bool) {
	footerEntry, ok := footerRange(src)
	if !ok {
		return TOCEntry{}, false
	}

	footer := make([]byte, footerEntry.Size)
	if _, err := src.ReadAt(footer, footerEntry.Offset); err != nil && !errors.Is(err, io.EOF) {
		return TOCEntry{}, false
	}

	return tocInfoFromFooterBytes(src.Size(), footer)
}

func tocInfoFromFooterBytes(blobSize int64, footer []byte) (TOCEntry, bool) {
	decompressors := footerDecompressors()

	for _, d := range decompressors {
		fSize := d.FooterSize()
		if fSize > int64(len(footer)) {
			continue
		}
		fOffset := int64(len(footer)) - fSize
		_, tocOffset, tocSize, err := d.ParseFooter(footer[fOffset:])
		if err != nil || tocOffset < 0 {
			continue
		}
		if tocSize <= 0 {
			tocSize = blobSize - tocOffset - fSize
		}
		if tocOffset < 0 || tocSize <= 0 {
			continue
		}
		return TOCEntry{Offset: tocOffset, Size: tocSize}, true
	}

	return TOCEntry{}, false
}

func footerDecompressors() []stargz.Decompressor {
	return []stargz.Decompressor{
		new(stargz.GzipDecompressor),
		new(stargz.LegacyGzipDecompressor),
		new(zstdchunked.Decompressor),
	}
}

func maxFooterSize(blobSize int64, decompressors ...stargz.Decompressor) int64 {
	var max int64
	for _, d := range decompressors {
		if size := d.FooterSize(); size > max && size <= blobSize {
			max = size
		}
	}
	return max
}
