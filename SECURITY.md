# Security Policy

## Reporting Security Issues

If you discover a security vulnerability in Blobber, please report it through GitHub's private vulnerability reporting feature:

1. Go to the [Security tab](../../security) of this repository
2. Click "Report a vulnerability"
3. Provide a detailed description of the issue

**Please do not report security vulnerabilities through public GitHub issues, discussions, or pull requests.**

Include as much of the following information as possible to help us understand and resolve the issue:

- Type of issue (e.g., path traversal, arbitrary file write, credential exposure)
- Full paths of source file(s) related to the issue
- Location of the affected source code (tag/branch/commit or direct URL)
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the issue and how an attacker might exploit it

## Supported Versions

We provide security updates for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 1.x     | :white_check_mark: |
| < 1.0   | :x:                |

## Response Timeline

- **Initial Response**: We aim to acknowledge receipt of your vulnerability report within 3 business days.
- **Status Update**: We will provide a more detailed response within 10 business days, including our assessment and expected timeline for a fix.
- **Resolution**: We strive to resolve critical vulnerabilities within 30 days of the initial report.

## Disclosure Policy

We follow a coordinated disclosure process:

1. Security issues are handled privately until a fix is available.
2. Once a fix is ready, we will create a security advisory and release a patched version.
3. We will publicly disclose the vulnerability after users have had reasonable time to update.
4. Credit will be given to the reporter (unless anonymity is preferred) in the security advisory.

## Security Practices

Blobber implements the following security measures:

### Artifact Signing and Verification

Blobber supports [Sigstore](https://sigstore.dev) signing and verification for artifacts you push and pull:

- **Keyless signing**: Sign artifacts using OIDC identity (GitHub, Google, Microsoft) without managing private keys
- **Key-based signing**: Sign with your own private keys (ECDSA, RSA, Ed25519)
- **Signature verification**: Verify artifact signatures before pulling, ensuring provenance and integrity
- **Identity verification**: Require signatures from specific OIDC issuers and subjects

```bash
# Push with signing
blobber push --sign ./config ghcr.io/myorg/config:v1

# Pull with verification
blobber pull --verify \
  --verify-issuer https://token.actions.githubusercontent.com \
  --verify-subject https://github.com/myorg/myrepo/.github/workflows/release.yml@refs/heads/main \
  ghcr.io/myorg/config:v1 ./output
```

See [Signing & Verification](https://blobber.meigma.dev/docs/how-to/sign-artifacts) for complete documentation.

### Release Binary Signing

- All release binaries are signed using [Cosign](https://github.com/sigstore/cosign) with keyless signing via GitHub Actions OIDC
- Each release includes SHA256 checksums for integrity verification
- Software Bill of Materials (SBOM) is generated for every release using Syft

### Code Quality

- Static analysis with [gosec](https://github.com/securego/gosec) security scanner
- Comprehensive linting with golangci-lint
- Path traversal protection via the internal `safepath` package

### Verifying Releases

You can verify the authenticity of release binaries:

```bash
# Verify binary signature
cosign verify-blob \
  --signature <binary>.sig \
  --certificate <binary>.pem \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp 'https://github.com/meigma/blobber/.github/workflows/release.yml@refs/tags/v' \
  <binary>

# Verify checksum
sha256sum -c checksums.txt
```

## Third-Party Dependencies

For vulnerabilities in third-party dependencies used by Blobber:

- If the vulnerability affects Blobber, please report it through our security reporting process above
- For vulnerabilities in upstream projects, please report directly to those projects:
  - **Go dependencies**: Use the project's security reporting mechanism or [Go vulnerability database](https://pkg.go.dev/vuln/)
  - **OCI/Container issues**: Report to the respective CNCF project

## Security-Related Configuration

When using Blobber:

- Registry credentials are read from Docker's credential store (`~/.docker/config.json`)
- Use credential helpers for enhanced security rather than storing plain credentials
- Cached data is stored locally; ensure appropriate file permissions on cache directories
- When pushing to registries, use authenticated connections and verify TLS certificates
- **Enable signature verification** in production with `--verify --verify-issuer --verify-subject`
- **Never use `--verify-unsafe`** in production; it accepts signatures from any identity
- Private signing keys should be stored securely with appropriate file permissions

## Learning More

- [Blobber Documentation](https://blobber.meigma.dev/)
- [Signing & Verification Guide](https://blobber.meigma.dev/docs/how-to/sign-artifacts)
- [About Signing](https://blobber.meigma.dev/docs/explanation/about-signing)
- [Sigstore Documentation](https://docs.sigstore.dev/)
- [ORAS Security](https://oras.land/docs/)
