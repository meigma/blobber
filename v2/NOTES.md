# Blobber Redesign Notes

## Goal

Define a consistent domain language for Blobber that can be used in both code and documentation. The language should:
- Reuse OCI terminology where appropriate
- Be specific to Blobber's constrained feature set
- Make it easy to reason about internal operations

## Domain Language

### Core Terms

| Term | Definition |
|------|------------|
| **Blob** | An estargz archive holding user files. The digestable, contiguous binary representation that Blobber creates, stores, and retrieves. Users interact with Blob contents via BlobHandle, not directly. |
| **BlobHandle** | Implements `fs.FS`; provides access to a Blob's contents. Can be backed by a local file or a network stream. |
| **Blob Reference** | A tag or digest that addresses a Blob in a Registry. Can be a concrete type or string alias. |
| **Blob Manifest** | A thin wrapper around the OCI Image Manifest. Contains a single layer pointing to the Blob. First-class citizen to support provenance features (signatures, SBOMs, SLSA attestations). |
| **Referrer** | An OCI artifact attached to a Blob Manifest via the `subject` field. Used for signatures, SBOMs, SLSA provenance, etc. |
| **Registry** | An OCI registry where Blobs are stored. |

### Supporting Terms (OCI-aligned)

| Term | Definition |
|------|------------|
| **Layer Descriptor** | The OCI descriptor in the manifest pointing to the Blob's location in the Registry. |
| **Artifact Type** | Media type identifying what kind of referrer an artifact is (e.g., `application/spdx+json` for SBOM). |

### Intentionally Excluded from Domain Language

- **User input (fs.FS)**: We say "Blobber takes user files and creates a Blob" without giving the input a domain name. This keeps flexibility for different input types (fs.FS, directory path, etc.)
- **TOC (Table of Contents)**: Implementation detail of estargz. Enables the BlobHandle abstraction but users don't need to know about it.
- **Image Config**: Implementation detail of OCI manifests.
- **DSSE Envelope**: Implementation detail of how SLSA provenance is signed. Users don't need to know about it.

## Core Types

```go
// BlobManifest represents a pushed blob's manifest
type BlobManifest struct {
    Digest    string     // Manifest digest: "sha256:abc..."
    Size      int64      // Manifest size in bytes
    Referrers []Referrer // Attached artifacts (eager-fetched by default)
}

// Referrer represents an artifact attached to a manifest
type Referrer struct {
    Digest       string            // Referrer manifest digest
    ArtifactType string            // e.g., "application/spdx+json"
    Size         int64
    Annotations  map[string]string
}

// PushResult contains the result of a Push operation
type PushResult struct {
    Reference string       // Digest reference: "ghcr.io/org/repo@sha256:abc..."
    Manifest  BlobManifest // The pushed blob's manifest
}

// BlobHandle provides fs.FS access to blob contents
type BlobHandle struct {
    // Implements fs.FS, fs.ReadDirFS, etc.
    // Also provides Close() for cleanup
}
```

## Operations

### Push
```
User files (fs.FS) → Blob → Registry
```
- `Push()` creates a Blob from user files and stores it in the target Registry
- Returns a `PushResult` containing both the Blob Reference and the Blob Manifest

### FetchManifest
```
Blob Reference → Blob Manifest (with Referrers)
```
- `FetchManifest()` retrieves the Blob Manifest from the Registry without pulling the Blob
- Referrers are eagerly fetched by default (metadata only, not content)
- Enables verification-before-pull workflows

### Pull
```
Blob Reference → BlobHandle (local-backed) → User files
```
- `Pull()` downloads a Blob locally and returns a `BlobHandle`
- Accepts optional `Policy` for verification before download
- BlobHandle is backed by a local temp file
- User can extract files via utility functions or work with `fs.FS` directly
- `Close()` cleans up the underlying temp file

### Stream
```
Blob Reference → BlobHandle (network-backed) → User files
```
- `Stream()` returns a `BlobHandle` directly (no local artifact)
- Accepts optional `Policy` for verification before streaming
- BlobHandle is backed by network requests using TOC for byte ranges
- User accesses files via `fs.FS` interface

### AttachArtifact
```
Blob Manifest + Artifact bytes → Referrer in Registry
```
- `AttachArtifact()` attaches arbitrary content to a Blob Manifest as an OCI referrer
- Artifact-agnostic: Blobber doesn't interpret the content, just stores it
- Returns the referrer digest for optional follow-up operations (e.g., signing)

## API Sketch

