//go:build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/meigma/blobber/v2"
	"github.com/meigma/blobber/v2/internal/testutils"
)

var benchRefCounter int64

func BenchmarkPull_NoCache(b *testing.B) {
	benchmarkPull(b, false)
}

func BenchmarkPull_FileCacheWarm(b *testing.B) {
	benchmarkPull(b, true)
}

func BenchmarkStream_NoCache(b *testing.B) {
	benchmarkStream(b, false)
}

func BenchmarkStream_FileCacheWarm(b *testing.B) {
	benchmarkStream(b, true)
}

func benchmarkPull(b *testing.B, warmCache bool) {
	b.Helper()

	ctx := context.Background()
	ref := benchmarkRef(b, "pull")
	srcFS := testutils.TestFS()

	opts := []blobber.ClientOption{
		blobber.WithPlainHTTP(true),
	}

	if warmCache {
		cacheDir := b.TempDir()
		opts = append(opts,
			blobber.WithRefCache(cacheDir, time.Hour),
			blobber.WithFileCache(cacheDir),
		)
	}

	client := blobber.NewClient(opts...)

	_, err := client.Push(ctx, ref, srcFS)
	require.NoError(b, err)

	if warmCache {
		handle, err := client.Pull(ctx, ref)
		require.NoError(b, err)
		require.NoError(b, handle.Close())
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		handle, err := client.Pull(ctx, ref)
		require.NoError(b, err)
		require.NoError(b, handle.Close())
	}
}

func benchmarkStream(b *testing.B, warmCache bool) {
	b.Helper()

	ctx := context.Background()
	ref := benchmarkRef(b, "stream")
	srcFS := testutils.TestFS()

	opts := []blobber.ClientOption{
		blobber.WithPlainHTTP(true),
	}

	if warmCache {
		cacheDir := b.TempDir()
		opts = append(opts, blobber.WithFileCache(cacheDir))
	}

	client := blobber.NewClient(opts...)

	_, err := client.Push(ctx, ref, srcFS)
	require.NoError(b, err)

	if warmCache {
		handle, err := client.Stream(ctx, ref)
		require.NoError(b, err)
		readHello(b, handle)
		require.NoError(b, handle.Close())
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		handle, err := client.Stream(ctx, ref)
		require.NoError(b, err)
		readHello(b, handle)
		require.NoError(b, handle.Close())
	}
}

func readHello(b *testing.B, handle *blobber.BlobHandle) {
	b.Helper()

	f, err := handle.Open("hello.txt")
	require.NoError(b, err)

	_, err = io.Copy(io.Discard, f)
	require.NoError(b, err)

	require.NoError(b, f.Close())
}

func benchmarkRef(b *testing.B, suffix string) string {
	b.Helper()

	name := strings.ToLower(b.Name())
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, " ", "-")

	counter := atomic.AddInt64(&benchRefCounter, 1)
	if suffix == "" {
		suffix = "blob"
	}

	return fmt.Sprintf("%s/%s-%d/%s:v1", registry.Host, name, counter, suffix)
}
