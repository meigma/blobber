---
sidebar_position: 4
---

# blobber cat

Output a file from an OCI image to stdout.

## Synopsis

```bash
blobber cat <reference> <path> [flags]
```

## Description

Streams the contents of a single file from an OCI registry image to stdout. Uses eStargz format to download only the requested file, not the entire image.

## Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `reference` | Yes | OCI image reference (e.g., `ghcr.io/org/repo:tag`) |
| `path` | Yes | Path to the file within the image |

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--insecure` | bool | `false` | Allow connections without TLS |
| `-v, --verbose` | bool | `false` | Enable debug logging |

## Output

Raw file contents to stdout. No additional formatting or newlines are added.

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | Error (image not found, file not found, auth failed) |

## Examples

View a file:

```bash
blobber cat ghcr.io/myorg/config:v1 app.yaml
```

View a nested file:

```bash
blobber cat ghcr.io/myorg/config:v1 config/database.yaml
```

Save to a local file:

```bash
blobber cat ghcr.io/myorg/config:v1 app.yaml > app.yaml
```

Pipe to another command:

```bash
blobber cat ghcr.io/myorg/config:v1 config.json | jq .
```

## Notes

- Path must match exactly as shown in `blobber list`
- Binary files are output as-is
- No trailing newline is added

## See Also

- [blobber list](/docs/reference/cli/list) - List available files
- [blobber pull](/docs/reference/cli/pull) - Download all files
- [How to Extract Single Files](/docs/how-to/extract-single-file) - Practical examples
