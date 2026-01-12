package oci

import (
	"oras.land/oras-go/v2/registry"
)

// Reference represents a parsed OCI reference.
//
// An OCI reference identifies a resource in a registry and has the form:
//
//	[registry/]repository[:tag|@digest]
//
// Examples:
//   - ghcr.io/myorg/myrepo:v1.0.0
//   - docker.io/library/alpine@sha256:abc123...
type Reference struct {
	ref registry.Reference
}

// ParseReference parses and validates an OCI reference string.
// Returns ErrInvalidReference if the reference is malformed.
func ParseReference(s string) (Reference, error) {
	ref, err := registry.ParseReference(s)
	if err != nil {
		return Reference{}, ErrInvalidReference
	}
	return Reference{ref: ref}, nil
}

// Registry returns the registry host (e.g., "ghcr.io").
func (r Reference) Registry() string {
	return r.ref.Registry
}

// Repository returns the repository path (e.g., "myorg/myrepo").
func (r Reference) Repository() string {
	return r.ref.Repository
}

// Tag returns the tag if present, empty string otherwise.
// Note: A reference has either a tag or a digest, not both.
func (r Reference) Tag() string {
	// ORAS stores tag in Reference field when it's a tag (not digest)
	if r.ref.Reference != "" && !isDigest(r.ref.Reference) {
		return r.ref.Reference
	}
	return ""
}

// Digest returns the digest if present, empty string otherwise.
// Note: A reference has either a tag or a digest, not both.
func (r Reference) Digest() string {
	if r.ref.Reference != "" && isDigest(r.ref.Reference) {
		return r.ref.Reference
	}
	return ""
}

// String returns the full reference string.
func (r Reference) String() string {
	return r.ref.String()
}

// RepositoryReference returns "registry/repository" without tag or digest.
func (r Reference) RepositoryReference() string {
	return r.ref.Registry + "/" + r.ref.Repository
}

// isDigest reports whether s looks like a digest (contains algorithm prefix).
func isDigest(s string) bool {
	// Digests have the form "algorithm:hex", e.g., "sha256:abc123..."
	for i := range len(s) {
		if s[i] == ':' {
			return i > 0 && i < len(s)-1
		}
		if s[i] == '@' {
			return false
		}
	}
	return false
}
