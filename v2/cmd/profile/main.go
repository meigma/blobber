package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/felixge/fgprof"
	"oras.land/oras-go/v2/registry/remote/credentials"

	"github.com/meigma/blobber/v2"
)

const (
	defaultFileSize       = int64(4 << 10)
	defaultLargeFileSize  = int64(128 << 20)
	defaultStreamReadSize = int64(0)
)

type config struct {
	op              string
	ref             string
	fileMode        string
	fileCount       int
	fileSize        sizeFlag
	largeFileSize   sizeFlag
	dataDir         string
	keepData        bool
	skipPush        bool
	iterations      int
	streamPaths     string
	streamReadCount int
	streamReadSize  sizeFlag
	cacheDir        string
	refCache        bool
	fileCache       bool
	manifestCache   bool
	refCacheTTL     time.Duration
	plainHTTP       bool
	pprofAddr       string
	enableFGProf    bool
	blockRate       int
	mutexRate       int
	useDockerCreds  bool
	requireCreds    bool
	timeout         time.Duration
}

type sizeFlag struct {
	bytes int64
	set   bool
}

func (s *sizeFlag) String() string {
	return strconv.FormatInt(s.bytes, 10)
}

func (s *sizeFlag) Set(value string) error {
	b, err := parseSize(value)
	if err != nil {
		return err
	}
	s.bytes = b
	s.set = true
	return nil
}

func main() {
	cfg := parseFlags()
	if err := validateConfig(cfg); err != nil {
		log.Fatal(err)
	}

	if cfg.pprofAddr != "" {
		startProfiler(cfg.pprofAddr, cfg.enableFGProf)
	}
	if cfg.blockRate > 0 {
		runtimeSetBlockProfileRate(cfg.blockRate)
	}
	if cfg.mutexRate > 0 {
		runtimeSetMutexProfileFraction(cfg.mutexRate)
	}

	ctx := context.Background()
	if cfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.timeout)
		defer cancel()
	}

	client, err := newClient(cfg)
	if err != nil {
		log.Fatal(err)
	}

	var data *dataSet
	if cfg.op == "push" || !cfg.skipPush {
		data, err = buildDataSet(cfg)
		if err != nil {
			log.Fatal(err)
		}
		if !cfg.keepData {
			defer data.cleanup()
		}
	}

	start := time.Now()

	switch cfg.op {
	case "push":
		err = runPush(ctx, client, cfg, data)
	case "pull":
		if data != nil && !cfg.skipPush {
			if pushErr := pushOnce(ctx, client, cfg.ref, data); pushErr != nil {
				log.Fatal(pushErr)
			}
		}
		err = runPull(ctx, client, cfg)
	case "stream":
		if data != nil && !cfg.skipPush {
			if pushErr := pushOnce(ctx, client, cfg.ref, data); pushErr != nil {
				log.Fatal(pushErr)
			}
		}
		err = runStream(ctx, client, cfg, data)
	default:
		err = fmt.Errorf("unsupported op: %s", cfg.op)
	}
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("op=%s iterations=%d duration=%s", cfg.op, cfg.iterations, time.Since(start))
}

