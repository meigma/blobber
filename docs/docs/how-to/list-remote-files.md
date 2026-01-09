---
sidebar_position: 3
---

# How to List Files Without Downloading

Inspect the contents of a remote image without downloading the full blob.

## Prerequisites

- blobber installed
- Registry access configured

## Basic Listing

List files in an image:

```bash
blobber ls ghcr.io/myorg/config:v1
```

Output:

```
app.yaml
config/database.yaml
config/server.yaml
```

## Detailed Listing

Add `-l` for file sizes and permissions:

```bash
blobber ls -l ghcr.io/myorg/config:v1
```

Output:

```
app.yaml              52      0644
config/database.yaml  128     0644
config/server.yaml    96      0644
```

Columns: path, size (bytes), mode (octal).

## Using the Alias

`list` is an alias for `ls` (for backwards compatibility):

```bash
blobber list ghcr.io/myorg/config:v1
blobber list -l ghcr.io/myorg/config:v1
```

## Scripting Examples

### Check if a file exists

```bash
if blobber ls ghcr.io/myorg/config:v1 | grep -q "app.yaml"; then
  echo "File exists"
fi
```

### Count files

```bash
blobber ls ghcr.io/myorg/config:v1 | wc -l
```

### Get total size

```bash
blobber ls -l ghcr.io/myorg/config:v1 | awk '{sum += $2} END {print sum " bytes"}'
```

### Find large files

```bash
blobber ls -l ghcr.io/myorg/data:v1 | awk '$2 > 1000000 {print}'
```

## Why This Is Efficient

Blobber uses eStargz format, which stores a table of contents at the end of the archive. Listing files only downloads this index, not the file contents.

For a 1GB image, listing might download only a few KB.

## See Also

- [CLI Reference: ls](../reference/cli/list.md)
- [How to Extract Single Files](./extract-single-file.md)
- [About eStargz](../explanation/about-estargz.md)
