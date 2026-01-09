---
sidebar_position: 5
---

# Sigstore Package

Package `sigstore` implements `blobber.Signer` and `blobber.Verifier` using sigstore-go.

## Separate Module

The sigstore package is a separate Go module to isolate dependencies:

```go
import "github.com/meigma/blobber/sigstore"
```

Users who don't need signing can import `github.com/meigma/blobber` without pulling in sigstore-go and its transitive dependencies (protobuf, gRPC, OIDC libraries).

---

## Signer

### NewSigner

```go
func NewSigner(opts ...SignerOption) (*Signer, error)
```

Creates a sigstore-based signer.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `opts` | `...SignerOption` | Configuration options |

**Returns:**

| Type | Description |
|------|-------------|
| `*Signer` | The configured signer |
| `error` | Error if configuration fails |

**Example (keyless):**

```go
signer, err := sigstore.NewSigner(
    sigstore.WithEphemeralKey(),
    sigstore.WithFulcio("https://fulcio.sigstore.dev"),
    sigstore.WithRekor("https://rekor.sigstore.dev"),
)
```

**Example (key-based):**

```go
signer, err := sigstore.NewSigner(
    sigstore.WithPrivateKeyPEM(pemData, nil),
)
```

---

## Signer Options

### WithEphemeralKey

```go
func WithEphemeralKey() SignerOption
```

Generates a new ephemeral key pair for signing. Use with `WithFulcio` for keyless signing.

**Example:**

```go
signer, err := sigstore.NewSigner(
    sigstore.WithEphemeralKey(),
    sigstore.WithFulcio("https://fulcio.sigstore.dev"),
)
```

---

### WithPrivateKey

```go
func WithPrivateKey(key crypto.Signer) SignerOption
```

Uses an existing `crypto.Signer` for signing.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `key` | `crypto.Signer` | Private key (ECDSA, RSA, or Ed25519) |

**Supported key types:**

| Type | Minimum Size |
|------|-------------|
| ECDSA | P-256, P-384, P-521 |
| RSA | 2048 bits |
| Ed25519 | Standard |

**Example:**

```go
key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
signer, err := sigstore.NewSigner(
    sigstore.WithPrivateKey(key),
)
```

---

### WithPrivateKeyPEM

```go
func WithPrivateKeyPEM(pemData, password []byte) SignerOption
```

Parses a PEM-encoded private key and uses it for signing.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `pemData` | `[]byte` | PEM-encoded private key |
| `password` | `[]byte` | Password for encrypted keys (nil for unencrypted) |

**Supported PEM formats:**

| Block Type | Format |
|------------|--------|
| `PRIVATE KEY` | PKCS#8 |
| `RSA PRIVATE KEY` | PKCS#1 |
| `EC PRIVATE KEY` | SEC1 |

**Example:**

```go
pemData, _ := os.ReadFile("private.pem")
signer, err := sigstore.NewSigner(
    sigstore.WithPrivateKeyPEM(pemData, nil),
)
```

With encrypted key:

```go
signer, err := sigstore.NewSigner(
    sigstore.WithPrivateKeyPEM(pemData, []byte("password")),
)
```

---

### WithFulcio

```go
func WithFulcio(baseURL string) SignerOption
```

Enables certificate issuance via Fulcio CA for keyless signing.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `baseURL` | `string` | Fulcio server URL |

**Public instance:** `https://fulcio.sigstore.dev`

**Example:**

```go
signer, err := sigstore.NewSigner(
    sigstore.WithEphemeralKey(),
    sigstore.WithFulcio("https://fulcio.sigstore.dev"),
)
```

---

### WithRekor

```go
func WithRekor(baseURL string) SignerOption
```

Enables transparency log recording via Rekor.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `baseURL` | `string` | Rekor server URL |

**Public instance:** `https://rekor.sigstore.dev`

**Example:**

```go
signer, err := sigstore.NewSigner(
    sigstore.WithEphemeralKey(),
    sigstore.WithFulcio("https://fulcio.sigstore.dev"),
    sigstore.WithRekor("https://rekor.sigstore.dev"),
)
```

---

## Verifier

### NewVerifier

```go
func NewVerifier(opts ...VerifierOption) (*Verifier, error)
```

Creates a sigstore-based verifier.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `opts` | `...VerifierOption` | Configuration options |

**Returns:**

| Type | Description |
|------|-------------|
| `*Verifier` | The configured verifier |
| `error` | Error if configuration fails |

**Example:**

```go
verifier, err := sigstore.NewVerifier(
    sigstore.WithIdentity(
        "https://accounts.google.com",
        "developer@company.com",
    ),
)
```