```go
// Push - returns PushResult with both reference and manifest
result, err := client.Push(ctx, "ghcr.io/org/repo:tag", myFS)
// result.Reference - the Blob Reference (digest)
// result.Manifest  - the Blob Manifest (with empty Referrers initially)

// FetchManifest - retrieve manifest with referrers
manifest, err := client.FetchManifest(ctx, "ghcr.io/org/repo:tag")
// manifest.Referrers contains attached artifacts (metadata only)

// AttachArtifact - attach content to a manifest
referrerDigest, err := client.AttachArtifact(ctx, result.Manifest,
    "application/spdx+json", sbomBytes)
// referrerDigest can be used for signing (see Extras)

// Pull with policy - verify before downloading
handle, err := client.Pull(ctx, "ghcr.io/org/repo:tag",
    blobber.WithPolicy(myPolicy))
err = blobber.CopyTo(handle, "/path/to/extract")
err = handle.Close()

// Stream with policy - selective file access
handle, err := client.Stream(ctx, "ghcr.io/org/repo:tag",
    blobber.WithPolicy(myPolicy))

// Single file: use fs.FS + stdlib (no Blobber helper needed)
f, err := handle.Open("config.yaml")
defer f.Close()
dest, _ := os.Create("/local/config.yaml")
io.Copy(dest, f)

// Or extract everything (optimizes to single fetch)
err = blobber.CopyTo(handle, "/path/to/extract")
err = handle.Close()
```

## Policy-Based Verification

Verification is decoupled from the core Blobber loop. Users provide a `Policy` to `Pull()` or `Stream()` that inspects the Blob Manifest and its referrers before allowing access to blob contents.

### Policy Interface

```go
// Policy verifies a blob manifest meets requirements
type Policy interface {
    Verify(ctx context.Context, manifest BlobManifest, fetcher ReferrerFetcher) error
}

// ReferrerFetcher allows policies to fetch referrer content when needed
type ReferrerFetcher interface {
    FetchReferrer(ctx context.Context, ref string, digest string) ([]byte, error)
}
```

### How It Works

1. `Pull()`/`Stream()` calls `FetchManifest()` to get manifest + referrer metadata
2. If a policy is provided, Blobber calls `policy.Verify(ctx, manifest, registry)`
3. Policy inspects `manifest.Referrers` for required artifact types
4. Policy calls `fetcher.FetchReferrer()` to get content when needed (e.g., signature bytes)
5. Policy returns `nil` to proceed, or `error` to block the operation

### Design Rationale

- **Core stays clean**: Blobber doesn't know what a "valid signature" or "valid SBOM" looks like
- **Composable**: Users can combine multiple policies (require signature AND SLSA)
- **Per-operation**: Different blobs can have different policies
- **Artifact-agnostic**: Same pattern works for any referrer type

## Referrers and Artifacts

### Eager vs Lazy Fetching

Referrer metadata is **eagerly fetched by default** when calling `FetchManifest()`. This populates `manifest.Referrers` with artifact types, digests, and annotations.

Referrer **content** is fetched on-demand by policies via `ReferrerFetcher`. This avoids downloading potentially large artifacts (SBOMs, attestations) unless actually needed.

An option can disable eager fetching for performance-critical paths where referrers aren't needed.

### AttachArtifact Flow

When `AttachArtifact()` is called:

1. Push content bytes as a blob → content digest
2. Create referrer manifest with:
   - `artifactType`: the provided media type
   - `subject`: pointing to the Blob Manifest digest
   - `layers`: containing the content blob
3. Push referrer manifest → referrer digest
4. Return referrer digest to caller

The referrer digest can be used for follow-up operations like signing.

## Extras (Separate Modules)

The core Blobber API is artifact-agnostic. Specialized functionality lives in separate submodules:

### sigstore/ - Cryptographic Signing

Provides helpers for signing blobs and referrers using Sigstore:

```go
// Sign a blob manifest
err = client.Sign(ctx, result.Manifest.Digest, sigstore.NewSigner(...))

// Sign an attached artifact (e.g., SBOM)
err = client.Sign(ctx, sbomReferrerDigest, sigstore.NewSigner(...))

// Create a signature verification policy
policy := sigstore.RequireSignature(sigstore.NewVerifier(...))
handle, err := client.Pull(ctx, ref, blobber.WithPolicy(policy))
```

### slsa/ - SLSA Provenance (Planned)

Provides helpers for attaching and verifying SLSA Build Provenance:

```go
// Attach pre-signed SLSA provenance (signed by build system)
referrerDigest, err := client.AttachArtifact(ctx, manifest,
    slsa.ArtifactType, provenanceBytes)

// Create an SLSA verification policy
policy := slsa.RequireProvenance(slsa.NewVerifier(...))
```

Note: SLSA provenance is typically pre-signed by the build system (GitHub Actions, etc.) using a DSSE envelope. No additional signing is needed when attaching.

### sbom/ - SBOM Support (Planned)

Provides helpers for SBOM generation and attachment:

