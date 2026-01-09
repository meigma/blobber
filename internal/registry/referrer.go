package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry"

	"github.com/meigma/blobber/core"
)

// PushReferrer pushes a referrer artifact that references the subject digest.
// The data is stored as a single layer in an OCI manifest with the subject field set.
// Returns the referrer manifest digest.
func (r *orasRegistry) PushReferrer(ctx context.Context, ref, subjectDigest string, data []byte, opts *core.ReferrerPushOptions) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	parsedRef, err := registry.ParseReference(ref)
	if err != nil {
		return "", core.ErrInvalidRef
	}

	repo, err := r.newRepository(parsedRef)
	if err != nil {
		return "", fmt.Errorf("create repository: %w", err)
	}

	// Create blob descriptor for the referrer data (e.g., signature bundle)
	blobDesc := ocispec.Descriptor{
		MediaType: opts.ArtifactType,
		Digest:    digest.FromBytes(data),
		Size:      int64(len(data)),
	}

	// Push the blob
	if pushErr := repo.Blobs().Push(ctx, blobDesc, bytes.NewReader(data)); pushErr != nil {
		return "", fmt.Errorf("push referrer blob: %w", mapError(pushErr))
	}

	// Parse subject digest
	subjDigest, err := digest.Parse(subjectDigest)
	if err != nil {
		return "", fmt.Errorf("parse subject digest: %w", err)
	}

	// Create empty config (OCI 1.1 artifact pattern)
	emptyConfig := []byte("{}")
	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeEmptyJSON,
		Digest:    digest.FromBytes(emptyConfig),
		Size:      int64(len(emptyConfig)),
	}

	// Push empty config
	if configErr := repo.Blobs().Push(ctx, configDesc, bytes.NewReader(emptyConfig)); configErr != nil {
		return "", fmt.Errorf("push config: %w", mapError(configErr))
	}

	// Build manifest annotations
	annotations := make(map[string]string)
	maps.Copy(annotations, opts.Annotations)

	// Create manifest with subject reference (OCI 1.1 referrer)
	manifest := ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: opts.ArtifactType,
		Config:       configDesc,
		Layers:       []ocispec.Descriptor{blobDesc},
		Subject: &ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageManifest,
			Digest:    subjDigest,
		},
		Annotations: annotations,
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("marshal manifest: %w", err)
	}

	manifestDesc := ocispec.Descriptor{
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: opts.ArtifactType,
		Digest:       digest.FromBytes(manifestJSON),
		Size:         int64(len(manifestJSON)),
	}

	// Push manifest (registry will index it as a referrer)
	if err := repo.Manifests().Push(ctx, manifestDesc, bytes.NewReader(manifestJSON)); err != nil {
		return "", fmt.Errorf("push manifest: %w", mapError(err))
	}

	return manifestDesc.Digest.String(), nil
}

// FetchReferrers returns all referrers for a subject digest, optionally filtered by artifact type.
// Uses the OCI 1.1 referrers API.
func (r *orasRegistry) FetchReferrers(ctx context.Context, ref, subjectDigest, artifactType string) ([]core.Referrer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	parsedRef, err := registry.ParseReference(ref)
	if err != nil {
		return nil, core.ErrInvalidRef
	}

	repo, err := r.newRepository(parsedRef)
	if err != nil {
		return nil, fmt.Errorf("create repository: %w", err)
	}

	subjDigest, err := digest.Parse(subjectDigest)
	if err != nil {
		return nil, fmt.Errorf("parse subject digest: %w", err)
	}

	// Query referrers using ORAS
	subjectDesc := ocispec.Descriptor{
		Digest: subjDigest,
	}

	var referrers []core.Referrer
	err = repo.Referrers(ctx, subjectDesc, artifactType, func(refs []ocispec.Descriptor) error {
		for _, desc := range refs {
			referrers = append(referrers, core.Referrer{
				Digest:       desc.Digest.String(),
				ArtifactType: desc.ArtifactType,
				Annotations:  desc.Annotations,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list referrers: %w", mapError(err))
	}

	return referrers, nil
}

// FetchReferrer fetches the content of a specific referrer by its digest.
// Returns the first layer's content (the signature/attestation data).
func (r *orasRegistry) FetchReferrer(ctx context.Context, ref, referrerDigest string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	parsedRef, err := registry.ParseReference(ref)
	if err != nil {
		return nil, core.ErrInvalidRef
	}

	repo, err := r.newRepository(parsedRef)
	if err != nil {
		return nil, fmt.Errorf("create repository: %w", err)
	}

	refDigest, err := digest.Parse(referrerDigest)
	if err != nil {
		return nil, fmt.Errorf("parse referrer digest: %w", err)
	}

	// Resolve the manifest to get full descriptor (including size)
	manifestDesc, err := repo.Manifests().Resolve(ctx, refDigest.String())
	if err != nil {
		return nil, fmt.Errorf("resolve referrer manifest: %w", mapError(err))
	}

	rc, err := repo.Manifests().Fetch(ctx, manifestDesc)
	if err != nil {
		return nil, fmt.Errorf("fetch referrer manifest: %w", mapError(err))
	}
	defer rc.Close()

	manifestBytes, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	// Parse manifest to get layer descriptor
	var manifest ocispec.Manifest
	if unmarshalErr := json.Unmarshal(manifestBytes, &manifest); unmarshalErr != nil {
		return nil, fmt.Errorf("parse referrer manifest: %w", unmarshalErr)
	}

	if len(manifest.Layers) == 0 {
		return nil, errors.New("referrer has no layers")
	}

	// Fetch the first layer (the signature/attestation data)
	layerRC, err := repo.Blobs().Fetch(ctx, manifest.Layers[0])
	if err != nil {
		return nil, fmt.Errorf("fetch referrer layer: %w", mapError(err))
	}
	defer layerRC.Close()

	return io.ReadAll(layerRC)
}
