package blobber

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	"oras.land/oras-go/v2/registry/remote/credentials"

	"github.com/meigma/blobber/core"
	"github.com/meigma/blobber/internal/archive"
	"github.com/meigma/blobber/internal/cache"
	"github.com/meigma/blobber/internal/registry"
	"github.com/meigma/blobber/internal/safepath"
)

// Client provides operations against OCI registries.
type Client struct {
	registry  Registry
	builder   ArchiveBuilder
	reader    ArchiveReader
	validator PathValidator
	logger    *slog.Logger

	// configuration passed to registry
	credStore credentials.Store
	plainHTTP bool
	userAgent string
	descCache bool

	// cache configuration (opt-in)
	cacheDir           string
	cache              *cache.Cache
	backgroundPrefetch bool
	lazyLoading        bool
	cacheTTL           time.Duration
	cacheVerifyOnRead  bool

	// signing configuration (opt-in)
	signer   Signer
	verifier Verifier
}

// NewClient creates a new blobber client.
//
// By default, credentials are resolved from Docker config (~/.docker/config.json)
// and credential helpers. Use WithCredentials or WithCredentialStore to override.
func NewClient(opts ...ClientOption) (*Client, error) {
	c := &Client{
		logger: slog.New(slog.DiscardHandler),
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	if c.cacheVerifyOnRead && c.lazyLoading {
		return nil, errors.New("cache verify on read is incompatible with lazy loading")
	}

	// Set up credential store if not provided
	if c.credStore == nil {
		store, err := registry.DefaultCredentialStore()
		if err != nil {
			return nil, fmt.Errorf("create credential store: %w", err)
		}
		c.credStore = store
	}

	// Wire up default implementations
	var regOpts []registry.Option
	if c.credStore != nil {
		regOpts = append(regOpts, registry.WithCredentialStore(c.credStore))
	}
	if c.plainHTTP {
		regOpts = append(regOpts, registry.WithPlainHTTP(true))
	}
	if c.userAgent != "" {
		regOpts = append(regOpts, registry.WithUserAgent(c.userAgent))
	}
	if c.descCache {
		regOpts = append(regOpts, registry.WithDescriptorCache(true))
	}

	c.registry = registry.New(regOpts...)
	c.builder = archive.NewBuilder(c.logger)
	c.reader = archive.NewReader()
	c.validator = safepath.NewValidator()

	// Initialize cache if configured
	if c.cacheDir != "" {
		cacheInstance, err := cache.New(c.cacheDir, c.registry, c.logger)
		if err != nil {
			return nil, fmt.Errorf("create cache: %w", err)
		}
		cacheInstance.SetVerifyOnRead(c.cacheVerifyOnRead)
		c.cache = cacheInstance
	}

	return c, nil
}

// OpenImage opens a remote image for reading.
// The ref must be fully qualified (e.g., "ghcr.io/org/repo:tag").
//
// The returned Image caches the eStargz reader for efficient multiple file access.
// The caller must call Image.Close when done to release resources.
//
// If a cache is configured (via WithCacheDir), the blob will be fetched from cache
// if available, or downloaded and cached for future use.
//
// If a verifier is configured (via WithVerifier), signatures are verified before
// returning. Returns ErrNoSignature if no signatures are found, or ErrSignatureInvalid
// if verification fails.
//
// The layer digest is verified while downloading for integrity.
func (c *Client) OpenImage(ctx context.Context, ref string) (*Image, error) {
	// Verify signature if verifier configured
	if c.verifier != nil {
		verifiedRef, err := c.verifySignature(ctx, ref)
		if err != nil {
			return nil, err
		}
		ref = verifiedRef
	}

	// Use cache if available
	if c.cache != nil {
		return c.openImageCached(ctx, ref)
	}

	// Resolve descriptor so we can verify the downloaded blob digest.
	desc, err := c.registry.ResolveLayer(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", ref, err)
	}

	// Pull the blob from registry directly
	blob, err := c.registry.FetchBlob(ctx, ref, desc)
	if err != nil {
		return nil, fmt.Errorf("pull %s: %w", ref, err)
	}
	defer blob.Close()

	// Create Image with cached reader
	img, err := newImageFromBlobWithDigest(ref, blob, desc.Size, desc.Digest, c.validator, c.logger)
	if err != nil {
		return nil, fmt.Errorf("open image %s: %w", ref, err)
	}

	return img, nil
}

// openImageCached opens an image using the cache.
func (c *Client) openImageCached(ctx context.Context, ref string) (*Image, error) {
	var desc LayerDescriptor
	var err error

	// Try TTL-based resolution first
	if c.cacheTTL > 0 {
		if cachedDesc, ok := c.cache.LookupByRef(ref, c.cacheTTL); ok {
			if c.hasCachedBlob(cachedDesc) {
				c.logger.Debug("using TTL-cached descriptor", "ref", ref, "digest", cachedDesc.Digest)
				desc = cachedDesc
			}
		}
	}

	// If no valid TTL cache hit, resolve from registry
	if desc.Digest == "" {
		desc, err = c.registry.ResolveLayer(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", ref, err)
		}
		// Update the reference index
		c.cache.UpdateRefIndex(ref, desc)
	}

	// Get blob handle from cache
	var handle BlobHandle
	if c.lazyLoading {
		// Lazy loading: fetch bytes on-demand via ReadAt
		handle, err = c.cache.OpenLazy(ctx, ref, desc)
	} else {
		// Eager loading: download entire blob upfront
		handle, err = c.cache.Open(ctx, ref, desc)
	}
	if err != nil {
		return nil, fmt.Errorf("open cached blob %s: %w", ref, err)
	}

	// Start background prefetch if enabled and blob is not complete
	if c.backgroundPrefetch && !handle.Complete() {
		c.cache.Prefetch(ctx, ref, desc)
	}

	// Create Image from the cached handle
	img, err := NewImageFromHandle(ref, handle, c.validator, c.logger)
	if err != nil {
		handle.Close()
		return nil, fmt.Errorf("open image %s: %w", ref, err)
	}

	return img, nil
}

// hasCachedBlob checks if a blob with the given descriptor is fully cached.
// This verifies both the entry metadata AND that the blob file exists with correct size.
func (c *Client) hasCachedBlob(desc LayerDescriptor) bool {
	entry, blobPath, _ := c.cache.LoadCompleteEntry(desc.Digest)
	if entry == nil {
		return false
	}

	// Verify blob file exists and has expected size
	info, err := os.Stat(blobPath)
	if err != nil {
		c.logger.Debug("TTL cache hit but blob file missing", "digest", desc.Digest, "error", err)
		return false
	}
	if info.Size() != entry.Size {
		c.logger.Debug("TTL cache hit but blob size mismatch", "digest", desc.Digest, "expected", entry.Size, "actual", info.Size())
		return false
	}

	return true
}

// verifySignature verifies that at least one valid signature exists for the image.
// For multi-arch images, this checks signatures on both the platform manifest (blobber's
// signing approach) and the OCI index (cosign's default). Verification succeeds if at
// least one valid signature is found on either.
// Returns a digest reference pinned to the verified platform manifest.
func (c *Client) verifySignature(ctx context.Context, ref string) (string, error) {
	// Fetch the top-level manifest (may be an OCI index for multi-arch)
	indexBytes, indexDigest, err := c.registry.FetchManifest(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("fetch manifest for %s: %w", ref, err)
	}

	// Resolve to the platform-specific manifest
	pinnedIndexRef := digestReference(ref, indexDigest)
	layerDesc, err := c.registry.ResolveLayer(ctx, pinnedIndexRef)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", ref, err)
	}

	platformDigest := layerDesc.ManifestDigest
	if platformDigest == "" {
		return "", fmt.Errorf("no manifest digest for %s", ref)
	}

	// Build list of manifests to check for signatures.
	// For single-arch, both digests are the same. For multi-arch, we check both
	// the platform manifest (blobber signs here) and the index (cosign signs here).
	type manifestToCheck struct {
		digest string
		bytes  []byte
	}

	// Fetch platform manifest bytes if needed
	platformRef := digestReference(ref, platformDigest)
	var platformBytes []byte
	if platformDigest == indexDigest {
		platformBytes = indexBytes
	} else {
		var fetchErr error
		platformBytes, _, fetchErr = c.registry.FetchManifest(ctx, platformRef)
		if fetchErr != nil {
			return "", fmt.Errorf("fetch platform manifest for %s: %w", ref, fetchErr)
		}
	}

	manifests := []manifestToCheck{
		{digest: platformDigest, bytes: platformBytes},
	}

	// Add index if different from platform manifest (multi-arch case)
	if indexDigest != platformDigest {
		manifests = append(manifests, manifestToCheck{
			digest: indexDigest,
			bytes:  indexBytes,
		})
	}

	// Try to verify signatures on each manifest
	var lastErr error
	for _, m := range manifests {
		if err := c.verifyManifestSignature(ctx, ref, m.digest, m.bytes); err == nil {
			return digestReference(ref, platformDigest), nil // Success
		} else if !errors.Is(err, ErrNoSignature) {
			lastErr = err
		}
	}

	// No valid signatures found on any manifest
	if lastErr != nil {
		return "", fmt.Errorf("%w: %v", ErrSignatureInvalid, lastErr)
	}
	return "", ErrNoSignature
}

