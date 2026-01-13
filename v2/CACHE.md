# Blobber v2 Cache Design

This document describes the caching system for Blobber v2. It serves as the implementation specification.

## Design Philosophy

The v1 cache attempted partial blob caching (arbitrary byte ranges, download resumption) which resulted in significant complexity: range tracking, gap detection, merging, atomic persistence of partial state, and deferred verification. The implementation was difficult to reason about and maintain.

**v2 takes a simpler approach: cache complete files only.**

- No partial state tracking
- No range merging
- No download resumption
- Anything partial, failed, or corrupted is silently discarded

This provides the same user-facing benefit (avoid redundant network calls) with dramatically simpler internals.

## Two Cache Types

### RefCache

Caches ref→digest mappings. Refs are mutable (tags can move), so a TTL controls freshness.

**Purpose:** Avoid the manifest fetch network call when resolving a ref to a digest.

**Behavior:**
- On ref lookup: check cache, return digest if within TTL
- On cache miss or stale: fetch from network, store result with timestamp
- Without RefCache configured: always fetch manifest from network

### FileCache

Caches extracted files by digest. Digest-addressed content is immutable, so no TTL is needed.

**Purpose:** Serve file content from local disk instead of network.

**Behavior:**
- Files are stored under their blob's digest
- Cache check is simple: file exists at path? Return it.
- Pruning uses LRU based on last access time

## Public API

```go
// Client options
func WithRefCache(path string, ttl time.Duration) ClientOption
func WithFileCache(path string) ClientOption

// Pruning (called explicitly by consumer)
func PruneFileCache(ctx context.Context, path string, strategy PruneStrategy) error

type PruneStrategy struct {
    MaxSize int64         // Evict LRU until under this size (bytes)
    MaxAge  time.Duration // Evict entries not accessed within this duration
}
```

### Usage Combinations

| RefCache | FileCache | Behavior |
|----------|-----------|----------|
| No | No | No caching - always network |
| Yes | No | Cache ref→digest only - skip manifest fetch if fresh, but always stream files from network |
| No | Yes | Always fetch manifest, but serve/cache files locally |
| Yes | Yes | Full caching - skip manifest fetch if fresh, serve files from cache |

## Cache Structure on Disk

```
<cache_root>/
  refs/
    <hash>/                    # SHA256 hash of ref string (filesystem-safe)
      cache.json               # { "ref": "ghcr.io/org/repo:v1", "digest": "sha256:abc", "updatedAt": "2024-01-01T00:00:00Z" }

  blobs/
    <hash>/                    # SHA256 hash of digest string (filesystem-safe)
      cache.json               # { "digest": "sha256:abc", "size": 12345, "lastAccessed": "2024-01-01T00:00:00Z" }
      config.yaml              # Extracted file
      data/
        settings.json          # Extracted file (preserves directory structure)
```

**Notes:**
- Ref and digest strings are hashed for filesystem safety (avoids `/`, `:`, `@` issues)
- Original ref/digest stored inside cache.json for debugging/inspection
- Size recorded at write time to avoid recalculating during prune

## Package Structure

```
internal/cache/
  cache.go        # Package doc, shared types
  refcache.go     # RefCache implementation
  filecache.go    # FileCache implementation
  cachedfile.go   # Tee-ing fs.File wrapper for Stream()
  prune.go        # Pruning logic
  options.go      # PruneStrategy
```

## Interfaces

```go
// internal/cache/cache.go

type RefCache interface {
    // Lookup returns cached digest if fresh, ok=false if miss or stale
    Lookup(ref string) (digest digest.Digest, ok bool)

    // Store records a ref→digest mapping with current timestamp
    Store(ref string, d digest.Digest)
}

type FileCache interface {
    // Get returns cached file if exists, fs.ErrNotExist otherwise
    Get(digest digest.Digest, path string) (fs.File, error)

    // WrapFile wraps f to cache contents on full read + checksum match
    // Used by Stream() for on-demand file caching
    WrapFile(digest digest.Digest, path string, f fs.File, size int64, checksum string) fs.File

    // TouchAccess updates lastAccessed for LRU (called on cache hit)
    TouchAccess(digest digest.Digest) error

    // Dir returns the cache directory path for a digest
    // Creates the directory if it doesn't exist
    // Used by Pull() for direct extraction
    Dir(digest digest.Digest) (string, error)

    // IsComplete returns true if digest has been fully cached
    IsComplete(digest digest.Digest) bool

    // MarkComplete records that all files for digest have been written
    // Updates cache.json with size and lastAccessed
    MarkComplete(digest digest.Digest, size int64) error
}

func NewRefCache(path string, ttl time.Duration) (RefCache, error)
func NewFileCache(path string) (FileCache, error)
```

