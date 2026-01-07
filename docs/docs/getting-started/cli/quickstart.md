---
sidebar_position: 2
---

# Quickstart

Push and pull files to an OCI registry in under 5 minutes.

## Prerequisites

- [blobber installed](/getting-started/cli/installation)
- Access to an OCI registry (GitHub Container Registry, Docker Hub, etc.)
- Docker credentials configured (`docker login`)

## 1. Create Test Files

Create a directory with some files to push:

```bash
mkdir myconfig
echo "database: postgres://localhost:5432/mydb" > myconfig/database.yaml
echo "port: 8080" > myconfig/server.yaml
echo "log_level: info" > myconfig/logging.yaml
```

## 2. Push to Registry

Push the directory to your registry:

```bash
blobber push ./myconfig ghcr.io/YOUR_USERNAME/config:v1
```

Output:

```
sha256:a1b2c3d4e5f6...
```

The SHA256 digest confirms your files are stored.

## 3. List Remote Files

See what's in the image without downloading:

```bash
blobber list ghcr.io/YOUR_USERNAME/config:v1
```

Output:

```
database.yaml
logging.yaml
server.yaml
```

## 4. Stream a Single File

View a specific file without pulling everything:

```bash
blobber cat ghcr.io/YOUR_USERNAME/config:v1 database.yaml
```

Output:

```
database: postgres://localhost:5432/mydb
```

## 5. Pull All Files

Download everything to a local directory:

```bash
blobber pull ghcr.io/YOUR_USERNAME/config:v1 ./downloaded
```

Verify the files:

```bash
ls ./downloaded
```

Output:

```
database.yaml  logging.yaml  server.yaml
```

## What's Next?

- [CLI Tutorial](/tutorials/cli-basics) - Learn all CLI features step-by-step
- [How to Authenticate](/how-to/authenticate) - Configure registry credentials
- [CLI Reference](/reference/cli/push) - Complete command documentation
