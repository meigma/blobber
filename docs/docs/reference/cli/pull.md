---
sidebar_position: 2
---

# blobber pull

Download an image from an OCI registry to a local directory.

## Synopsis

```bash
blobber pull <reference> <directory> [flags]
```

## Description

Downloads all files from an OCI registry image and extracts them to a local directory. By default, fails if any files already exist in the destination.

## Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `reference` | Yes | OCI image reference (e.g., `ghcr.io/org/repo:tag`) |
| `directory` | Yes | Destination directory path |

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--overwrite` | bool | `false` | Replace existing files instead of failing |
| `--insecure` | bool | `false` | Allow connections without TLS |
| `-v, --verbose` | bool | `false` | Enable debug logging |

### Verification Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--verify` | bool | `false` | Verify artifact signature before pulling |
| `--verify-issuer` | string | | Required OIDC issuer URL (e.g., `https://accounts.google.com`) |
| `--verify-subject` | string | | Required signer identity (e.g., `user@example.com`) |
| `--verify-unsafe` | bool | `false` | Accept any valid signature (unsafe, for development only) |
| `--trusted-root` | string | | Path to custom trusted root JSON file |

## Output

Silent on success. Errors are printed to stderr.

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | Error (not found, auth failed, conflicts, etc.) |

## Examples

Pull to a new directory:

```bash
blobber pull ghcr.io/myorg/config:v1 ./config
```

Pull and overwrite existing files:

```bash
blobber pull --overwrite ghcr.io/myorg/config:v1 ./config
```

Pull from an insecure registry:

```bash
blobber pull --insecure localhost:5000/test:v1 ./output
```

Pull with signature verification (production):

```bash
blobber pull --verify \
  --verify-issuer https://accounts.google.com \
  --verify-subject developer@company.com \
  ghcr.io/myorg/config:v1 ./config
```

Pull with verification for GitHub Actions signatures:

```bash
blobber pull --verify \
  --verify-issuer https://token.actions.githubusercontent.com \
  --verify-subject https://github.com/org/repo/.github/workflows/release.yml@refs/heads/main \
  ghcr.io/myorg/config:v1 ./config
```

Pull with verification using custom trusted root:

```bash
blobber pull --verify \
  --trusted-root ./custom-root.json \
  --verify-issuer https://auth.internal \
  --verify-subject ci@internal \
  ghcr.io/myorg/config:v1 ./config
```

Pull with verification accepting any signer (development only):

```bash
blobber pull --verify --verify-unsafe ghcr.io/myorg/config:v1 ./config
```

## Conflict Detection

Before downloading, blobber checks for file conflicts. If files would be overwritten:

```
Error: 3 files already exist (use --overwrite to replace)
```

With `--overwrite`, conflicting files are removed before extraction.

## Notes

- Creates the destination directory if it doesn't exist
- Preserves file permissions from the archive
- Preserves symbolic links
- Applies extraction safety limits (see below)

## Extraction Limits

Blobber enforces safety limits to prevent resource exhaustion:

| Limit | Value |
|-------|-------|
| Maximum files | 100,000 |
| Maximum total size | 10 GB |
| Maximum file size | 1 GB |

## Signature Verification

When `--verify` is specified:

1. Signature is checked **before** downloading the blob
2. If verification fails, no content is downloaded
3. Returns `ErrNoSignature` if no signature exists
4. Returns `ErrSignatureInvalid` if verification fails

**Note:** `--verify` requires either `--verify-issuer` + `--verify-subject` or `--verify-unsafe`.

## Configuration

Verification options can be configured via config file or environment variables:

**Config file** (`~/.config/blobber/config.yaml`):

```yaml
verify:
  enabled: true
  issuer: https://accounts.google.com
  subject: developer@company.com
  trusted-root: /path/to/trusted-root.json
```

**Environment variables**:

| Variable | Description |
|----------|-------------|
| `BLOBBER_VERIFY_ENABLED` | Enable signature verification |
| `BLOBBER_VERIFY_ISSUER` | Required OIDC issuer URL |
| `BLOBBER_VERIFY_SUBJECT` | Required signer identity |
| `BLOBBER_VERIFY_UNSAFE` | Accept any valid signature |
| `BLOBBER_VERIFY_TRUSTED_ROOT` | Path to custom trusted root |

See [How to Configure Blobber](../../how-to/configure-blobber.md) for details.

## See Also

- [blobber push](./push.md) - Upload to registry
- [blobber cat](./cat.md) - Stream single files
- [How to Verify Signatures](../../how-to/verify-signatures.md) - Verification guide
- [About Signing](../../explanation/about-signing.md) - Understanding Sigstore signing
