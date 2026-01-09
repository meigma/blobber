package blobber

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/opencontainers/go-digest"

	"github.com/meigma/blobber/core"
	"github.com/meigma/blobber/internal/progress"
)

// Push uploads files from src to the given image reference.
// The ref must be fully qualified (e.g., "ghcr.io/org/repo:tag").
// Returns the digest of the pushed image.
func (c *Client) Push(ctx context.Context, ref string, src fs.FS, opts ...PushOption) (string, error) {
	// Apply options
	cfg := &pushConfig{
		compression: GzipCompression(), // default
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// Build eStargz blob
	result, err := c.builder.Build(ctx, src, cfg.compression)
	if err != nil {
		return "", fmt.Errorf("build archive: %w", err)
	}
	defer result.Blob.Close()

	// Wrap blob reader for progress tracking if callback provided
	var blobReader io.Reader = result.Blob
	if cfg.progress != nil {
		blobReader = progress.NewReader(result.Blob, result.BlobSize, func(transferred, total int64) {
			cfg.progress(ProgressEvent{
				Operation:        "push",
				BytesTransferred: transferred,
				TotalBytes:       total,
			})
		})
	}

	// Push to registry
	regOpts := RegistryPushOptions{
		MediaType:   cfg.mediaType,
		Annotations: cfg.annotations,
		TOCDigest:   result.TOCDigest,
		DiffID:      result.DiffID,
		BlobDigest:  result.BlobDigest,
		BlobSize:    result.BlobSize,
	}

	manifestDigest, err := c.registry.Push(ctx, ref, blobReader, &regOpts)
	if err != nil {
		return "", fmt.Errorf("push %s: %w", ref, err)
	}

	// Sign and store as referrer if signer configured
	if c.signer != nil {
		if err := c.signAndStoreReferrer(ctx, ref, manifestDigest); err != nil {
			return "", fmt.Errorf("sign %s: %w", ref, err)
		}
	}

	return manifestDigest, nil
}

// signAndStoreReferrer signs the manifest and stores the signature as an OCI referrer.
func (c *Client) signAndStoreReferrer(ctx context.Context, ref, manifestDigest string) error {
	d, err := digest.Parse(manifestDigest)
	if err != nil {
		return fmt.Errorf("parse digest: %w", err)
	}

	// Fetch the manifest bytes for signing
	manifestRef := digestReference(ref, manifestDigest)
	manifestBytes, _, err := c.registry.FetchManifest(ctx, manifestRef)
	if err != nil {
		return fmt.Errorf("fetch manifest: %w", err)
	}

	// Sign the manifest
	sig, err := c.signer.Sign(ctx, d, manifestBytes)
	if err != nil {
		return fmt.Errorf("signing: %w", err)
	}

	// Store signature as OCI referrer artifact
	_, err = c.registry.PushReferrer(ctx, ref, manifestDigest, sig.Data, &core.ReferrerPushOptions{
		ArtifactType: sig.MediaType,
		Annotations: map[string]string{
			"org.opencontainers.image.created": time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		return fmt.Errorf("storing signature: %w", err)
	}

	return nil
}
