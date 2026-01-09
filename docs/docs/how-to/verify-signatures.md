---
sidebar_position: 9
---

# How to Verify Artifact Signatures

Verify signatures before using artifacts to ensure they come from trusted sources and haven't been tampered with.

## Prerequisites

- blobber installed
- A signed artifact in the registry
- Knowledge of the expected signer identity (for production use)

## Verify with Identity Requirements (Recommended)

For production, always specify the expected signer identity:

```bash
blobber pull --verify \
  --verify-issuer https://accounts.google.com \
  --verify-subject developer@company.com \
  ghcr.io/myorg/config:v1 ./output
```

This verifies:
1. A valid signature exists
2. The signer authenticated via the specified OIDC issuer
3. The signer's identity matches the expected subject

## Verify GitHub Actions Signatures

For artifacts signed in GitHub Actions:

```bash
blobber pull --verify \
  --verify-issuer https://token.actions.githubusercontent.com \
  --verify-subject https://github.com/myorg/myrepo/.github/workflows/release.yml@refs/heads/main \
  ghcr.io/myorg/config:v1 ./output
```

The subject format for GitHub Actions is the workflow path with ref.

## Verify Without Identity Check (Development Only)

For development and testing, you can accept any valid signature:

```bash
blobber pull --verify --verify-unsafe ghcr.io/myorg/config:v1 ./output
```

**Warning:** This accepts signatures from *any* identity. Never use in production.

## Use a Custom Trusted Root

For private Sigstore deployments or custom PKI:

```bash
blobber pull --verify \
  --trusted-root ./custom-trusted-root.json \
  --verify-issuer https://auth.internal.example.com \
  --verify-subject ci@internal.example.com \
  ghcr.io/myorg/config:v1 ./output
```

The trusted root JSON contains the CA certificates and transparency log keys for your Sigstore instance.

## Verify with OpenImage (Library)

Verification also works with `OpenImage` for listing and streaming:

```bash
blobber list --verify \
  --verify-issuer https://accounts.google.com \
  --verify-subject developer@company.com \
  ghcr.io/myorg/config:v1
```

```bash
blobber cat --verify \
  --verify-issuer https://accounts.google.com \
  --verify-subject developer@company.com \
  ghcr.io/myorg/config:v1 app.yaml
```

## Verify Using the Go Library

### Basic Verification

```go
import (
    "github.com/meigma/blobber"
    "github.com/meigma/blobber/sigstore"
)

// Create verifier with identity requirements
verifier, err := sigstore.NewVerifier(
    sigstore.WithIdentity(
        "https://accounts.google.com",  // issuer
        "developer@company.com",        // subject
    ),
)
if err != nil {
    return err
}

// Create client with verifier
client, err := blobber.NewClient(
    blobber.WithVerifier(verifier),
)
if err != nil {
    return err
}

// Pull will verify before extracting
err = client.Pull(ctx, "ghcr.io/org/config:v1", "./output")
if errors.Is(err, blobber.ErrNoSignature) {
    // No signature found
}
if errors.Is(err, blobber.ErrSignatureInvalid) {
    // Signature verification failed
}
```

### Verification with Custom Trusted Root

```go
verifier, err := sigstore.NewVerifier(
    sigstore.WithTrustedRootFile("./custom-root.json"),
    sigstore.WithIdentity("https://auth.internal", "ci@internal"),
)
```

### Development Verification (No Identity Check)

```go
// Warning: accepts any valid signature
verifier, err := sigstore.NewVerifier()
// No WithIdentity - logs a warning but allows any signer
```

## Handle Verification Failures

### No Signature Found

```
Error: no signature found (use --verify with signed artifacts)
```

The artifact was not signed. Either:
- Push again with `--sign`
- Remove `--verify` if signing is optional

### Signature Invalid

```
Error: signature verification failed (artifact may be tampered)
```

Possible causes:
- Wrong `--verify-issuer` or `--verify-subject`
- Artifact was tampered with
- Signature was created with different Sigstore instance
- Trusted root doesn't match the signing CA

### Wrong Identity

If verification fails due to identity mismatch:

1. Check the actual signer identity (requires inspecting the signature)
2. Update `--verify-issuer` and `--verify-subject` to match
3. Or re-sign with the expected identity

## Verify in CI/CD (GitHub Actions)

```yaml
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Pull with verification
        run: |
          blobber pull --verify \
            --verify-issuer https://token.actions.githubusercontent.com \
            --verify-subject https://github.com/myorg/myrepo/.github/workflows/release.yml@refs/heads/main \
            ghcr.io/myorg/config:${{ github.sha }} ./config

      - name: Deploy
        run: ./deploy.sh ./config
```

## Skip Verification for Specific Pulls

If a client is configured with a verifier but you need to skip verification:

**CLI:** Currently requires creating a separate client without `--verify`.

**Library:** Create a separate client without the verifier:

```go
// Client with verification
verifiedClient, _ := blobber.NewClient(blobber.WithVerifier(verifier))

// Client without verification
unverifiedClient, _ := blobber.NewClient()
```

## Verification and Caching

When verification is enabled:

1. Signature is checked **before** downloading the blob
2. If verification fails, no content is downloaded
3. Successful verification returns a digest-pinned reference
4. Subsequent operations use the pinned digest

This ensures you can't accidentally use an unverified artifact.

## Multiple Signers

If an artifact might be signed by different identities:

**CLI:** Run multiple verification attempts:

```bash
blobber pull --verify --verify-issuer https://issuer1 --verify-subject signer1@example.com ... || \
blobber pull --verify --verify-issuer https://issuer2 --verify-subject signer2@example.com ...
```

**Library:** Currently, configure one identity per verifier. For multiple allowed signers, implement custom verification logic.

## Troubleshooting

### "fetch trusted root" errors

- Check network connectivity to `tuf-repo-cdn.sigstore.dev`
- For air-gapped environments, provide a local `--trusted-root` file

### Verification succeeds but pulls wrong content

- Ensure you're verifying the tag you intend to use
- Tags are mutable; consider using digest references after verification

### Slow verification

- First verification fetches the trusted root (~100KB)
- Subsequent verifications use cached root
- Signature bundles are typically small (<10KB)

## See Also

- [How to Sign Artifacts](/docs/how-to/sign-artifacts) - Sign your artifacts
- [About Signing](/docs/explanation/about-signing) - Understanding signing concepts
- [CLI Reference: pull](/docs/reference/cli/pull) - All pull flags
- [Errors Reference](/docs/reference/library/errors) - Error handling
