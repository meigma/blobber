---
sidebar_position: 4
---

# How to Extract a Single File

Stream individual files from a remote image without downloading everything.

## Prerequisites

- blobber installed
- Registry access configured

## View File Contents

Stream a file to your terminal:

```bash
blobber cat ghcr.io/myorg/config:v1 app.yaml
```

## Save to Local File

Use `cp` to copy directly to a file:

```bash
blobber cp ghcr.io/myorg/config:v1 app.yaml ./local-app.yaml
```

Or redirect `cat` output:

```bash
blobber cat ghcr.io/myorg/config:v1 app.yaml > local-app.yaml
```

## Extract Nested Files

Use the full path as shown in `blobber ls`:

```bash
blobber cat ghcr.io/myorg/config:v1 config/database.yaml
```

## Pipe to Other Commands

### Parse JSON with jq

```bash
blobber cat ghcr.io/myorg/config:v1 settings.json | jq '.database'
```

### Parse YAML with yq

```bash
blobber cat ghcr.io/myorg/config:v1 app.yaml | yq '.version'
```

### Diff against local file

```bash
diff <(blobber cat ghcr.io/myorg/config:v1 app.yaml) ./local-app.yaml
```

### Search with grep

```bash
blobber cat ghcr.io/myorg/config:v1 server.yaml | grep port
```

## Extract Multiple Files

For multiple specific files, run multiple commands:

```bash
blobber cp ghcr.io/myorg/config:v1 app.yaml ./app.yaml
blobber cp ghcr.io/myorg/config:v1 database.yaml ./database.yaml
```

For many files, consider using `blobber pull` instead.

## Binary Files

Binary files work the same way:

```bash
blobber cp ghcr.io/myorg/assets:v1 logo.png ./logo.png
```

## Find the Correct Path

If you're unsure of the exact path:

```bash
blobber ls ghcr.io/myorg/config:v1 | grep database
```

Then use the exact path shown.

## Why This Is Efficient

Blobber uses eStargz format with HTTP range requests. When you `cat` a file, only that file's bytes are downloaded, not the entire image.

For a 1GB image, extracting a 1KB config file downloads approximately 1KB.

## See Also

- [CLI Reference: cat](/docs/reference/cli/cat)
- [CLI Reference: cp](/docs/reference/cli/cp)
- [How to List Remote Files](/docs/how-to/list-remote-files)
- [About eStargz](/docs/explanation/about-estargz)
