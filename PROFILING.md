# Profiling

This repo ships a profiling harness under `cmd/profile` that exercises push/pull
paths and emits CPU, fgprof (wall clock), and trace profiles. The harness is
guarded by the `profiling` build tag.

## Prerequisites

- Go toolchain installed (use the same `go` binary throughout your run).
- Docker credentials available (e.g., `docker login`).
  - The client reads Docker's credential chain from `~/.docker/config.json`
    (or `$DOCKER_CONFIG/config.json`).
  - DockerHub creds stored under `https://index.docker.io/v1/` are supported.

## Quick Start

Run a pull with CPU profiling and view the web UI:

```bash
go run -tags=profiling ./cmd/profile \
  -mode pull \
  -profile cpu \
  -ref docker.io/jmgilman/profiling:latest

go tool pprof -http=:0 profiles/cpu_pull_*.pprof
```

Run a push (no profiling) and just measure the timing:

```bash
go run -tags=profiling ./cmd/profile \
  -mode push \
  -profile none \
  -ref docker.io/jmgilman/profiling:latest \
  -payload tmp/profiledata
```

## Output Files

Profiles are written to `profiles/` by default. Each run creates:

- `cpu_*.pprof` (CPU profile, if `-profile cpu`)
- `fgprof_*.pprof` (wall-clock profile, if `-profile fgprof`)
- `trace_*.out` (execution trace, if `-profile trace`)
- `heap_*.pprof` and `allocs_*.pprof` (always written after the run)

File names include the mode, label (if set), and a timestamp.

## Common Workflows

### Pull with caching (cold and warm)

```bash
# Cold cache
go run -tags=profiling ./cmd/profile \
  -mode pull \
  -profile fgprof \
  -ref docker.io/jmgilman/profiling:latest \
  -cache-dir tmp/profilecache \
  -clear-cache \
  -unique-dest \
  -label cold

# Warm cache
go run -tags=profiling ./cmd/profile \
  -mode pull \
  -profile fgprof \
  -ref docker.io/jmgilman/profiling:latest \
  -cache-dir tmp/profilecache \
  -unique-dest \
  -label warm
```

### Repeat to stabilize samples

```bash
go run -tags=profiling ./cmd/profile \
  -mode pull \
  -profile cpu \
  -ref docker.io/jmgilman/profiling:latest \
  -repeat 20 \
  -unique-dest \
  -label repeat20
```

### Descriptor caching (reduces registry lookups between iterations)

```bash
go run -tags=profiling ./cmd/profile \
  -mode pull \
  -profile fgprof \
  -ref docker.io/jmgilman/profiling:latest \
  -descriptor-cache \
  -repeat 10 \
  -unique-dest \
  -label desc-cache
```

### Local registry (plain HTTP)

```bash
go run -tags=profiling ./cmd/profile \
  -mode pull \
  -profile cpu \
  -ref localhost:5001/your/repo:tag \
  -insecure
```

## Visualizing Profiles

### CPU or fgprof

Interactive terminal:

```bash
go tool pprof profiles/cpu_pull_*.pprof
```

Web UI:

```bash
go tool pprof -http=:0 profiles/cpu_pull_*.pprof
go tool pprof -http=:0 profiles/fgprof_pull_*.pprof
```

Compare two profiles:

```bash
go tool pprof -diff_base=profiles/cpu_before.pprof profiles/cpu_after.pprof
```

### Trace

```bash
go tool trace profiles/trace_pull_*.out
```

## Flag Reference

- `-ref` Fully qualified image reference (overrides `-registry/-repo/-tag`).
- `-registry/-repo/-tag` Components to build a reference when `-ref` is unset.
- `-mode` `push`, `pull`, or `both`.
- `-profile` `cpu`, `fgprof`, `trace`, or `none`.
- `-out` Output directory for profiles (default: `profiles`).
- `-label` Label suffix for output filenames.
- `-repeat` Number of iterations.
- `-unique-dest` Create a unique pull dir per iteration (avoids overlap).
- `-payload` Directory to push (push mode).
- `-pull-dir` Destination root for pulls.
- `-cache-dir` Enable the blob cache at the given path.
- `-clear-cache` Remove cache dir before running.
- `-lazy` Enable lazy loading (cache-only).
- `-prefetch` Enable background prefetch (cache-only).
- `-descriptor-cache` Cache resolved layer descriptors in-memory.
- `-insecure` Use HTTP (local registries).
- `-log-level` `debug`, `info`, `warn`, `error`.
- `-cache-stats` Print cache file stats after the run.
- `-timeout` Overall timeout for the run.

## Troubleshooting

- **Auth failures:** ensure `docker login` has been run and the Docker config is
  readable from `$HOME/.docker/config.json` or `$DOCKER_CONFIG/config.json`.
- **Go toolchain mismatch:** make sure the `go` binary and `GOROOT` are aligned
  (common when using goenv or multiple Go installs).
