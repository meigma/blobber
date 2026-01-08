---
sidebar_position: 1
---

# Installation

Add blobber to your Go project.

## Requirements

- Go 1.21 or later
- Docker credentials configured for authenticated registries

## Install the Module

Add blobber to your Go module:

```bash
go get github.com/meigma/blobber@latest
```

## Import

Import in your code:

```go
import "github.com/meigma/blobber"
```

## Verify Installation

Create a simple test file to verify the import works:

```go
package main

import (
    "fmt"
    "github.com/meigma/blobber"
)

func main() {
    client, err := blobber.NewClient()
    if err != nil {
        panic(err)
    }
    fmt.Printf("Client created: %T\n", client)
}
```

Run it:

```bash
go run main.go
```

Expected output:

```
Client created: *blobber.Client
```

## Authentication

Blobber uses your existing Docker credentials from `~/.docker/config.json`. If you can `docker push` to a registry from your machine, blobber can too.

For programmatic credential management, see [WithCredentials](/reference/library/options#withcredentials).

## Next Steps

- [Quickstart](/getting-started/library/quickstart) - Basic push/pull in your code
- [Library Tutorial](/tutorials/library-basics) - Comprehensive walkthrough
- [Library Reference](/reference/library/client) - Full API documentation
