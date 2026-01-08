# blobber

A Go library and CLI for pushing and pulling files to OCI container registries.

Blobber uses the [eStargz](https://github.com/containerd/stargz-snapshotter/blob/main/docs/estargz.md) format to enable listing and selective retrieval of files without downloading entire images.

## Installation

### CLI

```bash
curl -fsSL https://raw.githubusercontent.com/gilmanlab/blobber/master/install.sh | sh
```

See the [installation docs](https://blobber.gilman.io/getting-started/cli/installation) for other options (Homebrew, Scoop, Nix, Go).

### Library

```bash
go get github.com/gilmanlab/blobber
```

## Quick Start

### CLI

```bash
# Push a directory to a registry
blobber push ./config ghcr.io/myorg/config:v1

# List files without downloading
blobber list ghcr.io/myorg/config:v1

# Stream a single file to stdout
blobber cat ghcr.io/myorg/config:v1 app.yaml

# Pull all files to a local directory
blobber pull ghcr.io/myorg/config:v1 ./output
```

### Library

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/gilmanlab/blobber"
)

func main() {
    ctx := context.Background()
    client, _ := blobber.NewClient()

    // Push a directory
    digest, _ := client.Push(ctx, "ghcr.io/myorg/config:v1", os.DirFS("./config"))
    fmt.Println(digest)

    // Open an image and list files
    img, _ := client.OpenImage(ctx, "ghcr.io/myorg/config:v1")
    defer img.Close()

    entries, _ := img.List()
    for _, e := range entries {
        fmt.Printf("%s (%d bytes)\n", e.Path(), e.Size())
    }

    // Read a single file
    rc, _ := img.Open("app.yaml")
    defer rc.Close()
    // ... read from rc
}
```

## Features

- **List without download** - View file contents using only the eStargz table of contents
- **Selective retrieval** - Stream individual files via HTTP range requests
- **Any OCI registry** - Works with GHCR, Docker Hub, ECR, GCR, or self-hosted registries
- **Compression options** - gzip (default) or zstd
- **Local caching** - Cache blobs locally for faster repeated operations

## Configuration

Blobber stores configuration in `~/.config/blobber/config.yaml` (following XDG conventions).

```bash
# View current configuration
blobber config

# Create default config file
blobber config init

# Disable caching
blobber config set cache.enabled false

# Bypass cache for a single command
blobber --no-cache pull ghcr.io/myorg/config:v1 ./output
```

Configuration precedence: flags > environment variables > config file > defaults.

## Documentation

Full documentation is available at [blobber.gilman.io](https://blobber.gilman.io):

- [CLI Getting Started](https://blobber.gilman.io/getting-started/cli/installation)
- [Library Getting Started](https://blobber.gilman.io/getting-started/library/installation)
- [CLI Reference](https://blobber.gilman.io/reference/cli/push)
- [Library Reference](https://blobber.gilman.io/reference/library/client)
- [Configuration Guide](https://blobber.gilman.io/how-to/configure-blobber)

## Authentication

Blobber uses Docker credentials from `~/.docker/config.json`. If you can `docker push` to a registry, blobber can too.

```bash
docker login ghcr.io
```

## License

MIT License - see [LICENSE](LICENSE) for details.