func parseFlags() config {
	cfg := config{
		fileMode:        "many",
		fileCount:       1000,
		iterations:      1,
		refCacheTTL:     time.Hour,
		fileSize:        sizeFlag{bytes: defaultFileSize},
		largeFileSize:   sizeFlag{bytes: defaultLargeFileSize},
		streamReadSize:  sizeFlag{bytes: defaultStreamReadSize},
		streamReadCount: 1,
		useDockerCreds:  true,
	}

	flag.StringVar(&cfg.op, "op", "", "operation: push, pull, stream")
	flag.StringVar(&cfg.ref, "ref", "", "OCI reference (e.g. ghcr.io/meigma/blobber/profiling:profiling)")
	flag.StringVar(&cfg.fileMode, "file-mode", cfg.fileMode, "data mode: large, many, mixed")
	flag.IntVar(&cfg.fileCount, "file-count", cfg.fileCount, "number of small files")
	flag.Var(&cfg.fileSize, "file-size", "size of small files (e.g. 4KB, 64MB)")
	flag.Var(&cfg.largeFileSize, "large-file-size", "size of large file (e.g. 1GB)")
	flag.StringVar(&cfg.dataDir, "data-dir", "", "directory for generated data (empty uses temp)")
	flag.BoolVar(&cfg.keepData, "keep-data", false, "keep generated data on disk")
	flag.BoolVar(&cfg.skipPush, "skip-push", false, "skip push step for pull/stream")
	flag.IntVar(&cfg.iterations, "iterations", cfg.iterations, "number of iterations")
	flag.StringVar(&cfg.streamPaths, "stream-paths", "", "comma-separated paths to read during stream")
	flag.IntVar(&cfg.streamReadCount, "stream-read-count", cfg.streamReadCount, "number of files to read during stream (0 = all)")
	flag.Var(&cfg.streamReadSize, "stream-read-size", "bytes to read per file during stream (0 = full)")
	flag.StringVar(&cfg.cacheDir, "cache-dir", "", "cache directory")
	flag.BoolVar(&cfg.refCache, "ref-cache", false, "enable ref cache")
	flag.BoolVar(&cfg.fileCache, "file-cache", false, "enable file cache")
	flag.BoolVar(&cfg.manifestCache, "manifest-cache", false, "enable manifest cache (requires file cache)")
	flag.DurationVar(&cfg.refCacheTTL, "ref-cache-ttl", cfg.refCacheTTL, "ref cache TTL")
	flag.BoolVar(&cfg.plainHTTP, "plain-http", false, "use plain HTTP (local registries)")
	flag.StringVar(&cfg.pprofAddr, "pprof-addr", "", "pprof bind address (e.g. :6060)")
	flag.BoolVar(&cfg.enableFGProf, "fgprof", false, "enable fgprof handler at /debug/fgprof")
	flag.IntVar(&cfg.blockRate, "block-rate", 0, "block profile rate (runtime.SetBlockProfileRate)")
	flag.IntVar(&cfg.mutexRate, "mutex-rate", 0, "mutex profile rate (runtime.SetMutexProfileFraction)")
	flag.BoolVar(&cfg.useDockerCreds, "use-docker-creds", cfg.useDockerCreds, "use docker config credentials")
	flag.BoolVar(&cfg.requireCreds, "require-creds", false, "fail if docker credentials are unavailable")
	flag.DurationVar(&cfg.timeout, "timeout", 0, "timeout for the operation")

	flag.Parse()
	return cfg
}

func validateConfig(cfg config) error {
	if cfg.op == "" {
		return errors.New("op is required")
	}
	if cfg.ref == "" {
		return errors.New("ref is required")
	}
	switch cfg.op {
	case "push", "pull", "stream":
	default:
		return fmt.Errorf("invalid op: %s", cfg.op)
	}
	if cfg.iterations < 1 {
		return errors.New("iterations must be >= 1")
	}
	if cfg.op == "push" && !hasTag(cfg.ref) {
		return errors.New("push requires a tag reference (e.g. repo:tag)")
	}
	if cfg.fileMode != "large" && cfg.fileMode != "many" && cfg.fileMode != "mixed" {
		return fmt.Errorf("invalid file-mode: %s", cfg.fileMode)
	}
	if cfg.fileCount < 0 {
		return errors.New("file-count must be >= 0")
	}
	if cfg.fileSize.bytes < 0 || cfg.largeFileSize.bytes < 0 || cfg.streamReadSize.bytes < 0 {
		return errors.New("sizes must be >= 0")
	}
	if cfg.cacheDir == "" {
		if cfg.refCache || cfg.fileCache || cfg.manifestCache {
			return errors.New("cache-dir is required when enabling caches")
		}
	}
	if cfg.manifestCache && !cfg.fileCache {
		return errors.New("manifest-cache requires file-cache")
	}
	return nil
}

