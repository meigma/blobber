---
sidebar_position: 3
---

import ThemedImage from '@theme/ThemedImage';
import useBaseUrl from '@docusaurus/useBaseUrl';

# Architecture

This document explains how blobber is structured internally and how the pieces fit together.

## Design Philosophy

Blobber follows the UNIX philosophy: do one thing well. The core responsibility is moving files to and from OCI registries efficiently. Everything else—authentication, caching, compression—is delegated to focused components.

## Package Structure

```
blobber/
├── doc.go              # Package documentation
├── client.go           # Client type: orchestrates operations
├── image.go            # Image type: cached reader for remote images
├── push.go             # Push operation
├── pull.go             # Pull operation
├── options.go          # Functional options
├── compression.go      # Compression type constructors
├── entry.go            # FileEntry type
├── errors.go           # Sentinel errors
├── archive.go          # Type aliases for archive types
├── cache.go            # Cache-related data types
├── registry.go         # Registry option types
└── internal/
    ├── contracts/      # Internal interfaces shared across components
    ├── registry/       # ORAS-backed registry implementation
    ├── archive/        # eStargz build/read implementation
    ├── safepath/       # Path security validation
    └── cache/          # Blob caching
```

## Internal Contracts

Blobber keeps its internal interfaces in `internal/contracts` so they are not
part of the public API. These contracts are used to decouple components
without exposing them to external users.

### Registry

```go
type Registry interface {
    Push(ctx context.Context, ref string, layer io.Reader, opts *RegistryPushOptions) (string, error)
    Pull(ctx context.Context, ref string) (io.ReadCloser, int64, error)
    PullRange(ctx context.Context, ref string, offset, length int64) (io.ReadCloser, error)
    ResolveLayer(ctx context.Context, ref string) (LayerDescriptor, error)
    // ...
}
```

Handles all OCI registry communication. The default implementation uses ORAS.

### ArchiveBuilder

```go
type ArchiveBuilder interface {
    Build(ctx context.Context, src fs.FS, compression Compression) (*BuildResult, error)
}
```

Creates eStargz archives from filesystems.

### ArchiveReader

```go
type ArchiveReader interface {
    ReadTOC(r io.ReaderAt, size int64) (*TOC, error)
    OpenFile(r io.ReaderAt, size int64, entry TOCEntry) (io.Reader, error)
}
```

Reads eStargz archives, extracting the TOC and individual files.

### PathValidator

```go
type PathValidator interface {
    ValidatePath(path string) error
    ValidateExtraction(destDir string, entries []TOCEntry, limits ExtractLimits) error
    ValidateSymlink(destDir, linkPath, target string) error
}
```

Security validation for paths and extraction operations.

## Data Flow

### Push Operation

<ThemedImage
  alt="Push Operation Sequence Diagram"
  sources={{
    light: useBaseUrl('/img/generated/architecture-push-light.png'),
    dark: useBaseUrl('/img/generated/architecture-push-dark.png'),
  }}
/>

1. User provides an `fs.FS` and reference
2. ArchiveBuilder creates eStargz blob, computing digests
3. Registry pushes blob, then manifest
4. Returns manifest digest to user

### OpenImage + List

<ThemedImage
  alt="OpenImage and List Sequence Diagram"
  sources={{
    light: useBaseUrl('/img/generated/architecture-open-list-light.png'),
    dark: useBaseUrl('/img/generated/architecture-open-list-dark.png'),
  }}
/>

1. User requests to open an image
2. Registry resolves the reference to a layer descriptor
3. Blob is fetched (full or partial depending on lazy loading)
4. TOC is extracted from the blob
5. Image object caches the TOC and reader
6. Subsequent List/Open calls use cached data

### Open (Selective Retrieval)

<ThemedImage
  alt="Selective Open Sequence Diagram"
  sources={{
    light: useBaseUrl('/img/generated/architecture-selective-open-light.png'),
    dark: useBaseUrl('/img/generated/architecture-selective-open-dark.png'),
  }}
/>

1. User requests a specific file
2. Blobber looks up the file's location in the cached TOC
3. Only that file's bytes are fetched via range request
4. Returns reader to user

## Security Model

### Path Validation

All paths are validated before use:

1. **No traversal** - `..` components rejected
2. **No absolute paths** - Must be relative
3. **No null bytes** - Prevents injection
4. **Symlink containment** - Targets must stay within extraction directory

### Extraction Limits

Prevents resource exhaustion:

| Limit | Default |
|-------|---------|
| MaxFiles | 100,000 |
| MaxTotalSize | 10 GB |
| MaxFileSize | 1 GB |

Limits are checked before extraction begins using TOC metadata.

## Caching

### Blob Cache

When enabled, downloaded blobs are stored locally:

```
~/.blobber/cache/
├── blobs/sha256/<digest>      # Blob data
└── entries/sha256/<digest>.json  # Metadata
```

Benefits:
- Skip re-downloading unchanged images
- Faster repeated operations
- Reduced network usage

### Descriptor Cache

Optional in-memory cache for layer descriptors:
- Avoids re-resolving the same reference
- Trade-off: may return stale results for mutable tags

## Dependency Injection

The Client accepts interface implementations, enabling:

- **Testing** - Mock registry for unit tests
- **Flexibility** - Custom implementations if needed
- **Isolation** - Each component testable independently

```go
type Client struct {
    registry     Registry
    builder      ArchiveBuilder
    reader       ArchiveReader
    extractor    Extractor
    validator    PathValidator
    // ...
}
```

## Error Handling

Errors are wrapped with context at each layer:

```go
// In registry layer
return fmt.Errorf("resolving manifest: %w", err)

// In client layer
return "", fmt.Errorf("push failed: %w", err)
```

Sentinel errors enable precise handling:

```go
if errors.Is(err, blobber.ErrNotFound) {
    // Handle not found
}
```

## See Also

- [About eStargz](./about-estargz.md) - The format that enables selective retrieval
- [Why OCI Registries](./why-oci-registries.md) - Registry benefits
- [Library Reference](../reference/library/client.md) - API documentation
