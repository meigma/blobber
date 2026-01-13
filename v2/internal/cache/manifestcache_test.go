package cache

import (
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestCache_LoadMiss(t *testing.T) {
	t.Parallel()

	cache, err := NewManifestCache(t.TempDir())
	require.NoError(t, err)

	_, ok := cache.Load(digest.FromString("missing"))
	assert.False(t, ok)
}

func TestManifestCache_StoreAndLoad(t *testing.T) {
	t.Parallel()

	cache, err := NewManifestCache(t.TempDir())
	require.NoError(t, err)

	raw := []byte(`{"schemaVersion":2}`)
	d := digest.FromBytes(raw)

	err = cache.Store(d, raw)
	require.NoError(t, err)

	got, ok := cache.Load(d)
	assert.True(t, ok)
	assert.Equal(t, raw, got)
}

func TestManifestCache_StoreEmptyNoop(t *testing.T) {
	t.Parallel()

	cache, err := NewManifestCache(t.TempDir())
	require.NoError(t, err)

	d := digest.FromString("empty")
	err = cache.Store(d, nil)
	require.NoError(t, err)

	_, ok := cache.Load(d)
	assert.False(t, ok)
}
