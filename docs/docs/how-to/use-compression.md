---
sidebar_position: 2
---

# How to Choose Compression

Select between gzip and zstd compression when pushing images.

## Prerequisites

- blobber installed
- Registry access configured

## Available Algorithms

| Algorithm | Flag Value | Best For |
|-----------|------------|----------|
| gzip | `gzip` (default) | Maximum compatibility |
| zstd | `zstd` | Faster decompression, better ratios |

## Using gzip (Default)

Push with default compression:

```bash
blobber push ./data ghcr.io/myorg/data:v1
```

Or explicitly:

```bash
blobber push ./data ghcr.io/myorg/data:v1 --compression gzip
```

## Using zstd

Push with zstd compression:

```bash
blobber push ./data ghcr.io/myorg/data:v1 --compression zstd
```

## When to Use Each

### Use gzip when:

- You need maximum registry compatibility
- Other tools will read the image
- You're unsure which to choose

### Use zstd when:

- You control the entire workflow
- Decompression speed matters
- You're storing large files

## Verification

The compression algorithm is stored in the image. To verify:

```bash
blobber list ghcr.io/myorg/data:v1
```

Both algorithms produce valid eStargz images that work with `list`, `cat`, and `pull`.

## Notes

- You cannot change compression of an existing image
- Push a new tag if you need different compression
- Compression choice doesn't affect file listing or selective retrieval

## See Also

- [CLI Reference: push](/reference/cli/push)
- [About eStargz](/explanation/about-estargz) - How compression works internally
