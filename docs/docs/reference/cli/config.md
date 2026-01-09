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
  verify: false

sign:
  enabled: false
  key: ""  # Path to private key (for key-based signing)
  password: ""  # Private key password (if encrypted)
  fulcio: https://fulcio.sigstore.dev
  rekor: https://rekor.sigstore.dev

verify:
  enabled: false
  issuer: ""  # Required OIDC issuer (e.g., https://accounts.google.com)
  subject: ""  # Required signer identity (e.g., user@example.com)
  unsafe: false  # Accept any valid signature (development only)
  trusted-root: ""  # Path to custom trusted root JSON
```

## Configuration Precedence

Settings are resolved in this order (highest priority first):

1. **Command-line flags** (`--no-cache`, `--verbose`, etc.)
2. **Environment variables** (`BLOBBER_CACHE_ENABLED`, `BLOBBER_CACHE_DIR`, etc.)
3. **Config file** (`~/.config/blobber/config.yaml`)
4. **Defaults**

## Environment Variables

### General

| Variable | Description |
|----------|-------------|
| `BLOBBER_INSECURE` | Allow insecure connections |
| `BLOBBER_VERBOSE` | Enable verbose logging |

### Cache

| Variable | Description |
|----------|-------------|
| `BLOBBER_CACHE_ENABLED` | Enable/disable caching (`true`/`false`) |
| `BLOBBER_CACHE_DIR` | Cache directory path |
| `BLOBBER_CACHE_VERIFY` | Re-verify cached blobs on read (`true`/`false`) |

### Signing

| Variable | Description |
|----------|-------------|
| `BLOBBER_SIGN_ENABLED` | Enable signing on push |
| `BLOBBER_SIGN_KEY` | Path to private key for signing |
| `BLOBBER_SIGN_PASSWORD` | Password for encrypted private key |
| `BLOBBER_SIGN_FULCIO` | Fulcio CA URL for keyless signing |
| `BLOBBER_SIGN_REKOR` | Rekor transparency log URL |

### Verification

| Variable | Description |
|----------|-------------|
| `BLOBBER_VERIFY_ENABLED` | Enable signature verification on pull |
| `BLOBBER_VERIFY_ISSUER` | Required OIDC issuer URL |
| `BLOBBER_VERIFY_SUBJECT` | Required signer identity |
| `BLOBBER_VERIFY_UNSAFE` | Accept any valid signature (unsafe) |
| `BLOBBER_VERIFY_TRUSTED_ROOT` | Path to custom trusted root JSON |

## Output

Displays all settings in YAML format:

```yaml
cache:
  dir: ""
  enabled: true
  verify: false
sign:
  enabled: false
  fulcio: https://fulcio.sigstore.dev
  rekor: https://rekor.sigstore.dev
verify:
  enabled: false
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

#### Cache

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `cache.enabled` | bool | `true` | Enable blob caching |
| `cache.dir` | string | `""` | Cache directory (empty = XDG default) |
| `cache.verify` | bool | `false` | Re-verify cached blobs on read (slower) |

#### Signing

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `sign.enabled` | bool | `false` | Enable signing on push |
| `sign.key` | string | `""` | Path to private key for signing |
| `sign.password` | string | `""` | Password for encrypted private key |
| `sign.fulcio` | string | `https://fulcio.sigstore.dev` | Fulcio CA URL |
| `sign.rekor` | string | `https://rekor.sigstore.dev` | Rekor transparency log URL |

#### Verification

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `verify.enabled` | bool | `false` | Enable signature verification on pull |
| `verify.issuer` | string | `""` | Required OIDC issuer URL |
| `verify.subject` | string | `""` | Required signer identity |
| `verify.unsafe` | bool | `false` | Accept any valid signature |
| `verify.trusted-root` | string | `""` | Path to custom trusted root JSON |

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

Enable cache verification on read:

```bash
blobber config set cache.verify true
```

Enable signing with default Sigstore:

```bash
blobber config set sign.enabled true
```

Configure key-based signing:

```bash
blobber config set sign.key /path/to/private-key.pem
```

Configure verification with identity requirements:

```bash
blobber config set verify.enabled true
blobber config set verify.issuer https://accounts.google.com
blobber config set verify.subject developer@company.com
```

---

## XDG Base Directories

Blobber follows the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html):

| Purpose | Environment Variable | Default | Blobber Path |
|---------|---------------------|---------|--------------|
| Config | `XDG_CONFIG_HOME` | `~/.config` | `$XDG_CONFIG_HOME/blobber/config.yaml` |
| Cache | `XDG_CACHE_HOME` | `~/.cache` | `$XDG_CACHE_HOME/blobber/` |

## See Also

- [How to Configure Blobber](../../how-to/configure-blobber.md) - Configuration guide
- [blobber cache](./cache.md) - Cache management
