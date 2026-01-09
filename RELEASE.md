# Release Process

This project uses [release-please](https://github.com/googleapis/release-please) for automated releases and [GoReleaser](https://goreleaser.com/) for building and publishing binaries.

## How It Works

### 1. Commit Changes

All commits must follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

| Type | Version Bump | Example |
|------|--------------|---------|
| `fix:` | Patch (0.0.x) | `fix: handle nil pointer in registry client` |
| `feat:` | Minor (0.x.0) | `feat: add zstd compression support` |
| `feat!:` or `BREAKING CHANGE:` | Major (x.0.0) | `feat!: redesign Client API` |

Other types (`docs`, `chore`, `test`, `ci`, `refactor`, `style`) do not trigger releases but are tracked in the changelog.

### 2. Release PR

When commits are pushed to `master`, release-please automatically:

1. Analyzes commits since the last release
2. Creates or updates a **Release PR** with:
   - Version bump based on commit types
   - Updated `CHANGELOG.md`
   - Updated `.release-please-manifest.json`

The Release PR stays open and accumulates changes until you're ready to release.

### 3. Create Release

Merge the Release PR to trigger:

1. **release-please** creates a GitHub release with changelog notes and a `v*` tag
2. **GoReleaser** (triggered by the tag) builds and uploads:
   - Binaries for Linux, macOS, and Windows (amd64/arm64)
   - Checksums signed with Cosign
   - SBOMs in SPDX format
   - Homebrew formula update

## Release Artifacts

Each release includes:

| Artifact | Description |
|----------|-------------|
| `blobber_<version>_<os>_<arch>.tar.gz` | Binary archive |
| `checksums.txt` | SHA256 checksums |
| `checksums.txt.bundle` | Cosign signature bundle |
| `*.sbom.spdx.json` | Software Bill of Materials |
| `*.sbom.spdx.json.bundle` | SBOM signature bundle |

## Verifying Releases

Verify checksum signature:

```bash
cosign verify-blob \
  --bundle checksums.txt.bundle \
  --certificate-identity-regexp="https://github.com/meigma/blobber" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  checksums.txt
```

Verify artifact integrity:

```bash
sha256sum -c checksums.txt --ignore-missing
```

## Manual Version Override

To force a specific version, add to any commit message:

```
feat: add new feature

Release-As: 2.0.0
```

## Configuration Files

| File | Purpose |
|------|---------|
| `release-please-config.json` | Release-please settings |
| `.release-please-manifest.json` | Current version tracking |
| `.goreleaser.yaml` | GoReleaser build configuration |
| `.github/workflows/release-please.yml` | Release PR automation |
| `.github/workflows/release.yml` | Binary build and publish |
