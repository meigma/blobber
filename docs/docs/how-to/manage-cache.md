---
sidebar_position: 5
---

# How to Manage the Blob Cache

Monitor and control blobber's local cache for optimal disk usage.

## Prerequisites

- blobber installed

## Check Cache Status

View current cache statistics:

```bash
blobber cache info
```

Output:

```
Cache: /home/user/.cache/blobber
Size:  150 MB (157286400 bytes)
Entries: 3
```

## View Detailed Cache Info

See individual entries:

```bash
blobber cache info --long
```

Output:

```
Cache: /home/user/.cache/blobber
Size:  150 MB (157286400 bytes)
Entries: 3

DIGEST                                                            SIZE     LAST ACCESSED   COMPLETE
sha256:a1b2c3d4e5f6789...                                         50 MB    1 hour ago      yes
sha256:b2c3d4e5f6789a0...                                         50 MB    30 min ago      yes
sha256:c3d4e5f6789a0b1...                                         50 MB    5 min ago       yes
```

## Clear the Entire Cache

Remove all cached blobs:

```bash
blobber cache clear
```

Confirm when prompted, or skip confirmation:

```bash
blobber cache clear --yes
```

## Prune Old Entries

Remove entries not accessed in 7 days:

```bash
blobber cache prune --max-age 7d
```

## Limit Cache Size

Keep cache under 1GB, removing least recently used entries:

```bash
blobber cache prune --max-size 1GB
```

## Combine Pruning Strategies

Remove old entries AND enforce size limit:

```bash
blobber cache prune --max-age 7d --max-size 1GB
```

Order: age-based removal happens first, then LRU eviction.

## Use a Custom Cache Directory

All cache commands accept `--dir`:

```bash
blobber cache info --dir /custom/cache
blobber cache prune --dir /custom/cache --max-size 500MB
```

## Automate Cache Maintenance

Add to cron for daily cleanup:

```bash
0 3 * * * blobber cache prune --max-age 7d --max-size 2GB
```

## Cache Location

Default: `~/.cache/blobber` (following XDG Base Directory Specification)

To change permanently:

```bash
# Option 1: Set in config file
blobber config set cache.dir /path/to/cache

# Option 2: Environment variable
export BLOBBER_CACHE_DIR=/path/to/cache

# Option 3: XDG override (affects all XDG-compliant apps)
export XDG_CACHE_HOME=/custom/cache
```

## Bypass Cache Temporarily

Skip caching for a single operation without changing settings:

```bash
blobber --no-cache pull ghcr.io/myorg/config:v1 ./output
```

## Disable Caching Permanently

Turn off caching in the config file:

```bash
blobber config set cache.enabled false
```

## When to Clear vs Prune

| Scenario | Command |
|----------|---------|
| Free all disk space | `cache clear` |
| Regular maintenance | `cache prune --max-age 7d` |
| Disk pressure | `cache prune --max-size 500MB` |
| Troubleshooting | `cache clear` then retry |
| Skip cache once | `--no-cache` flag |

## See Also

- [CLI Reference: cache](/reference/cli/cache)
- [How to Configure Blobber](/how-to/configure-blobber)
