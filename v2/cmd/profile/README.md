# Blobber Profiling Harness

This CLI generates realistic datasets and runs push/pull/stream loops against
OCI registries (e.g. GHCR) while exposing pprof and fgprof endpoints.

## Prerequisites

- Go 1.25.4
- For GHCR: `docker login ghcr.io` (credentials are read from Docker config)

## Usage

Large file push:

```bash
/Users/josh/.goenv/versions/1.25.4/bin/go run ./cmd/profile \
  --op push \
  --ref ghcr.io/meigma/blobber/profiling:profiling \
  --file-mode large --file-size 2GB \
  --pprof-addr :6060 --fgprof
```

Thousands of small files (pull loop):

```bash
/Users/josh/.goenv/versions/1.25.4/bin/go run ./cmd/profile \
  --op pull \
  --ref ghcr.io/meigma/blobber/profiling:profiling \
  --file-mode many --file-count 10000 --file-size 4KB \
  --skip-push \
  --pprof-addr :6060 --fgprof
```

Stream a subset of files:

```bash
/Users/josh/.goenv/versions/1.25.4/bin/go run ./cmd/profile \
  --op stream \
  --ref ghcr.io/meigma/blobber/profiling:profiling \
  --file-mode many --file-count 10000 --file-size 4KB \
  --stream-read-count 100 --stream-read-size 64KB \
  --skip-push \
  --pprof-addr :6060 --fgprof
```

## Profiling Endpoints

- pprof: `http://localhost:6060/debug/pprof/`
- fgprof: `http://localhost:6060/debug/fgprof`

Example captures:

```bash
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
go tool pprof http://localhost:6060/debug/pprof/heap
```

## Key Flags

- `--op`: `push`, `pull`, or `stream`
- `--ref`: full OCI reference
- `--file-mode`: `large`, `many`, or `mixed`
- `--file-count`, `--file-size`, `--large-file-size`
- `--stream-read-count`, `--stream-read-size`, `--stream-paths`
- `--cache-dir`, `--ref-cache`, `--file-cache`, `--manifest-cache`
- `--pprof-addr`, `--fgprof`, `--block-rate`, `--mutex-rate`
