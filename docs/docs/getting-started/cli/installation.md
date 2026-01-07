---
sidebar_position: 1
---

# Installation

Install the blobber CLI.

## Using Go

If you have Go 1.21+ installed:

```bash
go install github.com/gilmanlab/blobber/cmd/blobber@latest
```

Verify the installation:

```bash
blobber --help
```

## From Source

Clone and build from source:

```bash
git clone https://github.com/gilmanlab/blobber.git
cd blobber
go build -o blobber ./cmd/blobber
```

Move the binary to your PATH:

```bash
mv blobber /usr/local/bin/
```

## Requirements

- **Docker credentials** configured for authenticated registries

Blobber uses your existing Docker credentials from `~/.docker/config.json`. If you can `docker push` to a registry, blobber can too.

## Next Steps

- [Quickstart](/getting-started/cli/quickstart) - Push and pull your first files
- [CLI Tutorial](/tutorials/cli-basics) - Learn all CLI features step-by-step
