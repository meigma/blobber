package estargz

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memTOCCache struct {
	entry       TOCEntry
	toc         []byte
	ok          bool
	storeCalls  int
	storeEntry  TOCEntry
	storeTOC    []byte
	storeDigest digest.Digest
	footerOK    bool
	footerBytes []byte
	footerStore int
}

type countingSource struct {
	*countingReader
}

func (s *countingSource) Close() error {
	return nil
}

func (c *memTOCCache) Load(d digest.Digest) (TOCEntry, []byte, bool, error) {
	return c.entry, c.toc, c.ok, nil
}

func (c *memTOCCache) Store(d digest.Digest, entry TOCEntry, toc []byte) error {
	c.storeCalls++
	c.storeEntry = entry
	c.storeTOC = toc
	c.storeDigest = d
	return nil
}

func (c *memTOCCache) LoadFooter(d digest.Digest) ([]byte, bool, error) {
	return c.footerBytes, c.footerOK, nil
}

func (c *memTOCCache) StoreFooter(d digest.Digest, footer []byte) error {
	c.footerStore++
	c.footerBytes = footer
	c.footerOK = true
	return nil
}

func TestTOCCacheWrap_UsesCachedTOC(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("cached toc"), Mode: 0o644},
	}
	blob := buildTestBlob(t, src)

	metaReader := newSizedReader(blob)
	entry, ok := tocInfoFromFooter(metaReader)
	require.True(t, ok)

	tocBytes := make([]byte, entry.Size)
	_, err := metaReader.ReadAt(tocBytes, entry.Offset)
	require.NoError(t, err)

	cache := &memTOCCache{
		entry: entry,
		toc:   tocBytes,
		ok:    true,
	}

	counter := &countingSource{countingReader: &countingReader{sizedReader: newSizedReader(blob)}}
	wrapped := WrapWithTOCCache(counter, digest.FromString("blob"), cache)

	reader, err := NewReader(context.Background(), wrapped)
	require.NoError(t, err)
	t.Cleanup(func() { reader.Close() })

	assert.Equal(t, 1, counter.ReadCount())
	assert.Equal(t, 0, cache.storeCalls)
}

func TestTOCCacheWrap_UsesCachedFooterAndTOC(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("cached footer"), Mode: 0o644},
	}
	blob := buildTestBlob(t, src)

	metaReader := newSizedReader(blob)
	entry, ok := tocInfoFromFooter(metaReader)
	require.True(t, ok)

	tocBytes := make([]byte, entry.Size)
	_, err := metaReader.ReadAt(tocBytes, entry.Offset)
	require.NoError(t, err)

	footerSize := maxFooterSize(int64(len(blob)), footerDecompressors()...)
	footerOffset := int64(len(blob)) - footerSize
	footerBytes := blob[footerOffset:]

	cache := &memTOCCache{
		entry:       entry,
		toc:         tocBytes,
		ok:          true,
		footerOK:    true,
		footerBytes: append([]byte(nil), footerBytes...),
	}

	counter := &countingSource{countingReader: &countingReader{sizedReader: newSizedReader(blob)}}
	wrapped := WrapWithTOCCache(counter, digest.FromString("blob"), cache)

	reader, err := NewReader(context.Background(), wrapped)
	require.NoError(t, err)
	t.Cleanup(func() { reader.Close() })

	assert.Equal(t, 0, counter.ReadCount())
}

func TestTOCCacheWrap_StoresTOCOnMiss(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("store toc"), Mode: 0o644},
	}
	blob := buildTestBlob(t, src)

	cache := &memTOCCache{}
	counter := &countingSource{countingReader: &countingReader{sizedReader: newSizedReader(blob)}}
	wrapped := WrapWithTOCCache(counter, digest.FromString("blob"), cache)

	reader, err := NewReader(context.Background(), wrapped)
	require.NoError(t, err)
	t.Cleanup(func() { reader.Close() })

	assert.Equal(t, 1, cache.storeCalls)
	assert.Equal(t, digest.FromString("blob"), cache.storeDigest)
	assert.Greater(t, len(cache.storeTOC), 0)
	assert.Equal(t, int64(len(cache.storeTOC)), cache.storeEntry.Size)
	assert.GreaterOrEqual(t, counter.ReadCount(), 2)
}

func TestTOCCacheWrap_StoresFooterOnMiss(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("footer miss"), Mode: 0o644},
	}
	blob := buildTestBlob(t, src)

	cache := &memTOCCache{}
	counter := &countingSource{countingReader: &countingReader{sizedReader: newSizedReader(blob)}}
	wrapped := WrapWithTOCCache(counter, digest.FromString("blob"), cache)

	footerSize := maxFooterSize(int64(len(blob)), footerDecompressors()...)
	footerOffset := int64(len(blob)) - footerSize
	buf := make([]byte, footerSize)

	n, err := wrapped.ReadAt(buf, footerOffset)
	require.NoError(t, err)
	require.Equal(t, int(footerSize), n)

	assert.Equal(t, 1, cache.footerStore)
	assert.Equal(t, int(footerSize), len(cache.footerBytes))
}
