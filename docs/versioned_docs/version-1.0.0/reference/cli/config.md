---
sidebar_position: 6
---

# blobber config

Manage blobber configuration.

## Synopsis

```bash
blobber config [subcommand] [flags]
```

## Description

Blobber stores configuration in a YAML file following XDG base directory conventions. The `config` command displays current settings and provides subcommands to initialize and modify the configuration file.

When run without a subcommand, displays all current configuration values.

## Configuration File

Location: `$XDG_CONFIG_HOME/blobber/config.yaml` (defaults to `~/.config/blobber/config.yaml`)

Format:

```yaml
cache:
  enabled: true
  dir: ""  # Empty means use default XDG cache path
```

## Configuration Precedence

Settings are resolved in this order (highest priority first):

1. **Command-line flags** (`--no-cache`, `--verbose`, etc.)
2. **Environment variables** (`BLOBBER_CACHE_ENABLED`, `BLOBBER_CACHE_DIR`, etc.)
3. **Config file** (`~/.config/blobber/config.yaml`)
4. **Defaults**

## Environment Variables

| Variable | Description |
|----------|-------------|
| `BLOBBER_CACHE_ENABLED` | Enable/disable caching (`true`/`false`) |
| `BLOBBER_CACHE_DIR` | Cache directory path |
| `BLOBBER_INSECURE` | Allow insecure connections |
| `BLOBBER_VERBOSE` | Enable verbose logging |

## Output

Displays all settings in YAML format:

```yaml
cache:
  dir: ""
  enabled: true
insecure: false
no-cache: false
verbose: false
```

## Examples

Show current configuration:

```bash
blobber config
```

---

## config path

Show the configuration file path.

### Synopsis

```bash
blobber config path
```

### Output

```
/home/user/.config/blobber/config.yaml
```

### Examples

```bash
blobber config path
```

---

## config init

Create a default configuration file.

### Synopsis

```bash
blobber config init
```

### Description

Creates a new configuration file with default values. Fails if the file already exists.

### Output

On success:

```
Created config file: /home/user/.config/blobber/config.yaml
```

If file exists:

```
Error: config file already exists: /home/user/.config/blobber/config.yaml
```

### Examples

```bash
blobber config init
```

---

## config set

Set a configuration value.

### Synopsis

```bash
blobber config set <key> <value>
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `key` | Yes | Configuration key (dot notation, e.g., `cache.enabled`) |
| `value` | Yes | Value to set |

### Available Keys

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `cache.enabled` | bool | `true` | Enable blob caching |
| `cache.dir` | string | `""` | Cache directory (empty = XDG default) |

### Output

```
Updated cache.enabled = false
```

### Examples

Disable caching:

```bash
blobber config set cache.enabled false
```

Use a custom cache directory:

```bash
blobber config set cache.dir /custom/cache/path
```

---

## XDG Base Directories

Blobber follows the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html):

| Purpose | Environment Variable | Default | Blobber Path |
|---------|---------------------|---------|--------------|
| Config | `XDG_CONFIG_HOME` | `~/.config` | `$XDG_CONFIG_HOME/blobber/config.yaml` |
| Cache | `XDG_CACHE_HOME` | `~/.cache` | `$XDG_CACHE_HOME/blobber/` |

## See Also

- [How to Configure Blobber](/docs/how-to/configure-blobber) - Configuration guide
- [blobber cache](/docs/reference/cli/cache) - Cache management