```go
// Generate and attach SBOM
sbomBytes := sbom.Generate(myFS, sbom.FormatSPDX)
referrerDigest, err := client.AttachArtifact(ctx, manifest,
    sbom.SPDXArtifactType, sbomBytes)

// Optionally sign the SBOM
err = client.Sign(ctx, referrerDigest, sigstore.NewSigner(...))
```

## Design Decisions

### Why "Blob" everywhere?
The project is called "Blobber" because it moves blobs around. A blob is any contiguous, digestable binary data. Blobber's "magic" is turning arbitrary files into a blob (estargz archive) that can be stored in OCI registries. We use "Blob" as the domain term rather than "archive" or distinguishing between local/remote states.

### Why BlobHandle implements fs.FS?
- Standard library alignment: users get `fs.ReadDir()`, `fs.WalkDir()`, etc. for free
- Symmetry: user provides `fs.FS`, user gets back `fs.FS`-compatible handle
- Abstraction: user doesn't care if Blob is local or remote
- Extraction becomes a utility function, not a core operation

### BlobHandle needs Close()
`fs.FS` doesn't define `Close()`, but BlobHandle needs cleanup (connections, temp files, cached TOC). BlobHandle will be a concrete type that implements `fs.FS` plus `Close()` and any other utility methods.

### Pull and Stream both return BlobHandle
Both operations return `BlobHandle`; the difference is the backing:
- Pull: BlobHandle backed by local temp file
- Stream: BlobHandle backed by network requests

Users don't need to care about the difference. `Close()` handles cleanup in both cases.

### Blob is transport, not a user-facing artifact
The Blob (estargz archive) is an intermediate transport format. Users should not work with it directly on the filesystem. They interact with Blob contents via BlobHandle's `fs.FS` interface. If they need files on disk, they use `CopyTo()`.

### TOC fetching is eager
For network-backed BlobHandle, the TOC must be fetched before `Open()` can work. Eager fetching (at Stream/Handle creation) is preferred to fail fast on invalid Blobs.

### Blob Manifest is first-class
The Blob Manifest is exposed as a core domain type (not hidden as an implementation detail) because:
- It's the anchor point for OCI referrers (signatures, SBOMs, SLSA attestations)
- Provenance features attach to the manifest
- Enables verification-before-pull workflows

### AttachArtifact is artifact-agnostic
Blobber core doesn't interpret artifact content. It just:
1. Stores bytes with an artifact type
2. Creates the OCI referrer structure
3. Returns the referrer digest

Interpretation happens in the extras modules (sigstore/, slsa/, sbom/).

### Signing is separate from attachment
`AttachArtifact()` does not accept a signer. Signing is a separate operation:
1. Attach artifact → get referrer digest
2. Sign referrer digest → creates signature referrer

This keeps concerns separated and the core API simple.

### Policy on Pull/Stream, not Client
Policies are passed to individual operations, not configured at client construction. This allows different blobs to have different verification requirements within the same client.

### Policy receives ReferrerFetcher
Policies may need to fetch referrer content (e.g., signature bytes for verification). Rather than eagerly fetching all content, the policy receives a `ReferrerFetcher` interface to fetch on-demand.

## BlobHandle Implementation

BlobHandle is a unified type that can be backed by either a local file or network requests. Both expose the same `fs.FS` interface.

### Local-Backed (from Pull)

- Blob is downloaded to a temp file
- `Open()` reads directly from local estargz archive
- Fast, seekable, supports repeated reads
- `Close()` cleans up the temp file

### Network-Backed (from Stream)

- No local artifact; file content fetched on-demand via HTTP range requests
- Designed for selective access to large blobs

**TOC Handling:**
- TOC (Table of Contents) is fetched eagerly at handle creation
- Enables `Open()` to know byte offsets without additional network calls
- Fail-fast: invalid blobs are detected immediately

**File Content Fetching:**
- Content is fetched lazily on first `Read()`, not at `Open()` time
- Streams directly to caller (no in-memory buffering)
- Re-reading the same bytes requires a new range request (no caching)

**Context Handling:**
- `fs.File.Read()` doesn't accept a context
- Context is stored on the BlobHandle at creation time
- All network requests use this stored context

**Seek Support:**
- Seek is supported via `io.Seeker` interface
- `Seek()` updates internal position (no network call)
- Next `Read()` issues a range request from the new position
- Seeking backwards and re-reading means re-fetching (streaming tradeoff)

### Use Case Guidance

| Use Case | Method | Why |
|----------|--------|-----|
| Need all/most files | `Pull()` | One download, fast local access |
| Need 1-2 files from large blob | `Stream()` | Fetch only what you read |
| Unsure which files you need | `Pull()` | Safer default, no latency surprises |

### CopyTo Optimization

`CopyTo()` is the only extraction helper. It exists because manually walking `fs.FS` and copying files is tedious boilerplate.

