package archive

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"path"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/stretchr/testify/require"
)

type datasetConfig struct {
	fileCount int
	minSize   int
	maxSize   int
	depth     int
	seed      int64
}

type dataset struct {
	files []testFile
	dirs  []string
	paths []string
}

type testFile struct {
	path string
	data []byte
}

func samplePaths(data dataset, count int, seed int64) []string {
	if count >= len(data.paths) {
		return data.paths
	}

	r := rand.New(rand.NewSource(seed))
	paths := make([]string, count)
	for i := 0; i < count; i++ {
		paths[i] = data.paths[r.Intn(len(data.paths))]
	}
	return paths
}

func buildDataset(cfg datasetConfig) dataset {
	r := rand.New(rand.NewSource(cfg.seed))
	dirs := make(map[string]struct{})
	files := make([]testFile, 0, cfg.fileCount)
	paths := make([]string, 0, cfg.fileCount)

	for i := 0; i < cfg.fileCount; i++ {
		segments := make([]string, 0, cfg.depth)
		for d := 0; d < cfg.depth; d++ {
			segments = append(segments, fmt.Sprintf("d%02d", r.Intn(64)))
		}
		fileName := fmt.Sprintf("file%05d.bin", i)
		fullPath := path.Join(append(segments, fileName)...)
		paths = append(paths, fullPath)

		size := cfg.minSize
		if cfg.maxSize > cfg.minSize {
			size = cfg.minSize + r.Intn(cfg.maxSize-cfg.minSize+1)
		}
		buf := make([]byte, size)
		_, _ = r.Read(buf)
		files = append(files, testFile{path: fullPath, data: buf})

		for depth := 1; depth <= cfg.depth; depth++ {
			dirPath := strings.Join(segments[:depth], "/")
			dirs[dirPath] = struct{}{}
		}
	}

	dirList := make([]string, 0, len(dirs))
	for dir := range dirs {
		dirList = append(dirList, dir)
	}
	sort.Strings(dirList)

	return dataset{files: files, dirs: dirList, paths: paths}
}

func buildArchive(t testing.TB, layout Layout, data dataset) ([]byte, []byte) {
	t.Helper()

	var dataBuf bytes.Buffer
	builder := NewBuilder(
		WithLayout(layout),
		WithDataWriter(&dataBuf),
	)

	now := time.Unix(1_700_000_000, 0).UTC()
	for _, dir := range data.dirs {
		info := testFileInfo{name: path.Base(dir), mode: 0o755 | fs.ModeDir, modTime: now}
		require.NoError(t, builder.AddDir(dir, info))
	}

	for _, file := range data.files {
		info := testFileInfo{name: path.Base(file.path), mode: 0o644, size: int64(len(file.data)), modTime: now}
		require.NoError(t, builder.AddFile(file.path, info, bytes.NewReader(file.data)))
	}

	var indexBuf bytes.Buffer
	_, err := builder.Finalize(&indexBuf)
	require.NoError(t, err)

	return indexBuf.Bytes(), dataBuf.Bytes()
}

func buildEstargz(t testing.TB, data dataset) []byte {
	t.Helper()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	now := time.Unix(1_700_000_000, 0).UTC()

	for _, dir := range data.dirs {
		hdr := &tar.Header{
			Name:     dir + "/",
			Mode:     0o755,
			Typeflag: tar.TypeDir,
			ModTime:  now,
		}
		require.NoError(t, tw.WriteHeader(hdr))
	}

	for _, file := range data.files {
		hdr := &tar.Header{
			Name:    file.path,
			Mode:    0o644,
			Size:    int64(len(file.data)),
			ModTime: now,
		}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write(file.data)
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())

	var out bytes.Buffer
	w := estargz.NewWriter(&out)
	require.NoError(t, w.AppendTarLossLess(bytes.NewReader(tarBuf.Bytes())))
	_, err := w.Close()
	require.NoError(t, err)

	return out.Bytes()
}

