---
sidebar_position: 1
---

# blobber push

Upload a directory to an OCI registry.

## Synopsis

```bash
blobber push <directory> <reference> [flags]
```

## Description

Uploads all files from a local directory to an OCI registry as an eStargz-compressed image layer. The directory structure is preserved.

## Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `directory` | Yes | Path to the directory to upload |
| `reference` | Yes | OCI image reference (e.g., `ghcr.io/org/repo:tag`) |

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--compression` | string | `gzip` | Compression algorithm: `gzip` or `zstd` |
| `--insecure` | bool | `false` | Allow connections without TLS |
| `-v, --verbose` | bool | `false` | Enable debug logging |

## Output

On success, prints the SHA256 digest of the pushed manifest:

```
sha256:a1b2c3d4e5f6...
```

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | Error (authentication, network, invalid input) |

## Examples

Push a directory with default compression:

```bash
blobber push ./config ghcr.io/myorg/config:v1
```

Push with zstd compression:

```bash
blobber push ./data ghcr.io/myorg/data:latest --compression zstd
```

Push to an insecure registry:

```bash
blobber push ./files localhost:5000/test:v1 --insecure
```

## Notes

- Symbolic links are preserved in the archive
- Empty directories are included
- File permissions are preserved
- Hidden files (dotfiles) are included

## See Also

- [blobber pull](/docs/reference/cli/pull) - Download from registry
- [How to Use Compression](/docs/how-to/use-compression) - Choosing compression algorithms
