---
sidebar_position: 3
---

# Options

Blobber uses functional options for configuration.

## Client Options

Options passed to `NewClient()`.

### WithCredentials

```go
func WithCredentials(registryHost, username, password string) ClientOption
```

Sets explicit credentials for a specific registry.

| Parameter | Type | Description |
|-----------|------|-------------|
| `registryHost` | `string` | Registry hostname (e.g., `ghcr.io`) |
| `username` | `string` | Username or token name |
| `password` | `string` | Password or token |

**Example:**

```go
client, err := blobber.NewClient(
    blobber.WithCredentials("ghcr.io", "username", os.Getenv("GITHUB_TOKEN")),
)
```

---

### WithInsecure

```go
func WithInsecure(insecure bool) ClientOption
```

Allows connections without TLS.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `insecure` | `bool` | `false` | Allow insecure connections |

**Example:**

```go
client, err := blobber.NewClient(
    blobber.WithInsecure(true),
)
```

---

### WithLogger

```go
func WithLogger(logger *slog.Logger) ClientOption
```

Sets a structured logger for debug output.

| Parameter | Type | Description |
|-----------|------|-------------|
| `logger` | `*slog.Logger` | Logger instance |

**Example:**

```go
client, err := blobber.NewClient(
    blobber.WithLogger(slog.Default()),
)
```

---

### WithUserAgent

```go
func WithUserAgent(ua string) ClientOption
```

Sets a custom User-Agent header for HTTP requests.

| Parameter | Type | Description |
|-----------|------|-------------|
| `ua` | `string` | User-Agent string |

**Example:**

```go
client, err := blobber.NewClient(
    blobber.WithUserAgent("myapp/1.0.0"),
)
```

---

### WithCacheDir

```go
func WithCacheDir(path string) ClientOption
```

Enables blob caching at the specified directory.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | `string` | Cache directory path |

**Example:**

```go
client, err := blobber.NewClient(
    blobber.WithCacheDir("/tmp/blobber-cache"),
)
```

---

### WithLazyLoading

```go
func WithLazyLoading(enabled bool) ClientOption
```

Enables on-demand blob fetching for `OpenImage`.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | `bool` | `false` | Enable lazy loading |

When enabled, `OpenImage` fetches only the TOC initially, downloading file contents on-demand.

**Example:**

```go
client, err := blobber.NewClient(
    blobber.WithLazyLoading(true),
)
```

---

### WithDescriptorCache

```go
func WithDescriptorCache(enabled bool) ClientOption
```

Enables in-memory caching of layer descriptors.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | `bool` | `false` | Enable descriptor caching |

**Note:** May return stale results for mutable tags.

---

### WithBackgroundPrefetch

```go
func WithBackgroundPrefetch(enabled bool) ClientOption
```

Enables background downloading of complete blobs during lazy loading.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enabled` | `bool` | `false` | Enable background prefetch |

---

## Push Options

Options passed to `Client.Push()`.

### WithCompression

```go
func WithCompression(c Compression) ClientOption
```

Sets the compression algorithm.

| Parameter | Type | Description |
|-----------|------|-------------|
| `c` | `Compression` | Compression algorithm |

**Compression constructors:**

```go
blobber.GzipCompression() // Default
blobber.ZstdCompression() // Better ratios, faster decompression
```

**Example:**

```go
digest, err := client.Push(ctx, ref, files,
    blobber.WithCompression(blobber.ZstdCompression()),
)
```

---

### WithAnnotations

```go
func WithAnnotations(annotations map[string]string) PushOption
```

Sets OCI annotations on the pushed manifest.

| Parameter | Type | Description |
|-----------|------|-------------|
| `annotations` | `map[string]string` | Key-value annotation pairs |

**Example:**

```go
digest, err := client.Push(ctx, ref, files,
    blobber.WithAnnotations(map[string]string{
        "org.opencontainers.image.source": "https://github.com/org/repo",
        "org.opencontainers.image.version": "1.0.0",
    }),
)
```

---

### WithMediaType

```go
func WithMediaType(mt string) PushOption
```

Sets a custom media type for the layer.

| Parameter | Type | Description |
|-----------|------|-------------|
| `mt` | `string` | Media type string |

---

## Pull Options

Options passed to `Client.Pull()`.

### WithExtractLimits

```go
func WithExtractLimits(limits ExtractLimits) PullOption
```

Sets safety limits for extraction.

**ExtractLimits fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `MaxFiles` | `int` | 100,000 | Maximum file count |
| `MaxTotalSize` | `int64` | 10 GB | Maximum total bytes |
| `MaxFileSize` | `int64` | 1 GB | Maximum per-file bytes |

**Example:**

```go
err := client.Pull(ctx, ref, destDir,
    blobber.WithExtractLimits(blobber.ExtractLimits{
        MaxFiles:     1000,
        MaxTotalSize: 100 * 1024 * 1024, // 100MB
        MaxFileSize:  10 * 1024 * 1024,  // 10MB
    }),
)
```

---

## See Also

- [Client](/reference/library/client) - Client methods
- [Errors](/reference/library/errors) - Error handling
