# blobber

A Go library and CLI for pushing and pulling arbitrary files to OCI container registries.

> **Note:** This project is under active development and not yet ready for use.

## Overview

Blobber treats OCI container registries as general-purpose file storage. It uses the eStargz (seekable tar.gz) format to enable:

- **File listing without download** - View contents of a remote image without pulling the full blob
- **Selective file retrieval** - Stream individual files on-demand
- **Compression options** - Choose between gzip (default) and zstd

## Installation

```bash
go install github.com/gilmanlab/blobber/cmd/blobber@latest
```

Or as a library:

```bash
go get github.com/gilmanlab/blobber
```

## Usage

### CLI

```bash
# Push a directory
blobber push ./config ghcr.io/org/config:v1

# List files (without downloading)
blobber list ghcr.io/org/config:v1

# Pull all files
blobber pull ghcr.io/org/config:v1 ./local-config

# Stream a single file
blobber cat ghcr.io/org/config:v1 app/config.yaml
```

### Library

```go
client, err := blobber.NewClient()
if err != nil {
    log.Fatal(err)
}

// Push
digest, err := client.Push(ctx, "ghcr.io/org/config:v1", os.DirFS("./config"))

// List
entries, err := client.List(ctx, "ghcr.io/org/config:v1")

// Open single file
rc, err := client.Open(ctx, "ghcr.io/org/config:v1", "app/config.yaml")
```

## License

MIT License - see [LICENSE](LICENSE) for details.
