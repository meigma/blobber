package oci

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantErr    error
		registry   string
		repository string
		tag        string
		digest     string
	}{
		{
			name:       "full reference with tag",
			input:      "ghcr.io/myorg/myrepo:v1.0.0",
			registry:   "ghcr.io",
			repository: "myorg/myrepo",
			tag:        "v1.0.0",
			digest:     "",
		},
		{
			name:       "full reference with digest",
			input:      "ghcr.io/myorg/myrepo@sha256:abcd1234567890abcd1234567890abcd1234567890abcd1234567890abcd1234",
			registry:   "ghcr.io",
			repository: "myorg/myrepo",
			tag:        "",
			digest:     "sha256:abcd1234567890abcd1234567890abcd1234567890abcd1234567890abcd1234",
		},
		{
			name:       "docker hub library image",
			input:      "docker.io/library/alpine:latest",
			registry:   "docker.io",
			repository: "library/alpine",
			tag:        "latest",
			digest:     "",
		},
		{
			name:       "registry with port",
			input:      "localhost:5000/myrepo:tag",
			registry:   "localhost:5000",
			repository: "myrepo",
			tag:        "tag",
			digest:     "",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: ErrInvalidReference,
		},
		{
			name:    "invalid reference",
			input:   "not a valid reference!!!",
			wantErr: ErrInvalidReference,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ref, err := ParseReference(tt.input)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.registry, ref.Registry())
			assert.Equal(t, tt.repository, ref.Repository())
			assert.Equal(t, tt.tag, ref.Tag())
			assert.Equal(t, tt.digest, ref.Digest())
		})
	}
}

func TestReference_String(t *testing.T) {
	t.Parallel()

	ref, err := ParseReference("ghcr.io/myorg/myrepo:v1.0.0")
	require.NoError(t, err)

	assert.Equal(t, "ghcr.io/myorg/myrepo:v1.0.0", ref.String())
}

func TestReference_RepositoryReference(t *testing.T) {
	t.Parallel()

	ref, err := ParseReference("ghcr.io/myorg/myrepo:v1.0.0")
	require.NoError(t, err)

	assert.Equal(t, "ghcr.io/myorg/myrepo", ref.RepositoryReference())
}

func TestIsDigest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected bool
	}{
		{"sha256:abcd1234", true},
		{"sha512:abcd1234", true},
		{"v1.0.0", false},
		{"latest", false},
		{"", false},
		{":", false},
		{":abc", false},
		{"abc:", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, isDigest(tt.input))
		})
	}
}
