---
sidebar_position: 2
---

# Tutorial: Learn the Blobber Library

In this tutorial, we'll learn to use blobber as a Go library by building a simple configuration management tool.

By the end, you'll know how to:

- Create a blobber client
- Push files to a registry
- List files in remote images
- Open and read individual files
- Pull complete images

## Prerequisites

- Go 1.21+
- Access to an OCI registry
- Docker credentials configured

## What We're Building

We'll create a small program that:

1. Pushes configuration files to a registry
2. Lists what's available
3. Reads specific files on-demand
4. Downloads everything when needed

## Step 1: Create a New Project

Set up a Go module:

```bash
mkdir blobber-example
cd blobber-example
go mod init blobber-example
go get github.com/gilmanlab/blobber@latest
```

Create `main.go`:

```go
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"testing/fstest"

	"github.com/gilmanlab/blobber"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx := context.Background()

	// We'll fill this in step by step
	_ = ctx
	return nil
}
```

## Step 2: Create a Client

Add client creation to the `run` function:

```go
func run() error {
	ctx := context.Background()

	// Create a client with default settings
	client, err := blobber.NewClient()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	fmt.Println("Client created successfully")
	_ = ctx
	return nil
}
```

Run it:

```bash
go run main.go
```

You should see:

```
Client created successfully
```

## Step 3: Push Files

Let's push some in-memory files. Add this after creating the client:

```go
func run() error {
	ctx := context.Background()

	client, err := blobber.NewClient()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	// Create an in-memory filesystem
	files := fstest.MapFS{
		"app.yaml": &fstest.MapFile{
			Data: []byte("name: myapp\nversion: 1.0.0\n"),
		},
		"database.yaml": &fstest.MapFile{
			Data: []byte("host: localhost\nport: 5432\n"),
		},
	}

	// Push to registry (replace with your registry)
	ref := "ghcr.io/YOUR_USERNAME/example-config:v1"
	digest, err := client.Push(ctx, ref, files)
	if err != nil {
		return fmt.Errorf("pushing: %w", err)
	}

	fmt.Printf("Pushed: %s\n", digest)
	return nil
}
```

Run it:

```bash
go run main.go
```

You should see:

```
Pushed: sha256:abc123...
```

## Step 4: Open and List Files

Now let's read what we pushed. Replace the run function:

```go
func run() error {
	ctx := context.Background()

	client, err := blobber.NewClient()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	ref := "ghcr.io/YOUR_USERNAME/example-config:v1"

	// Open the image for reading
	img, err := client.OpenImage(ctx, ref)
	if err != nil {
		return fmt.Errorf("opening image: %w", err)
	}
	defer img.Close()

	// List all files
	entries, err := img.List()
	if err != nil {
		return fmt.Errorf("listing: %w", err)
	}

	fmt.Println("Files in image:")
	for _, entry := range entries {
		fmt.Printf("  %s (%d bytes)\n", entry.Path(), entry.Size())
	}

	return nil
}
```

Run it:

```bash
go run main.go
```

You should see:

```
Files in image:
  app.yaml (26 bytes)
  database.yaml (28 bytes)
```

## Step 5: Read a Specific File

Let's read one file without downloading everything. Add this before the `return nil`:

```go
	// Open a specific file
	rc, err := img.Open("app.yaml")
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	fmt.Printf("\nContents of app.yaml:\n%s", content)
```

Run it:

```bash
go run main.go
```

You should see:

```
Files in image:
  app.yaml (26 bytes)
  database.yaml (28 bytes)

Contents of app.yaml:
name: myapp
version: 1.0.0
```

## Step 6: Walk All Files

The `Walk` method lets you process files with a callback. Replace the list logic:

```go
	// Walk all files
	fmt.Println("Walking files:")
	err = img.Walk(func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, _ := d.Info()
			fmt.Printf("  %s (%d bytes)\n", path, info.Size())
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking: %w", err)
	}
```

Note: You'll need to add `"io/fs"` to your imports.

## Step 7: Pull Everything

To download all files to disk, use the client's `Pull` method:

```go
	// Pull to local directory
	err = client.Pull(ctx, ref, "./downloaded")
	if err != nil {
		return fmt.Errorf("pulling: %w", err)
	}

	fmt.Println("\nPulled to ./downloaded")
```

Run it, then check the directory:

```bash
go run main.go
ls ./downloaded
```

## Step 8: Handle Errors

Blobber provides sentinel errors for common cases. Add error handling:

```go
import "errors"

// ...

	img, err := client.OpenImage(ctx, ref)
	if errors.Is(err, blobber.ErrNotFound) {
		return fmt.Errorf("image does not exist: %s", ref)
	}
	if errors.Is(err, blobber.ErrUnauthorized) {
		return fmt.Errorf("authentication failed for: %s", ref)
	}
	if err != nil {
		return fmt.Errorf("opening image: %w", err)
	}
```

## Complete Example

Here's the full program:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"testing/fstest"

	"github.com/gilmanlab/blobber"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx := context.Background()

	client, err := blobber.NewClient()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	// Create test files
	files := fstest.MapFS{
		"app.yaml": &fstest.MapFile{
			Data: []byte("name: myapp\nversion: 1.0.0\n"),
		},
		"database.yaml": &fstest.MapFile{
			Data: []byte("host: localhost\nport: 5432\n"),
		},
	}

	ref := "ghcr.io/YOUR_USERNAME/example-config:v1"

	// Push
	digest, err := client.Push(ctx, ref, files)
	if err != nil {
		return fmt.Errorf("pushing: %w", err)
	}
	fmt.Printf("Pushed: %s\n\n", digest)

	// Open image
	img, err := client.OpenImage(ctx, ref)
	if errors.Is(err, blobber.ErrNotFound) {
		return fmt.Errorf("image not found: %s", ref)
	}
	if err != nil {
		return fmt.Errorf("opening: %w", err)
	}
	defer img.Close()

	// Walk files
	fmt.Println("Files:")
	err = img.Walk(func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, _ := d.Info()
			fmt.Printf("  %s (%d bytes)\n", path, info.Size())
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking: %w", err)
	}

	// Read one file
	rc, err := img.Open("app.yaml")
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("reading: %w", err)
	}
	fmt.Printf("\napp.yaml:\n%s", content)

	return nil
}
```

## What We Learned

- `blobber.NewClient()` creates a client with default settings
- `client.Push(ctx, ref, fs)` uploads an `fs.FS` to a registry
- `client.OpenImage(ctx, ref)` opens an image for reading
- `img.List()` returns file entries
- `img.Open(path)` returns a reader for one file
- `img.Walk(fn)` walks all files
- `client.Pull(ctx, ref, dir)` downloads everything
- Sentinel errors like `ErrNotFound` enable precise error handling

## Next Steps

- [Library Reference: Client](/reference/library/client) - All client methods and options
- [Library Reference: Options](/reference/library/options) - Configure authentication, caching, and more
- [About eStargz](/explanation/about-estargz) - Why selective retrieval is efficient
