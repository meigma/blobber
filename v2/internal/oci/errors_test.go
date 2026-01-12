package oci

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

func TestMapError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected error
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: nil,
		},
		{
			name:     "ORAS ErrNotFound",
			err:      errdef.ErrNotFound,
			expected: ErrNotFound,
		},
		{
			name:     "wrapped ORAS ErrNotFound",
			err:      errors.Join(errors.New("context"), errdef.ErrNotFound),
			expected: ErrNotFound,
		},
		{
			name: "HTTP 401 Unauthorized",
			err: &errcode.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
			},
			expected: ErrUnauthorized,
		},
		{
			name: "HTTP 403 Forbidden",
			err: &errcode.ErrorResponse{
				StatusCode: http.StatusForbidden,
			},
			expected: ErrUnauthorized,
		},
		{
			name: "HTTP 404 Not Found",
			err: &errcode.ErrorResponse{
				StatusCode: http.StatusNotFound,
			},
			expected: ErrNotFound,
		},
		{
			name: "registry error code UNAUTHORIZED",
			err: &errcode.ErrorResponse{
				StatusCode: http.StatusOK,
				Errors: []errcode.Error{
					{Code: errcode.ErrorCodeUnauthorized},
				},
			},
			expected: ErrUnauthorized,
		},
		{
			name: "registry error code DENIED",
			err: &errcode.ErrorResponse{
				StatusCode: http.StatusOK,
				Errors: []errcode.Error{
					{Code: errcode.ErrorCodeDenied},
				},
			},
			expected: ErrUnauthorized,
		},
		{
			name: "registry error code NAME_UNKNOWN",
			err: &errcode.ErrorResponse{
				StatusCode: http.StatusOK,
				Errors: []errcode.Error{
					{Code: errcode.ErrorCodeNameUnknown},
				},
			},
			expected: ErrNotFound,
		},
		{
			name: "registry error code MANIFEST_UNKNOWN",
			err: &errcode.ErrorResponse{
				StatusCode: http.StatusOK,
				Errors: []errcode.Error{
					{Code: errcode.ErrorCodeManifestUnknown},
				},
			},
			expected: ErrNotFound,
		},
		{
			name: "registry error code BLOB_UNKNOWN",
			err: &errcode.ErrorResponse{
				StatusCode: http.StatusOK,
				Errors: []errcode.Error{
					{Code: errcode.ErrorCodeBlobUnknown},
				},
			},
			expected: ErrNotFound,
		},
		{
			name:     "unknown error passed through",
			err:      errors.New("some other error"),
			expected: errors.New("some other error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := mapError(tt.err)

			if tt.expected == nil {
				assert.Nil(t, result)
				return
			}

			// For sentinel errors, check identity.
			if errors.Is(tt.expected, ErrNotFound) || errors.Is(tt.expected, ErrUnauthorized) {
				assert.ErrorIs(t, result, tt.expected)
				return
			}

			// For pass-through errors, check the error is returned as-is.
			assert.Equal(t, tt.err, result)
		})
	}
}
