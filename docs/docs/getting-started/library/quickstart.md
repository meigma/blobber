---
sidebar_position: 2
---

# Quickstart

Basic blobber library usage in under 5 minutes.

## Prerequisites

- [blobber installed](/docs/getting-started/library/installation)
- Access to an OCI registry
- Docker credentials configured

## Push Files

Upload files to a registry:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/meigma/blobber"
)

func main() {
    ctx := context.Background()

    client, err := blobber.NewClient()
    if err != nil {
        log.Fatal(err)
    }

    // Push a local directory
    digest, err := client.Push(ctx, "ghcr.io/YOUR_USERNAME/config:v1", os.DirFS("./config"))
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Pushed: %s\n", digest)
}
```

## List Files

List files in a remote image without downloading:

```go
img, err := client.OpenImage(ctx, "ghcr.io/YOUR_USERNAME/config:v1")
if err != nil {
    log.Fatal(err)
}
defer img.Close()

entries, err := img.List()
if err != nil {
    log.Fatal(err)
}

for _, entry := range entries {
    fmt.Printf("%s (%d bytes)\n", entry.Path(), entry.Size())
}
```

## Read a Single File

Stream one file without downloading everything:

```go
rc, err := img.Open("app.yaml")
if err != nil {
    log.Fatal(err)
}
defer rc.Close()

content, err := io.ReadAll(rc)
if err != nil {
    log.Fatal(err)
}

fmt.Println(string(content))
```

## Pull All Files

Download everything to a local directory:

```go
err := client.Pull(ctx, "ghcr.io/YOUR_USERNAME/config:v1", "./output")
if err != nil {
    log.Fatal(err)
}
```

## Handle Errors

Use sentinel errors for precise error handling:

```go
import "errors"

img, err := client.OpenImage(ctx, ref)
if errors.Is(err, blobber.ErrNotFound) {
    fmt.Println("Image not found")
    return
}
if errors.Is(err, blobber.ErrUnauthorized) {
    fmt.Println("Authentication failed")
    return
}
if err != nil {
    log.Fatal(err)
}
```

## What's Next?

- [Library Tutorial](/docs/tutorials/library-basics) - Complete walkthrough with examples
- [Client Reference](/docs/reference/library/client) - All client methods
- [Options Reference](/docs/reference/library/options) - Configuration options
- [Errors Reference](/docs/reference/library/errors) - All sentinel errors