## Integration Points

### Client

The Client holds optional RefCache and FileCache instances, created from options.

**Ref Resolution:**
```go
func (c *Client) resolveRef(ctx context.Context, ref string) (digest.Digest, error) {
    // Check RefCache first
    if c.refCache != nil {
        if d, ok := c.refCache.Lookup(ref); ok {
            return d, nil
        }
    }

    // Cache miss or no cache: resolve via network
    d, err := c.registry.ResolveRef(ctx, ref)
    if err != nil {
        return "", err
    }

    // Store in cache
    if c.refCache != nil {
        c.refCache.Store(ref, d)
    }

    return d, nil
}
```

**Note:** This requires splitting the current `FetchBlob(ref)` into:
1. `ResolveRef(ref) -> digest` - resolve ref to digest
2. `FetchBlobByDigest(digest)` - fetch blob by digest (rejects refs)

### Pull()

```go
func (c *Client) Pull(ctx context.Context, ref string, opts ...PullOption) (*BlobHandle, error) {
    // ... policy verification ...

    digest, err := c.resolveRef(ctx, ref)
    if err != nil {
        return nil, err
    }

    if c.fileCache != nil {
        // Check if already cached
        if c.fileCache.IsComplete(digest) {
            c.fileCache.TouchAccess(digest)
            dir, _ := c.fileCache.Dir(digest)
            return newDirHandle(ctx, dir)
        }

        // Cache miss: download and extract to cache
        dir, _ := c.fileCache.Dir(digest)
        rc, err := c.registry.FetchBlobByDigest(ctx, ref, digest)
        if err != nil {
            return nil, err
        }
        defer rc.Close()

        size, err := extractToDir(ctx, rc, dir)
        if err != nil {
            return nil, err
        }

        c.fileCache.MarkComplete(digest, size)
        return newDirHandle(ctx, dir)
    }

    // No cache: existing temp file logic
    // ... download to temp, create estargz-backed handle ...
}
```

**newDirHandle:** A new constructor that creates a BlobHandle backed by a directory (using `os.DirFS` or similar) instead of an estargz archive. This requires the backing `Reader` interface to support both modes.

### Stream()

```go
func (c *Client) Stream(ctx context.Context, ref string, opts ...StreamOption) (*BlobHandle, error) {
    // ... policy verification ...

    digest, err := c.resolveRef(ctx, ref)
    if err != nil {
        return nil, err
    }

    // Open blob for range requests
    blobReader, err := c.registry.OpenBlobByDigest(ctx, ref, digest)
    if err != nil {
        return nil, err
    }

    // Create network-backed handle, optionally with file cache
    handle, err := newNetworkHandle(ctx, blobReader, fetchFull)
    if err != nil {
        return nil, err
    }

    // Inject file cache if configured
    if c.fileCache != nil {
        handle.cache = c.fileCache
        handle.digest = digest
    }

    return handle, nil
}
```

### BlobHandle.Open()

```go
func (h *BlobHandle) Open(name string) (fs.File, error) {
    if h.closed.Load() {
        return nil, fs.ErrClosed
    }

    // Check file cache first
    if h.cache != nil {
        if f, err := h.cache.Get(h.digest, name); err == nil {
            h.cache.TouchAccess(h.digest)
            return f, nil
        }
    }

    // Not cached: open from reader (network or local)
    f, err := h.reader.Open(name)
    if err != nil {
        return nil, err
    }

    // Wrap with caching tee if cache configured
    if h.cache != nil {
        entry := h.reader.Lookup(name) // Get TOC entry for size/checksum
        f = h.cache.WrapFile(h.digest, name, f, entry.Size, entry.Digest)
    }

    return f, nil
}
```

