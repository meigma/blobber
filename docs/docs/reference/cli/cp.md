---
sidebar_position: 5
---

# blobber cp

Copy a file from an OCI image to a local path.

## Synopsis

```bash
blobber cp <reference> <path> <destination> [flags]
```

## Description

Copies a single file from an OCI registry image to a local path. Uses eStargz format to download only the requested file, not the entire image. Parent directories are created automatically if they don't exist.

## Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `reference` | Yes | OCI image reference (e.g., `ghcr.io/org/repo:tag`) |
| `path` | Yes | Path to the file within the image |
| `destination` | Yes | Local file path to write to |

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--insecure` | bool | `false` | Allow connections without TLS |
| `-v, --verbose` | bool | `false` | Enable debug logging |

## Output

No output on success. Errors are printed to stderr.

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | Error (image not found, file not found, auth failed, write failed) |

## Examples

Copy a file to the current directory:

```bash
blobber cp ghcr.io/myorg/config:v1 app.yaml ./app.yaml
```

Copy a nested file:

```bash
blobber cp ghcr.io/myorg/config:v1 config/database.yaml ./database.yaml
```

Copy to a new directory (created automatically):

```bash
blobber cp ghcr.io/myorg/config:v1 app.yaml ./output/configs/app.yaml
```

## Notes

- Path must match exactly as shown in `blobber ls`
- Existing files are overwritten without warning
- Parent directories are created with mode 0755
- Binary files are copied as-is

## See Also

- [blobber cat](/docs/reference/cli/cat) - Stream a file to stdout
- [blobber ls](/docs/reference/cli/list) - List available files
- [blobber pull](/docs/reference/cli/pull) - Download all files
- [How to Extract Single Files](/docs/how-to/extract-single-file) - Practical examples
