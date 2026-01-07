---
sidebar_position: 5
---

# blobber cache

Manage the local blob cache.

## Synopsis

```bash
blobber cache <subcommand> [flags]
```

## Description

Blobber caches downloaded blobs locally to speed up repeated operations. The `cache` command provides subcommands to inspect and manage this cache.

## Global Cache Flags

These flags apply to all cache subcommands:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dir` | string | `~/.blobber/cache` | Cache directory path |

---

## cache info

Show cache statistics.

### Synopsis

```bash
blobber cache info [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-l, --long` | bool | `false` | Show detailed entry information |

### Output

Short format:

```
Cache: /Users/you/.blobber/cache
Size:  150 MB (157286400 bytes)
Entries: 3
```

Long format (`-l`):

```
Cache: /Users/you/.blobber/cache
Size:  150 MB (157286400 bytes)
Entries: 3

DIGEST                                                            SIZE     LAST ACCESSED   COMPLETE
sha256:a1b2c3d4e5f6789...                                         50 MB    1 hour ago      yes
sha256:b2c3d4e5f6789a0...                                         50 MB    30 min ago      yes
sha256:c3d4e5f6789a0b1...                                         50 MB    5 min ago       yes
```

### Examples

```bash
blobber cache info
blobber cache info --long
blobber cache info --dir /custom/cache
```

---

## cache clear

Remove all cached blobs.

### Synopsis

```bash
blobber cache clear [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-y, --yes` | bool | `false` | Skip confirmation prompt |

### Output

Without `--yes`, prompts for confirmation:

```
This will remove 3 entries (150 MB) from the cache. Continue? [y/N]
```

On success:

```
Cleared 3 entries (150 MB)
```

If already empty:

```
Cache is already empty
```

### Examples

```bash
blobber cache clear
blobber cache clear --yes
```

---

## cache prune

Remove old or excess cache entries.

### Synopsis

```bash
blobber cache prune [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--max-size` | string | — | Maximum cache size (e.g., `1GB`, `500MB`) |
| `--max-age` | string | — | Maximum entry age (e.g., `24h`, `7d`) |

At least one of `--max-size` or `--max-age` is required.

### Size Format

- `500MB`, `1GB`, `100KB`
- Case-insensitive

### Age Format

- `s` - seconds
- `m` - minutes
- `h` - hours
- `d` - days

Examples: `30m`, `24h`, `7d`, `1h30m`

### Output

```
Removed 2 entries (100 MB)
Remaining: 1 entry (50 MB)
```

If nothing to prune:

```
No entries to prune
```

### Pruning Order

1. Entries exceeding `--max-age` are removed first
2. Remaining entries are evicted LRU (least recently used) until under `--max-size`

### Examples

Remove entries older than 7 days:

```bash
blobber cache prune --max-age 7d
```

Keep cache under 1GB:

```bash
blobber cache prune --max-size 1GB
```

Combine both:

```bash
blobber cache prune --max-age 24h --max-size 500MB
```

---

## Cache Location

Default: `~/.blobber/cache`

Structure:

```
~/.blobber/cache/
├── blobs/
│   └── sha256/
│       └── <digest>          # Cached blob data
└── entries/
    └── sha256/
        └── <digest>.json     # Entry metadata
```

## See Also

- [How to Manage Cache](/how-to/manage-cache) - Cache management strategies
