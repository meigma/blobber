package cache

import (
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefCache_LookupMiss(t *testing.T) {
	t.Parallel()

	cache, err := NewRefCache(t.TempDir(), time.Hour)
	require.NoError(t, err)

	d, ok := cache.Lookup("ghcr.io/org/repo:v1")
	assert.False(t, ok)
	assert.Equal(t, digest.Digest(""), d)
}

func TestRefCache_StoreAndLookup(t *testing.T) {
	t.Parallel()

	cache, err := NewRefCache(t.TempDir(), time.Hour)
	require.NoError(t, err)

	ref := "ghcr.io/org/repo:v1"
	expected := digest.FromString("test-content")

	cache.Store(ref, expected)

	d, ok := cache.Lookup(ref)
	assert.True(t, ok)
	assert.Equal(t, expected, d)
}

func TestRefCache_StoreOverwrites(t *testing.T) {
	t.Parallel()

	cache, err := NewRefCache(t.TempDir(), time.Hour)
	require.NoError(t, err)

	ref := "ghcr.io/org/repo:latest"
	first := digest.FromString("first")
	second := digest.FromString("second")

	cache.Store(ref, first)
	d, ok := cache.Lookup(ref)
	require.True(t, ok)
	assert.Equal(t, first, d)

	cache.Store(ref, second)
	d, ok = cache.Lookup(ref)
	require.True(t, ok)
	assert.Equal(t, second, d)
}

func TestRefCache_TTLExpiry(t *testing.T) {
	t.Parallel()

	// Use short but not too short TTL for testing.
	// 100ms is long enough for file I/O but short enough to test expiry.
	cache, err := NewRefCache(t.TempDir(), 100*time.Millisecond)
	require.NoError(t, err)

	ref := "ghcr.io/org/repo:v1"
	expected := digest.FromString("content")

	cache.Store(ref, expected)

	// Should be fresh immediately.
	d, ok := cache.Lookup(ref)
	assert.True(t, ok)
	assert.Equal(t, expected, d)

	// Wait for TTL to expire.
	time.Sleep(150 * time.Millisecond)

	// Should be stale now.
	_, ok = cache.Lookup(ref)
	assert.False(t, ok)
}

func TestRefCache_DifferentRefs(t *testing.T) {
	t.Parallel()

	cache, err := NewRefCache(t.TempDir(), time.Hour)
	require.NoError(t, err)

	ref1 := "ghcr.io/org/repo:v1"
	ref2 := "ghcr.io/org/repo:v2"
	digest1 := digest.FromString("v1-content")
	digest2 := digest.FromString("v2-content")

	cache.Store(ref1, digest1)
	cache.Store(ref2, digest2)

	d1, ok := cache.Lookup(ref1)
	require.True(t, ok)
	assert.Equal(t, digest1, d1)

	d2, ok := cache.Lookup(ref2)
	require.True(t, ok)
	assert.Equal(t, digest2, d2)
}

func TestRefCache_SpecialCharactersInRef(t *testing.T) {
	t.Parallel()

	cache, err := NewRefCache(t.TempDir(), time.Hour)
	require.NoError(t, err)

	// Refs with special characters that would be problematic as filenames.
	refs := []string{
		"ghcr.io/org/repo:v1.2.3",
		"ghcr.io/org/repo@sha256:abc123",
		"localhost:5000/my-repo:latest",
		"registry.example.com/path/to/repo:tag",
	}

	for _, ref := range refs {
		expected := digest.FromString(ref)
		cache.Store(ref, expected)

		d, ok := cache.Lookup(ref)
		assert.True(t, ok, "ref: %s", ref)
		assert.Equal(t, expected, d, "ref: %s", ref)
	}
}
