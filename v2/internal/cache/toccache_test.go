package cache

import (
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/v2/internal/estargz"
)

func TestTOCCache_StoreLoad(t *testing.T) {
	t.Parallel()

	fileCache, err := NewFileCache(t.TempDir())
	require.NoError(t, err)

	tocCache := NewTOCCache(fileCache)
	d := digest.FromString("blob")
	entry := estargz.TOCEntry{Offset: 10, Size: 4}
	data := []byte("toc!")

	require.NoError(t, tocCache.Store(d, entry, data))

	loadedEntry, loadedData, ok, err := tocCache.Load(d)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, entry, loadedEntry)
	assert.Equal(t, data, loadedData)
}

func TestTOCCache_LoadMissing(t *testing.T) {
	t.Parallel()

	fileCache, err := NewFileCache(t.TempDir())
	require.NoError(t, err)

	tocCache := NewTOCCache(fileCache)
	d := digest.FromString("blob")

	_, _, ok, err := tocCache.Load(d)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestTOCCache_FooterStoreLoad(t *testing.T) {
	t.Parallel()

	fileCache, err := NewFileCache(t.TempDir())
	require.NoError(t, err)

	tocCache := NewTOCCache(fileCache)
	d := digest.FromString("blob")
	footer := []byte("footer")

	require.NoError(t, tocCache.StoreFooter(d, footer))

	loaded, ok, err := tocCache.LoadFooter(d)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, footer, loaded)
}
