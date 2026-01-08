---
sidebar_position: 2
---

# Image

The Image type provides methods for reading files from a remote OCI image without downloading the entire blob.

## Obtaining an Image

Images are created via `Client.OpenImage`:

```go
img, err := client.OpenImage(ctx, "ghcr.io/org/config:v1")
if err != nil {
    return err
}
defer img.Close()
```

Always call `Close()` when done to release resources.

---

## Methods

### List

```go
func (img *Image) List() ([]FileEntry, error)
```

Returns all files in the image.

**Returns:**

| Type | Description |
|------|-------------|
| `[]FileEntry` | Slice of file entries |
| `error` | Error if listing fails |

**Example:**

```go
entries, err := img.List()
if err != nil {
    return err
}

for _, entry := range entries {
    fmt.Printf("%s: %d bytes\n", entry.Path(), entry.Size())
}
```

---

### Open

```go
func (img *Image) Open(path string) (io.ReadCloser, error)
```

Opens a specific file for reading.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `path` | `string` | File path within the image |

**Returns:**

| Type | Description |
|------|-------------|
| `io.ReadCloser` | Reader for file contents |
| `error` | Error if file not found or read fails |

**Example:**

```go
rc, err := img.Open("config.yaml")
if err != nil {
    return err
}
defer rc.Close()

content, err := io.ReadAll(rc)
```

---

### Walk

```go
func (img *Image) Walk(fn fs.WalkDirFunc) error
```

Walks the file tree, calling fn for each file or directory.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `fn` | `fs.WalkDirFunc` | Callback function |

**Returns:**

| Type | Description |
|------|-------------|
| `error` | Error from callback or walk failure |

**Example:**

```go
err := img.Walk(func(path string, d fs.DirEntry, err error) error {
    if err != nil {
        return err
    }
    if !d.IsDir() {
        info, _ := d.Info()
        fmt.Printf("%s: %d bytes\n", path, info.Size())
    }
    return nil
})
```

---

### Close

```go
func (img *Image) Close() error
```

Releases resources associated with the image.

**Returns:**

| Type | Description |
|------|-------------|
| `error` | Error if close fails |

**Example:**

```go
defer img.Close()
```

---

## FileEntry

Files returned by `List()` implement `fs.DirEntry`:

```go
type FileEntry struct {
    // ...
}

func (f FileEntry) Name() string      // Base name
func (f FileEntry) Path() string      // Full path
func (f FileEntry) Size() int64       // Size in bytes
func (f FileEntry) Mode() fs.FileMode // File mode
func (f FileEntry) IsDir() bool       // Always false for files
func (f FileEntry) Type() fs.FileMode // File type bits
func (f FileEntry) Info() (fs.FileInfo, error)
```

**Example:**

```go
entries, _ := img.List()
for _, e := range entries {
    fmt.Printf("%s: %d bytes, mode %o\n", e.Path(), e.Size(), e.Mode())
}
```

---

## Thread Safety

The Image type is safe for concurrent use. Multiple goroutines can call `List()`, `Open()`, and `Walk()` simultaneously.

---

## See Also

- [Client](/docs/reference/library/client) - Creating clients and opening images
- [Tutorial: Library Basics](/docs/tutorials/library-basics) - Step-by-step usage guide
