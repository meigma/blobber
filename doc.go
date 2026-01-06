// Package blobber provides push/pull operations for arbitrary files
// to and from OCI container registries.
//
// Blobber uses eStargz (seekable tar.gz) format to enable efficient
// file listing and selective retrieval without downloading entire images.
//
// # Basic Usage
//
// Create a client and push files:
//
//	client, err := blobber.NewClient()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Push a directory
//	digest, err := client.Push(ctx, "ghcr.io/org/repo:v1", os.DirFS("./config"))
//
//	// List files without downloading
//	entries, err := client.List(ctx, "ghcr.io/org/repo:v1")
//
//	// Open a single file
//	rc, err := client.Open(ctx, "ghcr.io/org/repo:v1", "config.yaml")
//
// # Authentication
//
// By default, credentials are resolved from Docker config (~/.docker/config.json)
// and credential helpers. Override with WithCredentials or WithCredentialStore.
//
// # Compression
//
// eStargz blobs support gzip (default) and zstd compression:
//
//	client.Push(ctx, ref, src, blobber.WithCompression(blobber.ZstdCompression()))
package blobber