When `CopyTo()` is called on a network-backed handle, it optimizes by fetching the entire blob in a single request and streaming through extraction—rather than issuing per-file range requests.

```go
func CopyTo(h *BlobHandle, dest string) error {
    if h.isNetworkBacked() {
        // Single HTTP request, stream through decompression, extract
        return h.streamAllTo(dest)
    }
    // Local: walk fs.FS, copy files
    return walkAndCopy(h, dest)
}
```

For single-file access, users use standard `fs.FS` patterns with stdlib functions:

```go
f, _ := handle.Open("config.yaml")
defer f.Close()
io.Copy(os.Stdout, f)  // Single range request for this file
```

No `CopyFileTo()` helper exists—stdlib is sufficient and keeps the API minimal.

## Caching

### Decision: No Partial Blob Caching

Blobber caches **complete blobs only**. Partial/chunk-level caching is explicitly not supported.

**Rationale:**
- eStargz chunks have natural boundaries, but caching at chunk level introduces significant complexity:
  - Cache key management (blob digest + chunk offset)
  - Consistency tracking (which chunks are cached?)
  - Stale data detection requires per-chunk verification
  - Cache eviction partially breaks file access
- The complexity isn't worth it for Blobber's expected use cases

**Use Case Guidance:**

| Use Case | Method | Caching |
|----------|--------|---------|
| Need all/most files | `Pull()` | Full blob cached |
| Need a few files from large blob | `Stream()` | No caching (range requests) |
| Need a few files repeatedly | `Pull()` | Pay full download once, fast local access after |

**Streaming still uses range requests** for selective retrieval—you just don't get caching. If you stream the same files repeatedly, the answer is to pull the full blob.

## Implementation Practices

### Structured Logging

We use `log/slog` throughout the codebase to improve debugging capabilities. Key practices:

- All components accept an optional logger via functional options (e.g., `WithLogger(*slog.Logger)`)
- When no logger is provided, a no-op logger (`slog.New(slog.NewTextHandler(io.Discard, nil))`) is used to avoid nil checks
- Debug-level logging is used for operational details (e.g., "path validation failed", "entry added to tar")
- Errors returned to callers remain generic for security (e.g., `ErrPathTraversal`), while debug logs provide specifics

This approach allows operators to enable debug logging when troubleshooting without exposing sensitive details in error messages.

### Path Traversal Protection with os.Root

Go 1.24 introduced `os.Root`, a traversal-resistant filesystem API designed specifically for safely extracting archives. We use this API for all extraction operations.

**What os.Root provides:**

| Protection | Description |
|------------|-------------|
| Symlink traversal | Prevents symlinks from escaping the root directory |
| Path traversal | Blocks `..` components that would escape |
| Absolute paths | Rejects paths that don't stay within root |
| TOCTOU races | Atomic operations via `openat(2)` on Unix |
| Windows device names | Blocks reserved names like `NUL`, `COM1` |

**How it works:**

```go
// Open a root directory - all operations are confined here
root, err := os.OpenRoot(destDir)
if err != nil {
    return err
}
defer root.Close()

// These operations cannot escape destDir, even via symlinks
root.Mkdir("subdir", 0755)           // Safe directory creation
root.Create("subdir/file.txt")       // Safe file creation
root.Symlink("target", "linkname")   // Safe symlink creation
root.OpenFile("path", flags, mode)   // Safe file open
```

**Why both PathValidator and os.Root?**

We use a defense-in-depth approach:

1. **PathValidator** (lexical validation) - Rejects obviously malicious paths early, provides debug logging for why paths were rejected, catches Windows-specific bypasses (trailing dots/spaces)

2. **os.Root** (kernel enforcement) - Provides atomic, race-free protection at the syscall level, handles edge cases we might miss in lexical validation

PathValidator fails fast with clear diagnostics; os.Root provides the security guarantee.

**Platform notes:**

- Unix: Uses `openat(2)` syscalls for atomic traversal-resistant operations
- Windows: Uses directory handles; prevents rename/deletion while open
- Minimum version: Go 1.24.3+ (earlier 1.24.x had a CVE)

**Limitations:**

- Does not prevent traversal via mount points (bind mounts require root privileges to create, so this is an acceptable threat model)
- GOOS=js (Node.js) is vulnerable to TOCTOU races due to platform limitations

## Open Questions

- **Error types**: Do we need domain-specific error terminology?
- **PushResult**: What other fields might be useful? (Size, annotations, timing?)
- **Policy composition**: How do users combine multiple policies? `blobber.AllOf(policy1, policy2)`?
- **Referrer annotations**: Should `AttachArtifact` accept optional annotations?
- **BlobHandle.Path()**: Intentionally omitted (Blob is transport, not filesystem artifact). Revisit if users need to pass blob location to external tools.