## Cached File Wrapper (for Stream)

The `WrapFile` method returns an `fs.File` that:

1. Tees reads to a temp file
2. Tracks bytes read
3. On EOF (bytes read == expected size):
   - Computes checksum of temp file
   - Compares against expected checksum from TOC
   - If match: atomic rename temp → cached file
   - If mismatch: log warning, delete temp (not a hard error)
4. On Close before EOF: delete temp file

**Concurrency:** If multiple goroutines Open() the same uncached file simultaneously, use a lock check. If another writer is detected, skip caching (don't block). First-to-complete wins.

```go
// Pseudocode for cached file wrapper
type cachedFile struct {
    inner      fs.File
    temp       *os.File
    digest     digest.Digest
    path       string
    expected   int64
    checksum   string
    read       int64
    cache      *fileCache
}

func (f *cachedFile) Read(p []byte) (int, error) {
    n, err := f.inner.Read(p)
    if n > 0 {
        f.temp.Write(p[:n])
        f.read += int64(n)
    }

    if err == io.EOF && f.read == f.expected {
        f.promoteToCache()
    }

    return n, err
}

func (f *cachedFile) promoteToCache() {
    f.temp.Sync()

    // Verify checksum
    actual := computeChecksum(f.temp)
    if actual != f.checksum {
        log.Warn("checksum mismatch, discarding cached file", ...)
        os.Remove(f.temp.Name())
        return
    }

    // Atomic rename
    destPath := f.cache.filePath(f.digest, f.path)
    os.Rename(f.temp.Name(), destPath)
}

func (f *cachedFile) Close() error {
    f.inner.Close()

    if f.read < f.expected {
        // Partial read: discard temp
        os.Remove(f.temp.Name())
    }

    return f.temp.Close()
}
```

## Pruning

Pruning is explicit - the consumer calls it when desired (cron job, startup, manually).

```go
func PruneFileCache(ctx context.Context, path string, strategy PruneStrategy) error {
    // 1. Load all cache.json files from blobs/
    // 2. If MaxAge set: mark entries older than cutoff for removal
    // 3. If MaxSize set: sort remaining by lastAccessed, remove LRU until under size
    // 4. Delete marked directories
    // 5. Optionally: clean up orphaned ref entries pointing to deleted digests
}
```

**Size calculation:** Uses the `size` field from cache.json (recorded at write time) to avoid walking directory trees during prune.

## Registry Interface Changes

The registry interface needs to support digest-based operations separate from ref resolution:

```go
type Registry interface {
    // Existing methods...

    // ResolveRef resolves a ref to its digest without fetching the blob
    ResolveRef(ctx context.Context, ref string) (digest.Digest, error)

    // FetchBlobByDigest fetches a blob by digest
    // The ref is still needed to identify the repository
    FetchBlobByDigest(ctx context.Context, ref string, d digest.Digest) (io.ReadCloser, error)

    // OpenBlobByDigest opens a blob for range requests by digest
    OpenBlobByDigest(ctx context.Context, ref string, d digest.Digest) (estargz.BlobSource, error)
}
```

## What's NOT Included

- Partial blob caching (range tracking)
- Download resumption
- Chunk-level caching
- Automatic pruning (must be called explicitly)
- Cache warming/preloading

## Implementation Order

Suggested order for implementation:

1. **RefCache** - Simplest, standalone, immediately useful
2. **FileCache core** - Dir(), IsComplete(), MarkComplete(), Get(), TouchAccess()
3. **Pull() integration** - newDirHandle, extraction to cache
4. **Registry changes** - Split ref resolution from blob fetching
5. **WrapFile + cachedFile** - The tee-ing wrapper for Stream()
6. **Stream()/BlobHandle integration** - Wire up cache in Open()
7. **Pruning** - PruneStrategy, prune logic
8. **Tests** - Unit tests for each component, integration tests for full flow
