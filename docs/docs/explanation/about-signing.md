---
sidebar_position: 4
---

# About Artifact Signing

This document explains why artifact signing matters, how Sigstore works, and when to use signing with blobber.

## The Supply Chain Problem

When you pull an artifact from a registry, how do you know:

1. **It came from who you think it did?** - Anyone with write access could have pushed it
2. **It hasn't been modified?** - A compromised registry or MITM attack could alter content
3. **It's the same artifact you tested?** - Tags are mutable; `v1.0.0` today might differ from yesterday

Content-addressable digests (`sha256:...`) solve the integrity problem but not the provenance problem. You can verify an artifact is exactly what someone pushed, but not *who* pushed it or *whether you should trust them*.

Cryptographic signing solves this by binding an identity to an artifact.

## How Signing Works

### The Basic Flow

1. **Push time:** After uploading the artifact, the pusher signs the manifest digest with their private key
2. **Storage:** The signature is stored as an OCI "referrer" artifact, linked to the original manifest
3. **Pull time:** Before using the artifact, the puller fetches the signature and verifies it against the manifest

```
┌─────────────────────────────────────────────────────────────┐
│                        OCI Registry                          │
│                                                              │
│   ┌──────────────┐         references         ┌───────────┐ │
│   │  Signature   │ ◄─────────────────────────►│  Manifest │ │
│   │  (referrer)  │         (subject)          │  (image)  │ │
│   └──────────────┘                            └───────────┘ │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### OCI Referrers

Blobber uses the OCI 1.1 Referrers API to store signatures. A signature is stored as an OCI manifest with:

- **Subject field:** Points to the signed manifest's digest
- **Artifact type:** Identifies it as a Sigstore bundle
- **Layer:** Contains the actual signature data

This approach keeps signatures associated with their artifacts and works with standard OCI tooling.

## Sigstore: Keyless Signing

Traditional signing requires managing private keys—generating, storing, rotating, and revoking them. This operational burden discourages adoption.

[Sigstore](https://sigstore.dev) offers "keyless" signing that eliminates key management:

### How Keyless Works

1. **Ephemeral key:** A temporary key pair is generated for each signing operation
2. **OIDC authentication:** You authenticate with an identity provider (GitHub, Google, Microsoft)
3. **Fulcio CA:** Issues a short-lived certificate binding your OIDC identity to the ephemeral public key
4. **Rekor log:** Records the signing event in a tamper-evident transparency log
5. **Bundle:** The signature, certificate, and log entry are packaged together

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   GitHub    │     │   Fulcio    │     │   Rekor     │
│   (OIDC)    │     │   (CA)      │     │   (Log)     │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │
       │ 1. Authenticate   │                   │
       │◄──────────────────┤                   │
       │                   │                   │
       │ 2. Request cert   │                   │
       ├──────────────────►│                   │
       │                   │                   │
       │ 3. Short-lived    │                   │
       │    certificate    │                   │
       │◄──────────────────┤                   │
       │                   │                   │
       │ 4. Sign artifact  │                   │
       │   (locally)       │                   │
       │                   │                   │
       │ 5. Record in log  │                   │
       ├───────────────────┼──────────────────►│
       │                   │                   │
       │ 6. Log entry      │                   │
       │◄──────────────────┼───────────────────┤
       │                   │                   │
       ▼                   ▼                   ▼
   ┌─────────────────────────────────────────────┐
   │              Sigstore Bundle                 │
   │  (signature + certificate + log proof)      │
   └─────────────────────────────────────────────┘
```

### Why Keyless is Powerful

- **No key management:** Keys exist only for seconds
- **Identity-based trust:** Verify by email/identity, not key fingerprint
- **Auditability:** All signatures recorded in public transparency log
- **Compromise recovery:** No long-lived keys to steal

### Verification with Keyless

When verifying, you specify the expected identity:

```bash
blobber pull --verify \
  --verify-issuer https://accounts.google.com \
  --verify-subject developer@company.com \
  ghcr.io/org/config:v1 ./output
```

This verifies:
1. The signature is cryptographically valid
2. The certificate was issued by Fulcio
3. The signing was recorded in Rekor
4. The signer's identity matches your requirements

## Key-Based Signing

For environments that can't use OIDC (air-gapped, CI systems without OIDC), blobber supports traditional key-based signing:

