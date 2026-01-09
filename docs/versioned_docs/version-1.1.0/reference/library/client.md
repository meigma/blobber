---
sidebar_position: 1
---

# Client

The Client type provides methods for pushing and pulling files to OCI registries.

## Creating a Client

### NewClient

```go
func NewClient(opts ...ClientOption) (*Client, error)
```

Creates a new blobber client.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `opts` | `...ClientOption` | Configuration options |

**Returns:**

| Type | Description |
|------|-------------|
| `*Client` | The configured client |
| `error` | Error if configuration fails |

**Example:**

```go
client, err := blobber.NewClient()
if err != nil {
    log.Fatal(err)
}
```

With options:

```go
client, err := blobber.NewClient(
    blobber.WithInsecure(true),
    blobber.WithLogger(slog.Default()),
)
```

---

## Methods

### Push

```go
func (c *Client) Push(ctx context.Context, ref string, src fs.FS, opts ...PushOption) (string, error)
```

Uploads files to an OCI registry.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `ctx` | `context.Context` | Context for cancellation |
| `ref` | `string` | Image reference (e.g., `ghcr.io/org/repo:tag`) |
| `src` | `fs.FS` | Filesystem to upload |
| `opts` | `...PushOption` | Push options |

**Returns:**

| Type | Description |
|------|-------------|
| `string` | Manifest digest (e.g., `sha256:abc...`) |
| `error` | Error if push fails |

**Example:**

```go
digest, err := client.Push(ctx, "ghcr.io/org/config:v1", os.DirFS("./config"))
```

With options:

```go
digest, err := client.Push(ctx, ref, files,
    blobber.WithCompression(blobber.ZstdCompression()),
    blobber.WithAnnotations(map[string]string{
        "org.opencontainers.image.source": "https://github.com/org/repo",
    }),
)
```

---

### Pull

```go
func (c *Client) Pull(ctx context.Context, ref, destDir string, opts ...PullOption) error
```

Downloads all files from an image to a local directory.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `ctx` | `context.Context` | Context for cancellation |
| `ref` | `string` | Image reference |
| `destDir` | `string` | Destination directory (created if needed) |
| `opts` | `...PullOption` | Pull options |

**Returns:**

| Type | Description |
|------|-------------|
| `error` | Error if pull fails |

**Example:**

```go
err := client.Pull(ctx, "ghcr.io/org/config:v1", "./output")
```

With extraction limits:

```go
err := client.Pull(ctx, ref, "./output",
    blobber.WithExtractLimits(blobber.ExtractLimits{
        MaxFiles:     1000,
        MaxTotalSize: 100 * 1024 * 1024, // 100MB
    }),
)
```

---

### OpenImage

```go
func (c *Client) OpenImage(ctx context.Context, ref string) (*Image, error)
```

Opens a remote image for reading without downloading the full blob.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `ctx` | `context.Context` | Context for cancellation |
| `ref` | `string` | Image reference |

**Returns:**

| Type | Description |
|------|-------------|
| `*Image` | Handle for reading files |
| `error` | Error if open fails |

**Example:**

```go
img, err := client.OpenImage(ctx, "ghcr.io/org/config:v1")
if err != nil {
    return err
}
defer img.Close()

entries, err := img.List()
```

---

## See Also

- [Image](./image.md) - Reading files from opened images
- [Options](./options.md) - Client and operation options
- [Errors](./errors.md) - Error handling