func newClient(cfg config) (*blobber.Client, error) {
	opts := []blobber.ClientOption{
		blobber.WithUserAgent("blobber-profile"),
	}
	if cfg.plainHTTP {
		opts = append(opts, blobber.WithPlainHTTP(true))
	}
	if cfg.cacheDir != "" {
		if cfg.refCache {
			opts = append(opts, blobber.WithRefCache(cfg.cacheDir, cfg.refCacheTTL))
		}
		if cfg.fileCache {
			opts = append(opts, blobber.WithFileCache(cfg.cacheDir))
		}
		if cfg.manifestCache {
			opts = append(opts, blobber.WithManifestCache())
		}
	}
	if cfg.useDockerCreds {
		store, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
		if err != nil {
			if cfg.requireCreds {
				return nil, fmt.Errorf("load docker credentials: %w", err)
			}
			log.Printf("warning: failed to load docker credentials: %v", err)
		} else {
			opts = append(opts, blobber.WithCredentialStore(store))
		}
	}
	return blobber.NewClient(opts...), nil
}

func runPush(ctx context.Context, client *blobber.Client, cfg config, data *dataSet) error {
	if data == nil {
		return errors.New("push requires generated data")
	}

	for i := 0; i < cfg.iterations; i++ {
		ref := refForIteration(cfg.ref, i, cfg.iterations)
		if _, err := client.Push(ctx, ref, os.DirFS(data.root)); err != nil {
			return fmt.Errorf("push: %w", err)
		}
	}
	return nil
}

func pushOnce(ctx context.Context, client *blobber.Client, ref string, data *dataSet) error {
	if data == nil {
		return errors.New("push requires generated data")
	}
	_, err := client.Push(ctx, ref, os.DirFS(data.root))
	return err
}

func runPull(ctx context.Context, client *blobber.Client, cfg config) error {
	for i := 0; i < cfg.iterations; i++ {
		handle, err := client.Pull(ctx, cfg.ref)
		if err != nil {
			return fmt.Errorf("pull: %w", err)
		}
		if err := handle.Close(); err != nil {
			return fmt.Errorf("close pull handle: %w", err)
		}
	}
	return nil
}

func runStream(ctx context.Context, client *blobber.Client, cfg config, data *dataSet) error {
	paths := streamPaths(cfg, data)
	if len(paths) == 0 {
		return errors.New("no stream paths available")
	}

	for i := 0; i < cfg.iterations; i++ {
		handle, err := client.Stream(ctx, cfg.ref)
		if err != nil {
			return fmt.Errorf("stream: %w", err)
		}

		for _, path := range paths {
			if err := readStreamFile(handle, path, cfg.streamReadSize.bytes); err != nil {
				handle.Close()
				return err
			}
		}

		if err := handle.Close(); err != nil {
			return fmt.Errorf("close stream handle: %w", err)
		}
	}
	return nil
}

func readStreamFile(handle *blobber.BlobHandle, path string, limit int64) error {
	f, err := handle.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	if limit > 0 {
		if _, err := io.CopyN(io.Discard, f, limit); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read %s: %w", path, err)
		}
		return nil
	}

	if _, err := io.Copy(io.Discard, f); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

func streamPaths(cfg config, data *dataSet) []string {
	if cfg.streamPaths != "" {
		parts := strings.Split(cfg.streamPaths, ",")
		return trimAndLimit(parts, cfg.streamReadCount)
	}
	if data != nil && len(data.paths) > 0 {
		return trimAndLimit(data.paths, cfg.streamReadCount)
	}
	return []string{"hello.txt"}
}

func trimAndLimit(paths []string, limit int) []string {
	trimmed := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			trimmed = append(trimmed, path)
		}
	}
	if limit <= 0 || len(trimmed) <= limit {
		return trimmed
	}
	return trimmed[:limit]
}

type dataSet struct {
	root    string
	paths   []string
	cleanup func()
}

