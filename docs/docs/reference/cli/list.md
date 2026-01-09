---
sidebar_position: 3
---

# blobber ls

List files in an OCI image without downloading.

## Synopsis

```bash
blobber ls <reference> [flags]
blobber list <reference> [flags]  # alias for backwards compatibility
```

## Description

Displays the files contained in an OCI registry image. Uses eStargz format to read only the table of contents, making this operation efficient even for large images.

## Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `reference` | Yes | OCI image reference (e.g., `ghcr.io/org/repo:tag`) |

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-l, --long` | bool | `false` | Long format: show path, size, and mode |
| `--insecure` | bool | `false` | Allow connections without TLS |
| `-v, --verbose` | bool | `false` | Enable debug logging |

## Output

### Short Format (default)

One file path per line:

```
app.yaml
config/database.yaml
config/server.yaml
```

### Long Format (-l)

Three columns: path, size (bytes), mode (octal):

```
app.yaml              52      0644
config/database.yaml  128     0644
config/server.yaml    96      0644
```

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | Error (not found, auth failed, invalid archive) |

## Examples

List files:

```bash
blobber ls ghcr.io/myorg/config:v1
```

List with details:

```bash
blobber ls -l ghcr.io/myorg/config:v1
```

Using the `list` alias (backwards compatibility):

```bash
blobber list ghcr.io/myorg/config:v1
```

## Notes

- Directories are not listed, only files
- Paths are relative to the image root
- Order is determined by the archive structure

## See Also

- [blobber cat](/docs/reference/cli/cat) - Stream a single file to stdout
- [blobber cp](/docs/reference/cli/cp) - Copy a single file to local path
- [About eStargz](/docs/explanation/about-estargz) - Why listing is efficient
