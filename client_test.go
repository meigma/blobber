package blobber

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDigestReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ref            string
		manifestDigest string
		want           string
	}{
		{
			name:           "tag reference",
			ref:            "ghcr.io/org/repo:v1.0.0",
			manifestDigest: "sha256:abc123",
			want:           "ghcr.io/org/repo@sha256:abc123",
		},
		{
			name:           "digest reference",
			ref:            "ghcr.io/org/repo@sha256:old",
			manifestDigest: "sha256:new",
			want:           "ghcr.io/org/repo@sha256:new",
		},
		{
			name:           "localhost with port and tag",
			ref:            "localhost:5000/repo:latest",
			manifestDigest: "sha256:abc",
			want:           "localhost:5000/repo@sha256:abc",
		},
		{
			name:           "localhost with port no tag",
			ref:            "localhost:5000/repo",
			manifestDigest: "sha256:abc",
			want:           "localhost:5000/repo@sha256:abc",
		},
		{
			name:           "no tag or digest",
			ref:            "docker.io/library/alpine",
			manifestDigest: "sha256:xyz",
			want:           "docker.io/library/alpine@sha256:xyz",
		},
		{
			name:           "registry with port and nested repo",
			ref:            "myregistry.com:8443/org/repo/subdir:tag",
			manifestDigest: "sha256:def",
			want:           "myregistry.com:8443/org/repo/subdir@sha256:def",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := digestReference(tt.ref, tt.manifestDigest)
			assert.Equal(t, tt.want, got)
		})
	}
}
