//go:build profiling
// +build profiling

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"time"

	"github.com/felixge/fgprof"
	"github.com/grafana/pyroscope-go"

	"github.com/meigma/blobber"
)

type profileKind string

const (
	profileCPU     profileKind = "cpu"
	profileFG      profileKind = "fgprof"
	profileTrace   profileKind = "trace"
	profileNone    profileKind = "none"
	defaultRepo                = "cli-test/profile"
	defaultPayload             = "tmp/profiledata"
	defaultPullDir             = "tmp/profilepull"
)

const (
	modePush = "push"
	modePull = "pull"
	modeBoth = "both"
)

func main() {
	var (
		ref        = flag.String("ref", "", "fully qualified image reference (overrides registry/repo/tag)")
		registry   = flag.String("registry", "localhost:5001", "registry host:port")
		repo       = flag.String("repo", defaultRepo, "repository name (no registry)")
		tag        = flag.String("tag", "", "tag to use (default: timestamp)")
		payload    = flag.String("payload", defaultPayload, "payload directory to push")
		pullDir    = flag.String("pull-dir", defaultPullDir, "destination directory for pull")
		mode       = flag.String("mode", "push", "mode: push, pull, or both")
		profile    = flag.String("profile", "cpu", "profile type: cpu, fgprof, trace, none")
		outDir     = flag.String("out", "profiles", "output directory for profiles")
		label      = flag.String("label", "", "label suffix for profile files")
		stamp      = flag.Bool("stamp", true, "write a run stamp file before push")
		repeat     = flag.Int("repeat", 1, "number of iterations")
		unique     = flag.Bool("unique-dest", false, "use a unique destination per pull iteration")
		cacheDir   = flag.String("cache-dir", "", "cache directory (enables caching when set)")
		clearCache = flag.Bool("clear-cache", false, "clear cache directory before running")
		lazy       = flag.Bool("lazy", false, "enable lazy loading (cache only)")
		prefetch   = flag.Bool("prefetch", false, "enable background prefetch (cache only)")
		insecure   = flag.Bool("insecure", false, "use plain HTTP (for local registries)")
		logLevel   = flag.String("log-level", "", "log level: debug, info, warn, error")
		descCache  = flag.Bool("descriptor-cache", false, "cache layer descriptors in-memory")
		stats      = flag.Bool("cache-stats", false, "print cache file stats after run")
		timeout    = flag.Duration("timeout", 15*time.Minute, "overall timeout")
		pyroAddr   = flag.String("pyroscope", "", "Pyroscope server URL (enables streaming, disables local profiles)")
	)
	flag.Parse()

	runID := time.Now().UTC().Format("20060102T150405Z")
	if *tag == "" {
		*tag = runID
	}
	refValue := *ref
	if refValue == "" {
		if *registry == "" {
			log.Fatalf("registry is required when ref is not set")
		}
		refValue = fmt.Sprintf("%s/%s:%s", *registry, strings.TrimPrefix(*repo, "/"), *tag)
	}

	modeValue := strings.ToLower(*mode)
	if modeValue != modePush && modeValue != modePull && modeValue != modeBoth {
		log.Fatalf("invalid mode %q (expected %s, %s, or %s)", *mode, modePush, modePull, modeBoth)
	}

	profileKindValue := profileKind(strings.ToLower(*profile))
	if !isValidProfile(profileKindValue) {
		log.Fatalf("invalid profile %q (expected cpu, fgprof, trace, none)", *profile)
	}

	// When Pyroscope is enabled, stream profiles instead of writing locally
	var pyroProfiler *pyroscope.Profiler
	if *pyroAddr != "" {
		profiler, err := pyroscope.Start(pyroscope.Config{
			ApplicationName: "blobber-profile",
			ServerAddress:   *pyroAddr,
			// Grafana Cloud requires BasicAuth (AuthToken is deprecated)
			// User: instance ID from Grafana Cloud, Password: API token
			BasicAuthUser:     os.Getenv("PYROSCOPE_BASIC_AUTH_USER"),
			BasicAuthPassword: os.Getenv("PYROSCOPE_BASIC_AUTH_PASSWORD"),
			// Use a short upload rate since profiling runs are brief (~10s)
			UploadRate: 5 * time.Second,
			Logger:     pyroscope.StandardLogger,
			Tags: map[string]string{
				"mode":    modeValue,
				"git_sha": os.Getenv("GITHUB_SHA"),
				"git_ref": os.Getenv("GITHUB_REF_NAME"),
				"run_id":  runID,
			},
			ProfileTypes: []pyroscope.ProfileType{
				pyroscope.ProfileCPU,
				pyroscope.ProfileAllocObjects,
				pyroscope.ProfileAllocSpace,
				pyroscope.ProfileInuseObjects,
				pyroscope.ProfileInuseSpace,
			},
		})
		if err != nil {
			log.Fatalf("start pyroscope: %v", err)
		}
		pyroProfiler = profiler
		log.Printf("streaming profiles to %s", *pyroAddr)
	}

	if *pyroAddr == "" {
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			log.Fatalf("create profile output dir: %v", err)
		}
	}

	if modeValue != modePull {
		if _, err := os.Stat(*payload); err != nil {
			log.Fatalf("payload path %q: %v", *payload, err)
		}
	}
	if *repeat < 1 {
		log.Fatalf("repeat must be >= 1")
	}

	labelParts := []string{modeValue}
	if *label != "" {
		labelParts = append(labelParts, sanitizeLabel(*label))
	}
	labelParts = append(labelParts, runID)
	labelValue := strings.Join(labelParts, "_")

	// Only start local profiling when not streaming to Pyroscope
	var stopProfile func() error
	if *pyroAddr == "" {
		var err error
		stopProfile, err = startProfile(profileKindValue, *outDir, labelValue)
		if err != nil {
			log.Fatalf("start profile: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var clientOpts []blobber.ClientOption
	if *insecure {
		clientOpts = append(clientOpts, blobber.WithInsecure(true))
	}
	if *logLevel != "" {
		level, err := parseLogLevel(*logLevel)
		if err != nil {
			log.Fatalf("parse log level: %v", err)
		}
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
		clientOpts = append(clientOpts, blobber.WithLogger(logger))
	}
	if *descCache {
		clientOpts = append(clientOpts, blobber.WithDescriptorCache(true))
	}
	if *cacheDir != "" {
		absCacheDir, err := filepath.Abs(*cacheDir)
		if err != nil {
			log.Fatalf("resolve cache dir: %v", err)
		}
		if *clearCache {
			if err := os.RemoveAll(absCacheDir); err != nil {
				log.Fatalf("clear cache dir: %v", err)
			}
		}
		clientOpts = append(clientOpts, blobber.WithCacheDir(absCacheDir))
		if *lazy {
			clientOpts = append(clientOpts, blobber.WithLazyLoading(true))
		}
		if *prefetch {
			clientOpts = append(clientOpts, blobber.WithBackgroundPrefetch(true))
		}
	} else if *clearCache || *lazy || *prefetch {
		log.Fatalf("cache options require --cache-dir")
	}

	client, err := blobber.NewClient(clientOpts...)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	pullRoot := *pullDir
	if *unique {
		pullRoot = filepath.Join(*pullDir, runID)
		if err := recreateDir(pullRoot); err != nil {
			log.Fatalf("create pull root: %v", err)
		}
	}

	for i := range *repeat {
		if *repeat > 1 {
			log.Printf("iteration %d/%d", i+1, *repeat)
		}
		if modeValue == modePush || modeValue == modeBoth {
			if *stamp {
				if err := stampPayload(*payload); err != nil {
					log.Fatalf("stamp payload: %v", err)
				}
			}
			start := time.Now()
			if _, err := client.Push(ctx, refValue, os.DirFS(*payload)); err != nil {
				log.Fatalf("push: %v", err)
			}
			log.Printf("push complete: %s", time.Since(start))
		}

		if modeValue == modePull || modeValue == modeBoth {
			destDir := pullRoot
			if *unique {
				destDir = filepath.Join(pullRoot, fmt.Sprintf("iter-%03d", i+1))
				if err := os.MkdirAll(destDir, 0o755); err != nil {
					log.Fatalf("create pull dir: %v", err)
				}
			} else if err := recreateDir(destDir); err != nil {
				log.Fatalf("create pull dir: %v", err)
			}
			start := time.Now()
			if err := client.Pull(ctx, refValue, destDir); err != nil {
				log.Fatalf("pull: %v", err)
			}
			log.Printf("pull complete: %s", time.Since(start))
		}
	}

	// Stop profiling - either Pyroscope or local
	if pyroProfiler != nil {
		if err := pyroProfiler.Stop(); err != nil {
			log.Fatalf("stop pyroscope: %v", err)
		}
		log.Printf("pyroscope profiling stopped")
	} else {
		if stopErr := stopProfile(); stopErr != nil {
			log.Fatalf("stop profile: %v", stopErr)
		}
		if err := writeHeapProfile(*outDir, labelValue); err != nil {
			log.Fatalf("write heap profile: %v", err)
		}
		if err := writeAllocsProfile(*outDir, labelValue); err != nil {
			log.Fatalf("write allocs profile: %v", err)
		}
	}
	if *stats && *cacheDir != "" {
		if err := printCacheStats(*cacheDir); err != nil {
			log.Fatalf("print cache stats: %v", err)
		}
	}
}

func isValidProfile(kind profileKind) bool {
	switch kind {
	case profileCPU, profileFG, profileTrace, profileNone:
		return true
	default:
		return false
	}
}

func startProfile(kind profileKind, outDir, label string) (func() error, error) {
	switch kind {
	case profileCPU:
		path := filepath.Join(outDir, "cpu_"+label+".pprof")
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			_ = f.Close()
			return nil, err
		}
		return func() error {
			pprof.StopCPUProfile()
			return f.Close()
		}, nil
	case profileFG:
		path := filepath.Join(outDir, "fgprof_"+label+".pprof")
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		stop := fgprof.Start(f, fgprof.FormatPprof)
		return func() error {
			stopErr := stop()
			closeErr := f.Close()
			return errors.Join(stopErr, closeErr)
		}, nil
	case profileTrace:
		path := filepath.Join(outDir, "trace_"+label+".out")
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		if err := trace.Start(f); err != nil {
			_ = f.Close()
			return nil, err
		}
		return func() error {
			trace.Stop()
			return f.Close()
		}, nil
	case profileNone:
		return func() error { return nil }, nil
	default:
		return nil, fmt.Errorf("unknown profile type: %s", kind)
	}
}

func writeHeapProfile(outDir, label string) error {
	path := filepath.Join(outDir, "heap_"+label+".pprof")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	runtime.GC()
	return pprof.WriteHeapProfile(f)
}

func writeAllocsProfile(outDir, label string) error {
	path := filepath.Join(outDir, "allocs_"+label+".pprof")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return pprof.Lookup("allocs").WriteTo(f, 0)
}

func stampPayload(payloadDir string) error {
	content := fmt.Sprintf("run=%s\nrand=%d\n", time.Now().UTC().Format(time.RFC3339Nano), rand.Int63())
	return os.WriteFile(filepath.Join(payloadDir, ".profile-run.txt"), []byte(content), 0o644)
}

func recreateDir(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return os.MkdirAll(path, 0o755)
}

func sanitizeLabel(value string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return '_'
		}
	}, value)
}

func parseLogLevel(value string) (slog.Leveler, error) {
	switch strings.ToLower(value) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return nil, fmt.Errorf("unknown level %q", value)
	}
}

func printCacheStats(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	var (
		blobCount  int
		entryCount int
		blobSize   int64
		entrySize  int64
	)
	for _, target := range []struct {
		path  string
		count *int
		size  *int64
	}{
		{path: filepath.Join(absDir, "blobs", "sha256"), count: &blobCount, size: &blobSize},
		{path: filepath.Join(absDir, "entries", "sha256"), count: &entryCount, size: &entrySize},
	} {
		walkErr := filepath.WalkDir(target.path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			*target.count++
			*target.size += info.Size()
			return nil
		})
		if walkErr != nil {
			return walkErr
		}
	}
	log.Printf("cache stats: dir=%s blobs=%d entries=%d blob_bytes=%d entry_bytes=%d",
		absDir, blobCount, entryCount, blobSize, entrySize)
	return nil
}