**Note:** If no identity is specified, the verifier logs a warning and accepts any valid signature. This is unsafe for production.

---

## Verifier Options

### WithTrustedRoot

```go
func WithTrustedRoot(tr root.TrustedMaterial) VerifierOption
```

Sets a custom trusted root for verification.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `tr` | `root.TrustedMaterial` | Trusted root material |

**Default:** Public Sigstore trusted root (fetched on first use)

---

### WithTrustedRootFile

```go
func WithTrustedRootFile(path string) VerifierOption
```

Loads a trusted root from a JSON file.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `path` | `string` | Path to trusted root JSON file |

**Example:**

```go
verifier, err := sigstore.NewVerifier(
    sigstore.WithTrustedRootFile("./custom-root.json"),
    sigstore.WithIdentity("https://auth.internal", "ci@internal"),
)
```

---

### WithIdentity

```go
func WithIdentity(issuer, subject string) VerifierOption
```

Requires signatures from a specific OIDC identity.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `issuer` | `string` | OIDC provider URL |
| `subject` | `string` | Expected identity |

**Common issuers:**

| Provider | Issuer URL |
|----------|-----------|
| Google | `https://accounts.google.com` |
| GitHub Actions | `https://token.actions.githubusercontent.com` |
| Microsoft | `https://login.microsoftonline.com/{tenant}/v2.0` |

**Example:**

```go
verifier, err := sigstore.NewVerifier(
    sigstore.WithIdentity(
        "https://token.actions.githubusercontent.com",
        "https://github.com/org/repo/.github/workflows/release.yml@refs/heads/main",
    ),
)
```

---

### WithLogger

```go
func WithLogger(logger *slog.Logger) VerifierOption
```

Sets a custom logger for the verifier.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `logger` | `*slog.Logger` | Logger instance |

**Example:**

```go
verifier, err := sigstore.NewVerifier(
    sigstore.WithLogger(slog.Default()),
    sigstore.WithIdentity("https://accounts.google.com", "user@example.com"),
)
```

---

## Helper Functions

### ParsePrivateKeyPEM

```go
func ParsePrivateKeyPEM(pemData, password []byte) (crypto.Signer, error)
```

Parses a PEM-encoded private key.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `pemData` | `[]byte` | PEM-encoded private key |
| `password` | `[]byte` | Password for encrypted keys |

**Returns:**

| Type | Description |
|------|-------------|
| `crypto.Signer` | The parsed private key |
| `error` | Error if parsing fails |

---

### NewStaticKeypair

```go
func NewStaticKeypair(key crypto.Signer) (*StaticKeypair, error)
```

Creates a Keypair from an existing `crypto.Signer`. Used internally by `WithPrivateKey`.

---

## Complete Examples

### Keyless Signing and Verification

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/meigma/blobber"
    "github.com/meigma/blobber/sigstore"
)

func main() {
    ctx := context.Background()

    // Create keyless signer
    signer, err := sigstore.NewSigner(
        sigstore.WithEphemeralKey(),
        sigstore.WithFulcio("https://fulcio.sigstore.dev"),
        sigstore.WithRekor("https://rekor.sigstore.dev"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Push with signing
    pushClient, _ := blobber.NewClient(blobber.WithSigner(signer))
    _, err = pushClient.Push(ctx, "ghcr.io/org/config:v1", os.DirFS("./config"))
    if err != nil {
        log.Fatal(err)
    }

    // Create verifier with identity requirement
    verifier, err := sigstore.NewVerifier(
        sigstore.WithIdentity("https://accounts.google.com", "developer@company.com"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Pull with verification
    pullClient, _ := blobber.NewClient(blobber.WithVerifier(verifier))
    err = pullClient.Pull(ctx, "ghcr.io/org/config:v1", "./output")
    if err != nil {
        log.Fatal(err)
    }
}
```

### Key-Based Signing

```go
// Load key from file
pemData, _ := os.ReadFile("private.pem")

signer, err := sigstore.NewSigner(
    sigstore.WithPrivateKeyPEM(pemData, nil),
    sigstore.WithRekor("https://rekor.sigstore.dev"), // optional transparency
)
if err != nil {
    log.Fatal(err)
}

client, _ := blobber.NewClient(blobber.WithSigner(signer))
```

---

## See Also

- [Options Reference](./options.md) - Client options including `WithSigner` and `WithVerifier`
- [Errors Reference](./errors.md) - Signing/verification errors
- [How to Sign Artifacts](../../how-to/sign-artifacts.md) - Practical signing guide
- [About Signing](../../explanation/about-signing.md) - Conceptual overview
