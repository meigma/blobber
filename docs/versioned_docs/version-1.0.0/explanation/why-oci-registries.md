---
sidebar_position: 1
---

# Why OCI Registries?

OCI container registries are designed for distributing container images, but their properties make them excellent for storing any kind of file.

## The Problem with File Distribution

Distributing files across environments is a solved problem, but the solutions often have drawbacks:

**Object storage (S3, GCS, Azure Blob):**
- Requires separate credentials and SDKs
- No built-in versioning semantics
- Pull-based only, no push notifications
- Vendor-specific APIs

**Git/Git LFS:**
- Designed for source code, not binary artifacts
- LFS requires additional setup
- Large files strain the system
- Clone operations download everything

**FTP/SFTP:**
- Legacy authentication
- No content addressing
- Manual version management
- No distribution optimization

**Custom HTTP servers:**
- Build and maintain infrastructure
- Implement authentication yourself
- No standard tooling

## Why OCI Registries Work

Container registries solve these problems because they were designed for a similar use case: distributing large binary artifacts (container layers) reliably across the internet.

### Existing Infrastructure

Every major cloud provider runs OCI registries:
- GitHub Container Registry (ghcr.io)
- Docker Hub
- Amazon ECR
- Google Container Registry / Artifact Registry
- Azure Container Registry
- Self-hosted options (Harbor, Distribution)

You likely already have access to one. No new infrastructure needed.

### First-Class Authentication

Registries have robust authentication built in:
- Token-based auth
- OAuth integration (GitHub, Google, Azure AD)
- Credential helpers for secure storage
- Fine-grained access control

If you can `docker push`, you can `blobber push`. Same credentials, same permissions.

### Content Addressing

Every blob in a registry is identified by its SHA256 digest:

```
ghcr.io/org/config@sha256:abc123...
```

This provides:
- **Immutability** - Content at a digest never changes
- **Integrity** - Automatic verification on pull
- **Deduplication** - Identical content stored once
- **Reproducibility** - Reference exact versions in deployments

### Versioning with Tags

Tags provide human-readable version management:

```
ghcr.io/org/config:v1.0.0
ghcr.io/org/config:latest
ghcr.io/org/config:staging
```

Tags can be mutable (like `latest`) or treated as immutable (like semver tags). The underlying content is always addressable by digest.

### Distribution Optimization

Registries are built for efficient distribution:
- Edge caching through CDNs
- Parallel chunk downloads
- Resume interrupted transfers
- Geographic distribution

These optimizations happen automatically.

## Trade-offs

OCI registries aren't perfect for every use case:

**Overkill for small files:**
If you're distributing a single 1KB config file, the OCI manifest overhead adds complexity. A simple HTTP server might be simpler.

**No partial updates:**
Each push creates a complete new layer. You can't patch a single file in an existing image. For frequently-changing single files, consider whether this model fits.

**Registry availability:**
Your files are only accessible when the registry is. For critical systems, consider registry replication or local caching.

**Learning curve:**
Teams unfamiliar with container concepts need to understand references, digests, and tags.

## When to Use Blobber

Blobber makes sense when:

- You already use container registries
- You need versioned, immutable file distribution
- You want the same auth for code and config
- You're distributing to multiple environments
- You need selective file retrieval (eStargz advantage)

Consider alternatives when:

- Files are tiny and change constantly
- You need real-time synchronization
- You're in an environment without registry access

## The Bigger Picture

Using OCI registries for files follows a broader trend: leveraging container infrastructure for non-container workloads. Projects like ORAS (OCI Registry as Storage) formalize this pattern.

Blobber adds first-class eStargz support on top, enabling efficient selective retrieval that pure ORAS doesn't provide.

## See Also

- [About eStargz](./about-estargz.md) - Why selective retrieval is efficient
- [Architecture](./architecture.md) - How blobber uses OCI registries
