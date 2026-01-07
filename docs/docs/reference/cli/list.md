---
sidebar_position: 3
---

# blobber list

List files in an OCI image without downloading.

## Synopsis

```bash
blobber list <reference> [flags]
blobber ls <reference> [flags]
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
blobber list ghcr.io/myorg/config:v1
```

List with details:

```bash
blobber list -l ghcr.io/myorg/config:v1
```

Using the `ls` alias:

```bash
blobber ls ghcr.io/myorg/config:v1
```

## Notes

- Directories are not listed, only files
- Paths are relative to the image root
- Order is determined by the archive structure

## See Also

- [blobber cat](/reference/cli/cat) - Stream a single file
- [About eStargz](/explanation/about-estargz) - Why listing is efficient
