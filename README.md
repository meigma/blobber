# Blobber

[![CI](https://github.com/meigma/blobber/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/meigma/blobber/actions/workflows/ci.yml)
[![Docs](https://img.shields.io/badge/docs-blobber.meigma.dev-blue)](https://blobber.meigma.dev/docs)
[![Godocs](https://pkg.go.dev/badge/github.com/meigma/blobber.svg)](https://pkg.go.dev/github.com/meigma/blobber)
[![Release](https://img.shields.io/github/v/release/meigma/blobber)](https://github.com/meigma/blobber/releases)
[![License](https://img.shields.io/github/license/meigma/blobber)](LICENSE)

> A Go library and CLI for securely pushing and pulling files to OCI container registries.

Blobber uses the [eStargz](https://github.com/containerd/stargz-snapshotter/blob/main/docs/estargz.md) format to enable listing and selective retrieval of files without downloading entire images.
Listing and streaming require eStargz images; Blobber pushes eStargz by default.

## Quick Start

```bash
curl -fsSL https://blobber.meigma.dev/install.sh | sh

# Push a directory to a registry
blobber push ./config ghcr.io/myorg/config:v1

# List files without downloading
blobber list ghcr.io/myorg/config:v1
```

## Installation

### CLI

```bash
curl -fsSL https://blobber.meigma.dev/install.sh | sh
```

See the [installation docs](https://blobber.meigma.dev/docs/getting-started/cli/installation) for other options (Homebrew, Scoop, Nix, Go).

### Library

```bash
go get github.com/meigma/blobber
```

## Examples

### CLI

```bash
# Push a directory to a registry
blobber push ./config ghcr.io/myorg/config:v1

# Push with Sigstore signing
blobber push --sign ./config ghcr.io/myorg/config:v1

# List files without downloading
blobber list ghcr.io/myorg/config:v1

# Stream a single file to stdout
blobber cat ghcr.io/myorg/config:v1 app.yaml

# Pull all files to a local directory
blobber pull ghcr.io/myorg/config:v1 ./output

# Pull with signature verification
blobber pull --verify \
  --verify-issuer https://accounts.google.com \
  --verify-subject dev@company.com \
  ghcr.io/myorg/config:v1 ./output
```

### Library

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/meigma/blobber"
    "github.com/meigma/blobber/sigstore"
)

func main() {
    ctx := context.Background()

    // Create a client with Sigstore signing
    signer, _ := sigstore.NewSigner(sigstore.WithEphemeralKey())
    client, err := blobber.NewClient(blobber.WithSigner(signer))
    if err != nil {
        log.Fatal(err)
    }

    // Push a directory (automatically signed)
    digest, err := client.Push(ctx, "ghcr.io/myorg/config:v1", os.DirFS("./config"))
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(digest)

    // Create a client with signature verification
    verifier, _ := sigstore.NewVerifier(
        sigstore.WithIdentity("https://accounts.google.com", "dev@company.com"),
    )
    verifiedClient, _ := blobber.NewClient(blobber.WithVerifier(verifier))

    // Open an image (signature verified before access)
    img, err := verifiedClient.OpenImage(ctx, "ghcr.io/myorg/config:v1")
    if err != nil {
        log.Fatal(err)
    }
    defer img.Close()

    entries, _ := img.List()
    for _, e := range entries {
        fmt.Printf("%s (%d bytes)\n", e.Path(), e.Size())
    }
}
```

## Features

- **List without download** - View file contents using only the eStargz table of contents
- **Selective retrieval** - Stream individual files via HTTP range requests
- **Sigstore signing** - Sign artifacts for supply chain security; verify before pulling
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

Full documentation is available at [blobber.meigma.dev](https://blobber.meigma.dev):

- [CLI Getting Started](https://blobber.meigma.dev/docs/getting-started/cli/installation)
- [Library Getting Started](https://blobber.meigma.dev/docs/getting-started/library/installation)
- [Signing & Verification](https://blobber.meigma.dev/docs/how-to/sign-artifacts)
- [CLI Reference](https://blobber.meigma.dev/docs/reference/cli/push)
- [Library Reference](https://blobber.meigma.dev/docs/reference/library/client)
- [Configuration Guide](https://blobber.meigma.dev/docs/how-to/configure-blobber)

## Authentication

Blobber uses Docker credentials from `~/.docker/config.json`. If you can `docker push` to a registry, blobber can too.

```bash
docker login ghcr.io
```

## License

MIT License - see [LICENSE](LICENSE) for details.