func buildDataSet(cfg config) (*dataSet, error) {
	root := cfg.dataDir
	cleanup := func() {}
	if root == "" {
		tempDir, err := os.MkdirTemp("", "blobber-profile-data-*")
		if err != nil {
			return nil, err
		}
		root = tempDir
		cleanup = func() {
			_ = os.RemoveAll(root)
		}
	}

	switch cfg.fileMode {
	case "large":
		if cfg.fileSize.bytes == 0 {
			return nil, errors.New("file-size must be > 0 for large mode")
		}
		paths := []string{"large.bin"}
		if err := writeFile(root, paths[0], cfg.fileSize.bytes); err != nil {
			return nil, err
		}
		return &dataSet{root: root, paths: paths, cleanup: cleanup}, nil
	case "many":
		if cfg.fileCount == 0 {
			return nil, errors.New("file-count must be > 0 for many mode")
		}
		paths, err := writeManyFiles(root, cfg.fileCount, cfg.fileSize.bytes)
		if err != nil {
			return nil, err
		}
		return &dataSet{root: root, paths: paths, cleanup: cleanup}, nil
	case "mixed":
		if cfg.fileCount == 0 {
			return nil, errors.New("file-count must be > 0 for mixed mode")
		}
		if cfg.fileSize.bytes == 0 || cfg.largeFileSize.bytes == 0 {
			return nil, errors.New("file-size and large-file-size must be > 0 for mixed mode")
		}
		paths := []string{"large.bin"}
		if err := writeFile(root, paths[0], cfg.largeFileSize.bytes); err != nil {
			return nil, err
		}
		smallPaths, err := writeManyFiles(root, cfg.fileCount, cfg.fileSize.bytes)
		if err != nil {
			return nil, err
		}
		paths = append(paths, smallPaths...)
		return &dataSet{root: root, paths: paths, cleanup: cleanup}, nil
	default:
		return nil, fmt.Errorf("unknown file-mode: %s", cfg.fileMode)
	}
}

func writeManyFiles(root string, count int, size int64) ([]string, error) {
	if size == 0 {
		return nil, errors.New("file-size must be > 0")
	}
	paths := make([]string, 0, count)
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("files/file-%06d.bin", i+1)
		if err := writeFile(root, name, size); err != nil {
			return nil, err
		}
		paths = append(paths, name)
	}
	return paths, nil
}

func writeFile(root, relPath string, size int64) error {
	dst := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}

	bufSize := int64(4 << 20)
	if size < bufSize {
		bufSize = size
	}
	buf := make([]byte, bufSize)
	for i := range buf {
		buf[i] = byte(i)
	}

	remaining := size
	for remaining > 0 {
		n := int64(len(buf))
		if remaining < n {
			n = remaining
		}
		if _, err := f.Write(buf[:n]); err != nil {
			f.Close()
			return err
		}
		remaining -= n
	}

	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

func startProfiler(addr string, enableFGProf bool) {
	if enableFGProf {
		http.Handle("/debug/fgprof", fgprof.Handler())
	}

	go func() {
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatalf("pprof server error: %v", err)
		}
	}()
	log.Printf("pprof server listening on %s", addr)
}

func parseSize(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("size is empty")
	}
	value = strings.ToUpper(value)

	multiplier := int64(1)
	for _, suffix := range []struct {
		suffix string
		mult   int64
	}{
		{"KB", 1 << 10},
		{"K", 1 << 10},
		{"MB", 1 << 20},
		{"M", 1 << 20},
		{"GB", 1 << 30},
		{"G", 1 << 30},
		{"TB", 1 << 40},
		{"T", 1 << 40},
		{"B", 1},
	} {
		if strings.HasSuffix(value, suffix.suffix) {
			multiplier = suffix.mult
			value = strings.TrimSuffix(value, suffix.suffix)
			break
		}
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("size missing numeric value")
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, errors.New("size must be >= 0")
	}

	const maxInt64 = int64(^uint64(0) >> 1)
	if n > maxInt64/multiplier {
		return 0, errors.New("size overflows int64")
	}

	return n * multiplier, nil
}

func hasTag(ref string) bool {
	at := strings.LastIndex(ref, "@")
	if at != -1 {
		return false
	}
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	return lastColon != -1 && lastColon > lastSlash
}

func refForIteration(ref string, i, total int) string {
	if total <= 1 {
		return ref
	}
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon == -1 || lastColon < lastSlash {
		return ref
	}
	tag := ref[lastColon+1:]
	base := ref[:lastColon+1]
	return fmt.Sprintf("%s%s-%d", base, tag, i+1)
}

var runtimeSetBlockProfileRate = func(rate int) {
	runtime.SetBlockProfileRate(rate)
}

var runtimeSetMutexProfileFraction = func(rate int) {
	runtime.SetMutexProfileFraction(rate)
}
