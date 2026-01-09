---
sidebar_position: 1
---

# How to Authenticate with Registries

Configure credentials for private OCI registries.

## Prerequisites

- blobber installed
- Account on your target registry

## Using Docker Credentials

Blobber uses your existing Docker credentials from `~/.docker/config.json`.

### Step 1: Log in with Docker

```bash
docker login ghcr.io
```

Enter your username and token when prompted.

### Step 2: Verify with Blobber

```bash
blobber list ghcr.io/your-org/your-repo:tag
```

If authentication works, you'll see the file listing.

## Registry-Specific Instructions

### GitHub Container Registry (ghcr.io)

1. Create a Personal Access Token with `read:packages` and `write:packages` scopes
2. Log in:

```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u YOUR_USERNAME --password-stdin
```

### Docker Hub

```bash
docker login
```

### Amazon ECR

```bash
aws ecr get-login-password --region us-east-1 | \
  docker login --username AWS --password-stdin 123456789.dkr.ecr.us-east-1.amazonaws.com
```

### Google Container Registry (gcr.io)

```bash
gcloud auth configure-docker
```

### Azure Container Registry

```bash
az acr login --name myregistry
```

## Using Credential Helpers

Docker credential helpers provide secure credential storage. Blobber supports them automatically.

### macOS (Keychain)

Docker Desktop configures this by default. Verify in `~/.docker/config.json`:

```json
{
  "credsStore": "osxkeychain"
}
```

### Linux (pass)

Install docker-credential-pass, then configure:

```json
{
  "credsStore": "pass"
}
```

## Troubleshooting

### Error: authentication failed

1. Verify credentials work with Docker:

```bash
docker pull ghcr.io/your-org/your-repo:tag
```

2. Check token permissions (read/write packages)

3. Verify the token hasn't expired

### Error: unauthorized

The image may not exist, or you may lack access. Verify:

```bash
docker manifest inspect ghcr.io/your-org/your-repo:tag
```

## See Also

- [CLI Reference: push](../reference/cli/push.md)
- [CLI Reference: pull](../reference/cli/pull.md)
