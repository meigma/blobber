---
slug: /
title: Blobber
sidebar_position: 1
---

# Blobber

Blobber is a Go module and CLI for pushing and pulling arbitrary files to and from OCI container registries. It turns any container registry into a general-purpose storage medium.

## Key Features

- **Works with any OCI registry** - GitHub Container Registry, Docker Hub, AWS ECR, or your own
- **List files without downloading** - See what's in an image before pulling
- **Stream individual files** - Fetch only what you need, on-demand
- **First-class eStargz support** - Efficient seekable compression for selective retrieval
- **Cryptographic signing** - Sign artifacts with Sigstore for supply chain security

## Quick Example

```bash
# Push a directory to a registry
blobber push ./config ghcr.io/myorg/config:v1

# List files without downloading
blobber ls ghcr.io/myorg/config:v1

# Stream a single file to stdout
blobber cat ghcr.io/myorg/config:v1 app.yaml

# Pull everything to a local directory
blobber pull ghcr.io/myorg/config:v1 ./output
```

## Choose Your Path

<div className="row">
  <div className="col col--6">
    <div className="card margin-bottom--lg">
      <div className="card__header">
        <h3>CLI User</h3>
      </div>
      <div className="card__body">
        <p>Use blobber from the command line to push and pull files.</p>
      </div>
      <div className="card__footer">
        <a className="button button--primary button--block" href="/docs/getting-started/cli/installation">Get Started</a>
      </div>
    </div>
  </div>
  <div className="col col--6">
    <div className="card margin-bottom--lg">
      <div className="card__header">
        <h3>Go Developer</h3>
      </div>
      <div className="card__body">
        <p>Import blobber as a library in your Go applications.</p>
      </div>
      <div className="card__footer">
        <a className="button button--secondary button--block" href="/docs/getting-started/library/installation">Get Started</a>
      </div>
    </div>
  </div>
</div>

## Why OCI Registries?

Container registries offer compelling properties for file storage:

- **Existing infrastructure** - Registries are already deployed everywhere
- **Built-in authentication** - Leverage registry auth mechanisms
- **Immutability** - Pin to digest for reproducible deployments
- **Versioning** - Tag-based version management

Learn more in [Why OCI Registries](/docs/explanation/why-oci-registries).
