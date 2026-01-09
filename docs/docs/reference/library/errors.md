---
sidebar_position: 4
---

# Errors

Blobber exports sentinel errors for precise error handling.

## Sentinel Errors

### ErrNotFound

```go
var ErrNotFound = core.ErrNotFound
```

The requested image or file was not found.

**When returned:**

- Image reference doesn't exist in registry
- File path doesn't exist in image
- Layer digest not found

**Example:**

```go
img, err := client.OpenImage(ctx, ref)
if errors.Is(err, blobber.ErrNotFound) {
    log.Printf("Image not found: %s", ref)
    return nil
}
```

---

### ErrUnauthorized

```go
var ErrUnauthorized = core.ErrUnauthorized
```

Authentication failed.

**When returned:**

- Invalid credentials
- Token expired
- Insufficient permissions

**Example:**

```go
_, err := client.Push(ctx, ref, files)
if errors.Is(err, blobber.ErrUnauthorized) {
    return fmt.Errorf("authentication failed: check your credentials")
}
```

---

### ErrInvalidRef

```go
var ErrInvalidRef = core.ErrInvalidRef
```

The image reference is malformed.

**When returned:**

- Invalid registry hostname
- Missing repository name
- Invalid tag or digest format

**Example:**

```go
if errors.Is(err, blobber.ErrInvalidRef) {
    return fmt.Errorf("invalid reference format: %s", ref)
}
```

---

### ErrPathTraversal

```go
var ErrPathTraversal = core.ErrPathTraversal
```

A path traversal attack was detected.

**When returned:**

- Archive contains `..` path components
- Absolute paths in archive
- Symlinks pointing outside extraction directory

**Example:**

```go
if errors.Is(err, blobber.ErrPathTraversal) {
    return fmt.Errorf("security violation: path traversal detected")
}
```

---

### ErrExtractLimits

```go
var ErrExtractLimits = core.ErrExtractLimits
```

Extraction safety limits were exceeded.

**When returned:**

- File count exceeds `MaxFiles`
- Total size exceeds `MaxTotalSize`
- Single file exceeds `MaxFileSize`

**Example:**

```go
err := client.Pull(ctx, ref, destDir)
if errors.Is(err, blobber.ErrExtractLimits) {
    return fmt.Errorf("extraction limits exceeded: archive too large")
}
```

---

### ErrInvalidArchive

```go
var ErrInvalidArchive = core.ErrInvalidArchive
```

The blob is not a valid eStargz archive.

**When returned:**

- Corrupted archive data
- Missing TOC (table of contents)
- Unsupported compression format

**Example:**

```go
if errors.Is(err, blobber.ErrInvalidArchive) {
    return fmt.Errorf("invalid archive: not a valid eStargz blob")
}
```

---

### ErrClosed

```go
var ErrClosed = core.ErrClosed
```

Operation attempted on a closed resource.

**When returned:**

- Calling methods on a closed Image
- Reading from a closed file handle

**Example:**

```go
if errors.Is(err, blobber.ErrClosed) {
    return fmt.Errorf("resource already closed")
}
```

---

### ErrRangeNotSupported

```go
var ErrRangeNotSupported = core.ErrRangeNotSupported
```

The registry doesn't support HTTP range requests.

**When returned:**

- Attempting selective file retrieval on incompatible registry

**Example:**

```go
if errors.Is(err, blobber.ErrRangeNotSupported) {
    // Fall back to full download
    return client.Pull(ctx, ref, destDir)
}
```

---

### ErrSignatureInvalid

```go
var ErrSignatureInvalid = core.ErrSignatureInvalid
```

Signature verification failed.

**When returned:**

- Signature doesn't match the artifact content (possible tampering)
- Signer identity doesn't match expected issuer/subject
- Certificate chain validation failed
- Transparency log proof invalid

**Example:**

```go
_, err := client.OpenImage(ctx, ref)
if errors.Is(err, blobber.ErrSignatureInvalid) {
    return fmt.Errorf("artifact may be tampered or signed by unexpected identity")
}
```

---

### ErrNoSignature

```go
var ErrNoSignature = core.ErrNoSignature
```

No signature was found when verification was required.

**When returned:**

- Client configured with `WithVerifier` but artifact has no signature
- No referrer artifacts of signature type exist

**Example:**

```go
err := client.Pull(ctx, ref, destDir)
if errors.Is(err, blobber.ErrNoSignature) {
    return fmt.Errorf("artifact is not signed; push with --sign first")
}
```

---

## Error Handling Pattern

Use `errors.Is()` for sentinel error checking:

```go
import "errors"

func handleImage(ctx context.Context, client *blobber.Client, ref string) error {
    img, err := client.OpenImage(ctx, ref)
    switch {
    case errors.Is(err, blobber.ErrNotFound):
        return fmt.Errorf("image %s not found", ref)
    case errors.Is(err, blobber.ErrUnauthorized):
        return fmt.Errorf("not authorized to access %s", ref)
    case errors.Is(err, blobber.ErrInvalidRef):
        return fmt.Errorf("invalid image reference: %s", ref)
    case errors.Is(err, blobber.ErrNoSignature):
        return fmt.Errorf("image %s is not signed", ref)
    case errors.Is(err, blobber.ErrSignatureInvalid):
        return fmt.Errorf("image %s has invalid signature", ref)
    case err != nil:
        return fmt.Errorf("opening image: %w", err)
    }
    defer img.Close()

    // ... use img
    return nil
}
```

---

## See Also

- [Client](/docs/reference/library/client) - Methods that return these errors
- [Options](/docs/reference/library/options) - Configuring limits