```bash
blobber push --sign --sign-key ./private.pem ./config ghcr.io/org/config:v1
```

Key-based signing:
- Uses your existing private key (ECDSA, RSA, or Ed25519)
- Can optionally record to Rekor for auditability
- Requires you to manage key lifecycle

## Trust Models

### Keyless Trust

Trust is based on identity claims from OIDC providers:

| What You Trust | Example |
|----------------|---------|
| OIDC Provider | `https://accounts.google.com` |
| Identity | `developer@company.com` |
| Fulcio CA | Issues valid certificates |
| Rekor Log | Provides tamper evidence |

**Best for:** Teams using GitHub Actions, Google Cloud Build, or similar CI systems with OIDC.

### Key-Based Trust

Trust is based on possession of private keys:

| What You Trust | Example |
|----------------|---------|
| Public Key | Distributed out-of-band |
| Key Holder | Whoever controls the private key |

**Best for:** Air-gapped environments, custom PKI, or when OIDC isn't available.

## When to Sign

### Sign When

- **Distributing to production** - Ensure only authorized artifacts deploy
- **Publishing public artifacts** - Let consumers verify authenticity
- **Compliance requirements** - Audit trails for artifact provenance
- **Multi-team workflows** - Team A produces, Team B consumes

### Maybe Skip When

- **Local development** - Overhead without security benefit
- **Ephemeral test artifacts** - Short-lived, not worth signing
- **Air-gapped without key infra** - Keyless won't work, keys add complexity

## When to Verify

### Verify When

- **Pulling in CI/CD** - Gate deployments on signature validity
- **Production systems** - Prevent unauthorized artifacts from running
- **Consuming external artifacts** - Trust but verify

### Verification Modes

| Mode | Flag | Use Case |
|------|------|----------|
| Strict identity | `--verify-issuer` + `--verify-subject` | Production (recommended) |
| Any valid signature | `--verify-unsafe` | Development/testing only |

## Trade-offs

### Benefits

- **Supply chain security:** Cryptographic proof of provenance
- **Tamper detection:** Any modification invalidates signature
- **Audit trail:** Transparency log records all signing events
- **Zero key management:** With keyless signing

### Costs

- **Latency:** Signing adds ~1-2 seconds (Fulcio + Rekor calls)
- **Network dependency:** Keyless requires Sigstore infrastructure
- **Complexity:** Additional concepts to understand
- **Identity management:** Must track authorized signers

### Failure Modes

| Scenario | Impact | Mitigation |
|----------|--------|------------|
| Sigstore outage | Can't sign (keyless) | Fall back to key-based or skip signing |
| Wrong identity configured | Verification fails | Document expected identities |
| Expired certificates | Verification fails | Rekor timestamp proves validity at signing time |

## Architecture in Blobber

Blobber's signing architecture:

```
┌─────────────────────────────────────────────────────────────┐
│                      blobber (root module)                   │
│                                                              │
│   ┌─────────────┐    ┌─────────────┐                        │
│   │   Signer    │    │  Verifier   │  ◄── Interfaces        │
│   │  interface  │    │  interface  │                        │
│   └──────┬──────┘    └──────┬──────┘                        │
│          │                  │                                │
└──────────┼──────────────────┼────────────────────────────────┘
           │                  │
           ▼                  ▼
┌─────────────────────────────────────────────────────────────┐
│                blobber/sigstore (separate module)            │
│                                                              │
│   ┌─────────────┐    ┌─────────────┐                        │
│   │   Signer    │    │  Verifier   │  ◄── Implementations   │
│   │   (impl)    │    │   (impl)    │                        │
│   └─────────────┘    └─────────────┘                        │
│                                                              │
│   Uses: sigstore-go, protobuf, gRPC, OIDC                   │
└─────────────────────────────────────────────────────────────┘
```

The sigstore implementation lives in a separate Go module to avoid pulling heavy dependencies into the core blobber package. Users who don't need signing can use blobber without sigstore-go.

## See Also

- [How to Sign Artifacts](/docs/how-to/sign-artifacts) - Step-by-step signing guide
- [How to Verify Signatures](/docs/how-to/verify-signatures) - Verification guide
- [CLI Reference: push](/docs/reference/cli/push) - Signing flags
- [CLI Reference: pull](/docs/reference/cli/pull) - Verification flags
- [Sigstore Documentation](https://docs.sigstore.dev) - Official Sigstore docs
