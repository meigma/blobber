---
sidebar_position: 8
---

# How to Sign Artifacts

Sign artifacts when pushing to provide cryptographic proof of provenance.

## Prerequisites

- blobber installed
- For keyless signing: Access to an OIDC provider (GitHub, Google, etc.)
- For key-based signing: A private key file (PEM format)

## Sign with Keyless Signing (Recommended)

Keyless signing uses your OIDC identity (e.g., GitHub account) and requires no key management.

### Step 1: Push with the --sign flag

```bash
blobber push --sign ./config ghcr.io/myorg/config:v1
```

### Step 2: Complete OIDC authentication

A browser window opens for authentication. Sign in with your identity provider.

After authentication, blobber:
1. Generates an ephemeral key pair
2. Obtains a certificate from Fulcio
3. Signs the manifest
4. Records the signature in Rekor
5. Stores the signature as an OCI referrer

Output:

```
sha256:abc123...
```

## Sign with a Private Key

For environments without OIDC access (air-gapped, certain CI systems).

### Step 1: Generate a key (if needed)

```bash
openssl ecparam -genkey -name prime256v1 -noout -out private.pem
```

### Step 2: Push with the key

```bash
blobber push --sign --sign-key private.pem ./config ghcr.io/myorg/config:v1
```

### Step 3: (Optional) Add to Rekor for auditability

Key-based signatures can still be recorded in Rekor:

```bash
blobber push --sign --sign-key private.pem --rekor-url https://rekor.sigstore.dev ./config ghcr.io/myorg/config:v1
```

## Sign with an Encrypted Key

If your private key is password-protected:

```bash
blobber push --sign --sign-key private.pem --sign-key-pass "your-password" ./config ghcr.io/myorg/config:v1
```

Or via environment variable:

```bash
export BLOBBER_SIGN_KEY_PASS="your-password"
blobber push --sign --sign-key private.pem ./config ghcr.io/myorg/config:v1
```

## Use Custom Sigstore Infrastructure

For private Sigstore deployments:

```bash
blobber push --sign \
  --fulcio-url https://fulcio.internal.example.com \
  --rekor-url https://rekor.internal.example.com \
  ./config ghcr.io/myorg/config:v1
```

## Sign in CI/CD (GitHub Actions)

GitHub Actions provides OIDC tokens automatically:

```yaml
jobs:
  push:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
      id-token: write  # Required for OIDC signing
    steps:
      - uses: actions/checkout@v4

      - name: Login to GHCR
        run: echo "${{ secrets.GITHUB_TOKEN }}" | docker login ghcr.io -u ${{ github.actor }} --password-stdin

      - name: Push with signing
        run: blobber push --sign ./config ghcr.io/${{ github.repository }}/config:${{ github.sha }}
```

The signature will be bound to your GitHub Actions workflow identity.

## Sign Using the Go Library

### Keyless Signing

```go
import (
    "github.com/meigma/blobber"
    "github.com/meigma/blobber/sigstore"
)

// Create a keyless signer
signer, err := sigstore.NewSigner(
    sigstore.WithEphemeralKey(),
    sigstore.WithFulcio("https://fulcio.sigstore.dev"),
    sigstore.WithRekor("https://rekor.sigstore.dev"),
)
if err != nil {
    return err
}

// Create client with signer
client, err := blobber.NewClient(
    blobber.WithSigner(signer),
)
if err != nil {
    return err
}

// Push will automatically sign
digest, err := client.Push(ctx, "ghcr.io/org/config:v1", files)
```

### Key-Based Signing

```go
import (
    "crypto/ecdsa"
    "crypto/elliptic"
    "crypto/rand"

    "github.com/meigma/blobber"
    "github.com/meigma/blobber/sigstore"
)

// Generate or load your key
key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

// Create signer with the key
signer, err := sigstore.NewSigner(
    sigstore.WithPrivateKey(key),
)
if err != nil {
    return err
}

client, err := blobber.NewClient(
    blobber.WithSigner(signer),
)
```

### From PEM File

```go
pemData, _ := os.ReadFile("private.pem")

signer, err := sigstore.NewSigner(
    sigstore.WithPrivateKeyPEM(pemData, nil), // nil password for unencrypted
    sigstore.WithRekor("https://rekor.sigstore.dev"), // optional
)
```

## Verify Your Signature Was Created

List referrers to confirm the signature exists:

```bash
# Using crane (from google/go-containerregistry)
crane manifest ghcr.io/myorg/config:v1 | jq -r '.digest' | xargs -I {} \
  crane referrers ghcr.io/myorg/config@{}
```

Or verify by pulling with verification:

```bash
blobber pull --verify --verify-unsafe ghcr.io/myorg/config:v1 ./output
```

## Troubleshooting

### "no keypair configured"

You must provide either `--sign-key` for key-based signing or use keyless (default with just `--sign`).

### "failed to get Fulcio certificate"

- Check network connectivity to `fulcio.sigstore.dev`
- Ensure OIDC authentication completed successfully
- For CI, verify `id-token: write` permission is set

### "failed to submit to Rekor"

- Check network connectivity to `rekor.sigstore.dev`
- Verify the Rekor URL is correct for private deployments

### Browser doesn't open for OIDC

In headless environments (CI, containers), Sigstore uses device flow or environment-provided OIDC tokens. Ensure your CI system supports OIDC.

## See Also

- [How to Verify Signatures](./verify-signatures.md) - Verify signed artifacts
- [About Signing](../explanation/about-signing.md) - Understanding signing concepts
- [CLI Reference: push](../reference/cli/push.md) - All push flags