// verifyManifestSignature verifies signatures attached to a specific manifest digest.
func (c *Client) verifyManifestSignature(ctx context.Context, ref, manifestDigest string, manifestBytes []byte) error {
	// Fetch all referrers (signatures, SBOMs, attestations, etc.)
	referrers, err := c.registry.FetchReferrers(ctx, ref, manifestDigest, "")
	if err != nil {
		return fmt.Errorf("fetching referrers: %w", err)
	}

	// Filter out known non-signature referrers (SBOMs, attestations)
	// This prevents them from being treated as failed signature attempts.
	// Unknown types are passed through to support custom signers.
	var signatureReferrers []core.Referrer
	for _, r := range referrers {
		if !IsNonSignatureArtifactType(r.ArtifactType) {
			signatureReferrers = append(signatureReferrers, r)
		}
	}

	if len(signatureReferrers) == 0 {
		return ErrNoSignature
	}

	// Try to verify at least one signature
	var lastErr error
	for _, referrer := range signatureReferrers {
		sigData, fetchErr := c.registry.FetchReferrer(ctx, ref, referrer.Digest)
		if fetchErr != nil {
			lastErr = fetchErr
			continue
		}

		sig := &Signature{
			Data:      sigData,
			MediaType: referrer.ArtifactType,
		}

		d, parseErr := digest.Parse(manifestDigest)
		if parseErr != nil {
			lastErr = parseErr
			continue
		}

		if verifyErr := c.verifier.Verify(ctx, d, manifestBytes, sig); verifyErr != nil {
			lastErr = verifyErr
			continue
		}

		// At least one signature verified successfully
		return nil
	}

	// All signatures failed verification
	if lastErr != nil {
		return fmt.Errorf("%w: %v", ErrSignatureInvalid, lastErr)
	}
	return ErrSignatureInvalid
}

// digestReference constructs a digest reference from a tag reference.
// Example: "ghcr.io/org/repo:tag" + "sha256:abc..." -> "ghcr.io/org/repo@sha256:abc..."
func digestReference(ref, manifestDigest string) string {
	// Find the repository part (everything before : or @)
	var repo string
	if idx := strings.LastIndex(ref, "@"); idx != -1 {
		repo = ref[:idx]
	} else if idx := strings.LastIndex(ref, ":"); idx != -1 {
		// Handle port numbers by finding the last : after the last /
		lastSlash := strings.LastIndex(ref, "/")
		if lastSlash != -1 && idx > lastSlash {
			repo = ref[:idx]
		} else {
			// No tag, just use the whole ref
			repo = ref
		}
	} else {
		repo = ref
	}
	return repo + "@" + manifestDigest
}