type countingReaderAt struct {
	r     *bytes.Reader
	reads int64
	bytes int64
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	c.reads++
	n, err := c.r.ReadAt(p, off)
	c.bytes += int64(n)
	return n, err
}

func (c *countingReaderAt) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

func BenchmarkIndexParseA(b *testing.B) {
	data := buildDataset(datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
	indexBytes, _ := buildArchive(b, LayoutA, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr := &countingReaderAt{r: bytes.NewReader(indexBytes)}
		_, err := OpenIndex(cr, int64(len(indexBytes)))
		require.NoError(b, err)
		b.ReportMetric(float64(cr.reads), "reads")
		b.ReportMetric(float64(cr.bytes), "bytes")
	}
}

func BenchmarkIndexParseA50k(b *testing.B) {
	data := buildDataset(datasetConfig{fileCount: 50_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
	indexBytes, _ := buildArchive(b, LayoutA, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr := &countingReaderAt{r: bytes.NewReader(indexBytes)}
		_, err := OpenIndex(cr, int64(len(indexBytes)))
		require.NoError(b, err)
		b.ReportMetric(float64(cr.reads), "reads")
		b.ReportMetric(float64(cr.bytes), "bytes")
	}
}

func BenchmarkIndexParseB(b *testing.B) {
	data := buildDataset(datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
	indexBytes, _ := buildArchive(b, LayoutB, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr := &countingReaderAt{r: bytes.NewReader(indexBytes)}
		_, err := OpenIndex(cr, int64(len(indexBytes)))
		require.NoError(b, err)
		b.ReportMetric(float64(cr.reads), "reads")
		b.ReportMetric(float64(cr.bytes), "bytes")
	}
}

func BenchmarkIndexParseB50k(b *testing.B) {
	data := buildDataset(datasetConfig{fileCount: 50_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
	indexBytes, _ := buildArchive(b, LayoutB, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr := &countingReaderAt{r: bytes.NewReader(indexBytes)}
		_, err := OpenIndex(cr, int64(len(indexBytes)))
		require.NoError(b, err)
		b.ReportMetric(float64(cr.reads), "reads")
		b.ReportMetric(float64(cr.bytes), "bytes")
	}
}

func BenchmarkIndexLookupA(b *testing.B) {
	data := buildDataset(datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
	indexBytes, _ := buildArchive(b, LayoutA, data)

	idx, err := OpenIndex(bytes.NewReader(indexBytes), int64(len(indexBytes)))
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := data.paths[i%len(data.paths)]
		_, ok, err := idx.Lookup(path)
		require.NoError(b, err)
		if !ok {
			b.Fatalf("missing path: %s", path)
		}
	}
}

func BenchmarkIndexLookupA_Cached(b *testing.B) {
	benchmarkIndexLookupCached(b, LayoutA, datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkIndexLookupA50k(b *testing.B) {
	data := buildDataset(datasetConfig{fileCount: 50_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
	indexBytes, _ := buildArchive(b, LayoutA, data)

	idx, err := OpenIndex(bytes.NewReader(indexBytes), int64(len(indexBytes)))
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := data.paths[i%len(data.paths)]
		_, ok, err := idx.Lookup(path)
		require.NoError(b, err)
		if !ok {
			b.Fatalf("missing path: %s", path)
		}
	}
}

func BenchmarkIndexLookupA50k_Cached(b *testing.B) {
	benchmarkIndexLookupCached(b, LayoutA, datasetConfig{fileCount: 50_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkIndexLookupB(b *testing.B) {
	data := buildDataset(datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
	indexBytes, _ := buildArchive(b, LayoutB, data)

	idx, err := OpenIndex(bytes.NewReader(indexBytes), int64(len(indexBytes)))
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := data.paths[i%len(data.paths)]
		_, ok, err := idx.Lookup(path)
		require.NoError(b, err)
		if !ok {
			b.Fatalf("missing path: %s", path)
		}
	}
}

func BenchmarkIndexLookupB_Cached(b *testing.B) {
	benchmarkIndexLookupCached(b, LayoutB, datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkIndexLookupB50k(b *testing.B) {
	data := buildDataset(datasetConfig{fileCount: 50_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
	indexBytes, _ := buildArchive(b, LayoutB, data)

	idx, err := OpenIndex(bytes.NewReader(indexBytes), int64(len(indexBytes)))
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := data.paths[i%len(data.paths)]
		_, ok, err := idx.Lookup(path)
		require.NoError(b, err)
		if !ok {
			b.Fatalf("missing path: %s", path)
		}
	}
}

func BenchmarkIndexLookupB50k_Cached(b *testing.B) {
	benchmarkIndexLookupCached(b, LayoutB, datasetConfig{fileCount: 50_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkEstargzParse(b *testing.B) {
	data := buildDataset(datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
	blob := buildEstargz(b, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr := &countingReaderAt{r: bytes.NewReader(blob)}
		sr := io.NewSectionReader(cr, 0, int64(len(blob)))
		_, err := estargz.Open(sr)
		require.NoError(b, err)
		b.ReportMetric(float64(cr.reads), "reads")
		b.ReportMetric(float64(cr.bytes), "bytes")
	}
}

func BenchmarkIndexLookupA_Normalized(b *testing.B) {
	benchmarkIndexLookupNormalized(b, LayoutA, datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkIndexLookupB_Normalized(b *testing.B) {
	benchmarkIndexLookupNormalized(b, LayoutB, datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkIndexLookupA_NormalizedMap(b *testing.B) {
	benchmarkIndexLookupNormalizedMap(b, LayoutA, datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkIndexLookupB_NormalizedMap(b *testing.B) {
	benchmarkIndexLookupNormalizedMap(b, LayoutB, datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkIndexLookupA_NormalizedIndex(b *testing.B) {
	benchmarkIndexLookupNormalizedIndex(b, LayoutA, datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkIndexLookupB_NormalizedIndex(b *testing.B) {
	benchmarkIndexLookupNormalizedIndex(b, LayoutB, datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkEstargzParse50k(b *testing.B) {
	data := buildDataset(datasetConfig{fileCount: 50_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
	blob := buildEstargz(b, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr := &countingReaderAt{r: bytes.NewReader(blob)}
		sr := io.NewSectionReader(cr, 0, int64(len(blob)))
		_, err := estargz.Open(sr)
		require.NoError(b, err)
		b.ReportMetric(float64(cr.reads), "reads")
		b.ReportMetric(float64(cr.bytes), "bytes")
	}
}

func BenchmarkEstargzLookup(b *testing.B) {
	data := buildDataset(datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
	blob := buildEstargz(b, data)

	sr := io.NewSectionReader(bytes.NewReader(blob), 0, int64(len(blob)))
	reader, err := estargz.Open(sr)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := data.paths[i%len(data.paths)]
		_, ok := reader.Lookup(path)
		if !ok {
			b.Fatalf("missing path: %s", path)
		}
	}
}

func BenchmarkEstargzLookup50k(b *testing.B) {
	data := buildDataset(datasetConfig{fileCount: 50_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
	blob := buildEstargz(b, data)

	sr := io.NewSectionReader(bytes.NewReader(blob), 0, int64(len(blob)))
	reader, err := estargz.Open(sr)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := data.paths[i%len(data.paths)]
		_, ok := reader.Lookup(path)
		if !ok {
			b.Fatalf("missing path: %s", path)
		}
	}
}

func BenchmarkIndexOpenAndLookupA(b *testing.B) {
	benchmarkIndexOpenAndLookup(b, LayoutA, datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkIndexOpenAndLookupA50k(b *testing.B) {
	benchmarkIndexOpenAndLookup(b, LayoutA, datasetConfig{fileCount: 50_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkIndexOpenAndLookupB(b *testing.B) {
	benchmarkIndexOpenAndLookup(b, LayoutB, datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkIndexOpenAndLookupB50k(b *testing.B) {
	benchmarkIndexOpenAndLookup(b, LayoutB, datasetConfig{fileCount: 50_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkEstargzOpenAndLookup(b *testing.B) {
	benchmarkEstargzOpenAndLookup(b, datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkEstargzOpenAndLookup50k(b *testing.B) {
	benchmarkEstargzOpenAndLookup(b, datasetConfig{fileCount: 50_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42})
}

func BenchmarkIndexListAndOpenA(b *testing.B) {
	benchmarkIndexListAndOpen(b, LayoutA, datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42}, 2_000, 100)
}

func BenchmarkIndexListAndOpenB(b *testing.B) {
	benchmarkIndexListAndOpen(b, LayoutB, datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42}, 2_000, 100)
}

func BenchmarkEstargzListAndOpen(b *testing.B) {
	benchmarkEstargzListAndOpen(b, datasetConfig{fileCount: 10_000, minSize: 1024, maxSize: 8 * 1024, depth: 3, seed: 42}, 2_000, 100)
}

func benchmarkIndexOpenAndLookup(b *testing.B, layout Layout, cfg datasetConfig) {
	b.Helper()

	data := buildDataset(cfg)
	indexBytes, _ := buildArchive(b, layout, data)
	target := data.paths[len(data.paths)/2]

	var reads int64
	var bytesRead int64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr := &countingReaderAt{r: bytes.NewReader(indexBytes)}
		idx, err := OpenIndex(cr, int64(len(indexBytes)))
		require.NoError(b, err)
		_, ok, err := idx.Lookup(target)
		require.NoError(b, err)
		if !ok {
			b.Fatalf("missing path: %s", target)
		}
		reads = cr.reads
		bytesRead = cr.bytes
	}

	b.ReportMetric(float64(reads), "reads")
	b.ReportMetric(float64(bytesRead), "bytes")
}

func benchmarkIndexLookupCached(b *testing.B, layout Layout, cfg datasetConfig) {
	b.Helper()

	data := buildDataset(cfg)
	indexBytes, _ := buildArchive(b, layout, data)

	idx, err := OpenIndex(bytes.NewReader(indexBytes), int64(len(indexBytes)), WithLookupCache())
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := data.paths[i%len(data.paths)]
		_, ok, err := idx.Lookup(path)
		require.NoError(b, err)
		if !ok {
			b.Fatalf("missing path: %s", path)
		}
	}
}

func benchmarkIndexLookupNormalized(b *testing.B, layout Layout, cfg datasetConfig) {
	b.Helper()

	data := buildDataset(cfg)
	indexBytes, _ := buildArchive(b, layout, data)

	idx, err := OpenIndex(bytes.NewReader(indexBytes), int64(len(indexBytes)), WithLookupCache())
	require.NoError(b, err)

	normalized := make([]string, len(data.paths))
	for i, p := range data.paths {
		clean, err := NormalizePath(p)
		require.NoError(b, err)
		normalized[i] = clean
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := normalized[i%len(normalized)]
		_, ok, err := idx.LookupNormalized(path)
		require.NoError(b, err)
		if !ok {
			b.Fatalf("missing path: %s", path)
		}
	}
}

func benchmarkIndexLookupNormalizedMap(b *testing.B, layout Layout, cfg datasetConfig) {
	b.Helper()

	data := buildDataset(cfg)
	indexBytes, _ := buildArchive(b, layout, data)

	idx, err := OpenIndex(bytes.NewReader(indexBytes), int64(len(indexBytes)))
	require.NoError(b, err)
	require.NoError(b, idx.BuildLookupMap())

	normalized := make([]string, len(data.paths))
	for i, p := range data.paths {
		clean, err := NormalizePath(p)
		require.NoError(b, err)
		normalized[i] = clean
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := normalized[i%len(normalized)]
		_, ok, err := idx.LookupNormalized(path)
		require.NoError(b, err)
		if !ok {
			b.Fatalf("missing path: %s", path)
		}
	}
}

func benchmarkIndexLookupNormalizedIndex(b *testing.B, layout Layout, cfg datasetConfig) {
	b.Helper()

	data := buildDataset(cfg)
	indexBytes, _ := buildArchive(b, layout, data)

	idx, err := OpenIndex(bytes.NewReader(indexBytes), int64(len(indexBytes)))
	require.NoError(b, err)
	require.NoError(b, idx.BuildLookupMap())

	normalized := make([]string, len(data.paths))
	for i, p := range data.paths {
		clean, err := NormalizePath(p)
		require.NoError(b, err)
		normalized[i] = clean
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := normalized[i%len(normalized)]
		_, ok, err := idx.LookupIndexNormalized(path)
		require.NoError(b, err)
		if !ok {
			b.Fatalf("missing path: %s", path)
		}
	}
}

func benchmarkEstargzOpenAndLookup(b *testing.B, cfg datasetConfig) {
	b.Helper()

	data := buildDataset(cfg)
	blob := buildEstargz(b, data)
	target := data.paths[len(data.paths)/2]

	var reads int64
	var bytesRead int64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr := &countingReaderAt{r: bytes.NewReader(blob)}
		sr := io.NewSectionReader(cr, 0, int64(len(blob)))
		reader, err := estargz.Open(sr)
		require.NoError(b, err)
		_, ok := reader.Lookup(target)
		if !ok {
			b.Fatalf("missing path: %s", target)
		}
		reads = cr.reads
		bytesRead = cr.bytes
	}

	b.ReportMetric(float64(reads), "reads")
	b.ReportMetric(float64(bytesRead), "bytes")
}

func benchmarkIndexListAndOpen(b *testing.B, layout Layout, cfg datasetConfig, listCount, openCount int) {
	b.Helper()

	data := buildDataset(cfg)
	indexBytes, dataBytes := buildArchive(b, layout, data)
	idx, err := OpenIndex(bytes.NewReader(indexBytes), int64(len(indexBytes)))
	require.NoError(b, err)
	samples := samplePaths(data, openCount, cfg.seed+1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		total := 0
		entries := idx.Entries()
		limit := listCount
		if limit > len(entries) {
			limit = len(entries)
		}
		for _, entry := range entries[:limit] {
			total += len(entry.Path)
		}
		for _, p := range samples {
			entry, ok, err := idx.Lookup(p)
			require.NoError(b, err)
			if !ok {
				b.Fatalf("missing path: %s", p)
			}
			start := entry.DataOffset
			end := start + entry.CompSize
			if end > uint64(len(dataBytes)) {
				b.Fatalf("data bounds invalid for %s", p)
			}
			total += int(entry.RawSize)
			_ = dataBytes[start:end]
		}
		if total == 0 {
			b.Fatal("unused result")
		}
	}
}

func benchmarkEstargzListAndOpen(b *testing.B, cfg datasetConfig, listCount, openCount int) {
	b.Helper()

	data := buildDataset(cfg)
	blob := buildEstargz(b, data)
	sr := io.NewSectionReader(bytes.NewReader(blob), 0, int64(len(blob)))
	reader, err := estargz.Open(sr)
	require.NoError(b, err)
	samples := samplePaths(data, openCount, cfg.seed+1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		total := 0
		limit := listCount
		if limit > len(data.paths) {
			limit = len(data.paths)
		}
		for _, p := range data.paths[:limit] {
			_, ok := reader.Lookup(p)
			if !ok {
				b.Fatalf("missing path: %s", p)
			}
			total++
		}
		for _, p := range samples {
			r, err := reader.OpenFile(p)
			require.NoError(b, err)
			n, err := io.Copy(io.Discard, r)
			require.NoError(b, err)
			total += int(n)
		}
		if total == 0 {
			b.Fatal("unused result")
		}
	}
}
