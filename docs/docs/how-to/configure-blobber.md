---
sidebar_position: 7
---

# How to Configure Blobber

Customize blobber behavior through configuration files, environment variables, or command-line flags.

## Prerequisites

- blobber installed

## Create a Configuration File

Initialize the default configuration:

```bash
blobber config init
```

Output:

```
Created config file: /home/user/.config/blobber/config.yaml
```

## View Current Configuration

Display all settings and their effective values:

```bash
blobber config
```

Output:

```yaml
cache:
  dir: ""
  enabled: true
insecure: false
no-cache: false
verbose: false
```

## Find the Config File Location

```bash
blobber config path
```

Output:

```
/home/user/.config/blobber/config.yaml
```

## Disable Caching Permanently

Edit the config file or use the set command:

```bash
blobber config set cache.enabled false
```

Verify:

```bash
blobber config
```

## Use a Custom Cache Directory

```bash
blobber config set cache.dir /path/to/custom/cache
```

## Bypass Cache for a Single Command

Use `--no-cache` to skip caching without changing configuration:

```bash
blobber --no-cache pull ghcr.io/myorg/config:v1 ./output
```

## Use Environment Variables

Set environment variables for session-wide configuration:

```bash
export BLOBBER_CACHE_ENABLED=false
export BLOBBER_CACHE_DIR=/tmp/blobber-cache
```

Add to your shell profile (`~/.bashrc`, `~/.zshrc`) for persistence.

## Override Config with Flags

Command-line flags take highest priority:

```bash
# Uses --no-cache even if cache.enabled=true in config
blobber --no-cache pull ghcr.io/myorg/config:v1 ./output
```

## Configuration Precedence

Settings resolve in this order (highest priority first):

1. Command-line flags
2. Environment variables
3. Config file
4. Defaults

## Edit the Config File Directly

The config file is plain YAML:

```bash
cat ~/.config/blobber/config.yaml
```

```yaml
cache:
  enabled: true
  dir: ""
```

Edit with any text editor:

```bash
vim ~/.config/blobber/config.yaml
```

## Reset to Defaults

Remove the config file to restore defaults:

```bash
rm ~/.config/blobber/config.yaml
```

## Use XDG Paths on Different Systems

Blobber respects XDG environment variables:

```bash
# Custom config location
export XDG_CONFIG_HOME=/custom/config
blobber config path  # /custom/config/blobber/config.yaml

# Custom cache location
export XDG_CACHE_HOME=/custom/cache
blobber cache info   # Shows /custom/cache/blobber
```

## See Also

- [CLI Reference: config](/reference/cli/config)
- [How to Manage Cache](/how-to/manage-cache)
