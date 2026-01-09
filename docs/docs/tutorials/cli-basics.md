---
sidebar_position: 1
---

# Tutorial: Learn the Blobber CLI

In this tutorial, we'll learn to use the blobber CLI by working through a realistic scenario: managing configuration files for a web application.

By the end, you'll know how to:

- Push directories to an OCI registry
- List files in remote images
- Stream individual files on-demand
- Pull complete images to local directories
- Use long-format output for detailed information

## Prerequisites

- [blobber installed](/docs/getting-started/cli/installation)
- Access to an OCI registry
- Docker credentials configured

## What We're Building

We'll create configuration files for a web application, push them to a registry, and practice all the ways to retrieve them. This mirrors how teams distribute configuration across environments.

## Step 1: Create a Project Directory

Let's create a realistic set of configuration files:

```bash
mkdir webapp-config
cd webapp-config
```

Create the main application config:

```bash
cat > app.yaml << 'EOF'
name: mywebapp
version: 1.0.0
environment: production
EOF
```

Create a database configuration:

```bash
cat > database.yaml << 'EOF'
host: db.example.com
port: 5432
name: webapp_prod
pool_size: 20
EOF
```

Create a nested directory for feature flags:

```bash
mkdir features
cat > features/flags.json << 'EOF'
{
  "dark_mode": true,
  "beta_features": false,
  "max_upload_mb": 100
}
EOF
```

Your directory structure should look like this:

```
webapp-config/
├── app.yaml
├── database.yaml
└── features/
    └── flags.json
```

## Step 2: Push to the Registry

Now we'll push these files to your registry. Replace `YOUR_REGISTRY` with your actual registry path:

```bash
blobber push . ghcr.io/YOUR_REGISTRY/webapp-config:v1
```

You should see output like:

```
sha256:7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e
```

This digest uniquely identifies your configuration bundle. Save it for reproducible deployments.

## Step 3: List Remote Files

Let's verify what we pushed without downloading anything:

```bash
blobber ls ghcr.io/YOUR_REGISTRY/webapp-config:v1
```

Output:

```
app.yaml
database.yaml
features/flags.json
```

Notice how nested files show their full path.

## Step 4: Use Long Format

For more details, add the `-l` flag:

```bash
blobber ls -l ghcr.io/YOUR_REGISTRY/webapp-config:v1
```

Output:

```
app.yaml             52      0644
database.yaml        75      0644
features/flags.json  89      0644
```

The columns show: path, size in bytes, and file mode.

## Step 5: Stream a Single File

Suppose you only need to check the database config. Instead of downloading everything:

```bash
blobber cat ghcr.io/YOUR_REGISTRY/webapp-config:v1 database.yaml
```

Output:

```
host: db.example.com
port: 5432
name: webapp_prod
pool_size: 20
```

This is powerful for large images - you download only what you need.

## Step 6: Stream Nested Files

Files in subdirectories work the same way:

```bash
blobber cat ghcr.io/YOUR_REGISTRY/webapp-config:v1 features/flags.json
```

Output:

```json
{
  "dark_mode": true,
  "beta_features": false,
  "max_upload_mb": 100
}
```

## Step 7: Redirect to a File

You can save streamed content directly:

```bash
blobber cat ghcr.io/YOUR_REGISTRY/webapp-config:v1 app.yaml > local-app.yaml
cat local-app.yaml
```

Output:

```
name: mywebapp
version: 1.0.0
environment: production
```

## Step 8: Pull Everything

When you need all the files, use pull:

```bash
blobber pull ghcr.io/YOUR_REGISTRY/webapp-config:v1 ./restored-config
```

Verify the contents:

```bash
find ./restored-config -type f
```

Output:

```
./restored-config/app.yaml
./restored-config/database.yaml
./restored-config/features/flags.json
```

## Step 9: Handle Existing Files

If you try to pull into a directory with existing files:

```bash
blobber pull ghcr.io/YOUR_REGISTRY/webapp-config:v1 ./restored-config
```

You'll see an error:

```
Error: 3 files already exist (use --overwrite to replace)
```

To overwrite existing files:

```bash
blobber pull --overwrite ghcr.io/YOUR_REGISTRY/webapp-config:v1 ./restored-config
```

## Step 10: Push a New Version

Let's update a config and push a new version:

```bash
cat > app.yaml << 'EOF'
name: mywebapp
version: 1.1.0
environment: production
debug: false
EOF
```

Push as v2:

```bash
blobber push . ghcr.io/YOUR_REGISTRY/webapp-config:v2
```

Now you have two versions available:

```bash
blobber cat ghcr.io/YOUR_REGISTRY/webapp-config:v1 app.yaml
blobber cat ghcr.io/YOUR_REGISTRY/webapp-config:v2 app.yaml
```

## Step 11: Sign Your Artifacts (Optional)

For production use, you can sign artifacts to ensure integrity and provenance:

```bash
blobber push --sign . ghcr.io/YOUR_REGISTRY/webapp-config:v1-signed
```

This opens a browser for OIDC authentication, then signs the artifact using Sigstore.

Later, verify when pulling:

```bash
blobber pull --verify \
  --verify-issuer https://accounts.google.com \
  --verify-subject your-email@example.com \
  ghcr.io/YOUR_REGISTRY/webapp-config:v1-signed ./verified-output
```

If the signature is invalid or missing, the pull fails before downloading any content.

## What We Learned

- `blobber push <dir> <ref>` uploads a directory to a registry
- `blobber ls <ref>` shows files without downloading
- `blobber ls -l <ref>` shows sizes and permissions
- `blobber cat <ref> <path>` streams a single file
- `blobber pull <ref> <dir>` downloads everything
- `blobber pull --overwrite` replaces existing files
- Tags (`:v1`, `:v2`) manage versions
- `--sign` adds cryptographic signatures for supply chain security
- `--verify` ensures artifacts are signed by trusted identities

## Next Steps

- [How to Authenticate](/docs/how-to/authenticate) - Configure credentials for private registries
- [How to Sign Artifacts](/docs/how-to/sign-artifacts) - Detailed signing guide
- [How to Use Compression](/docs/how-to/use-compression) - Choose between gzip and zstd
- [CLI Reference](/docs/reference/cli/push) - Complete command documentation
- [About eStargz](/docs/explanation/about-estargz) - Why selective retrieval is efficient
- [About Signing](/docs/explanation/about-signing) - Understanding Sigstore signing
