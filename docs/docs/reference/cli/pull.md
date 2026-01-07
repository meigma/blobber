---
sidebar_position: 2
---

# blobber pull

Download an image from an OCI registry to a local directory.

## Synopsis

```bash
blobber pull <reference> <directory> [flags]
```

## Description

Downloads all files from an OCI registry image and extracts them to a local directory. By default, fails if any files already exist in the destination.

## Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `reference` | Yes | OCI image reference (e.g., `ghcr.io/org/repo:tag`) |
| `directory` | Yes | Destination directory path |

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--overwrite` | bool | `false` | Replace existing files instead of failing |
| `--insecure` | bool | `false` | Allow connections without TLS |
| `-v, --verbose` | bool | `false` | Enable debug logging |

## Output

Silent on success. Errors are printed to stderr.

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | Error (not found, auth failed, conflicts, etc.) |

## Examples

Pull to a new directory:

```bash
blobber pull ghcr.io/myorg/config:v1 ./config
```

Pull and overwrite existing files:

```bash
blobber pull --overwrite ghcr.io/myorg/config:v1 ./config
```

Pull from an insecure registry:

```bash
blobber pull --insecure localhost:5000/test:v1 ./output
```

## Conflict Detection

Before downloading, blobber checks for file conflicts. If files would be overwritten:

```
Error: 3 files already exist (use --overwrite to replace)
```

With `--overwrite`, conflicting files are removed before extraction.

## Notes

- Creates the destination directory if it doesn't exist
- Preserves file permissions from the archive
- Preserves symbolic links
- Applies extraction safety limits (see below)

## Extraction Limits

Blobber enforces safety limits to prevent resource exhaustion:

| Limit | Value |
|-------|-------|
| Maximum files | 100,000 |
| Maximum total size | 10 GB |
| Maximum file size | 1 GB |

## See Also

- [blobber push](/reference/cli/push) - Upload to registry
- [blobber cat](/reference/cli/cat) - Stream single files
