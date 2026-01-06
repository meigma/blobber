package registry

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"

	"github.com/gilmanlab/blobber/core"
)

func TestMapError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		want    error
		wantNil bool
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			wantNil: true,
		},
		{
			name: "401 status returns ErrUnauthorized",
			err: &errcode.ErrorResponse{
				Method:     http.MethodGet,
				URL:        &url.URL{Path: "/v2/test/manifests/latest"},
				StatusCode: http.StatusUnauthorized,
			},
			want: core.ErrUnauthorized,
		},
		{
			name: "403 status returns ErrUnauthorized",
			err: &errcode.ErrorResponse{
				Method:     http.MethodGet,
				URL:        &url.URL{Path: "/v2/test/manifests/latest"},
				StatusCode: http.StatusForbidden,
			},
			want: core.ErrUnauthorized,
		},
		{
			name: "404 status returns ErrNotFound",
			err: &errcode.ErrorResponse{
				Method:     http.MethodGet,
				URL:        &url.URL{Path: "/v2/test/manifests/latest"},
				StatusCode: http.StatusNotFound,
			},
			want: core.ErrNotFound,
		},
		{
			name: "UNAUTHORIZED error code returns ErrUnauthorized",
			err: &errcode.ErrorResponse{
				Method:     http.MethodGet,
				URL:        &url.URL{Path: "/v2/test/manifests/latest"},
				StatusCode: http.StatusOK,
				Errors: errcode.Errors{
					{Code: errcode.ErrorCodeUnauthorized, Message: "access denied"},
				},
			},
			want: core.ErrUnauthorized,
		},
		{
			name: "DENIED error code returns ErrUnauthorized",
			err: &errcode.ErrorResponse{
				Method:     http.MethodGet,
				URL:        &url.URL{Path: "/v2/test/manifests/latest"},
				StatusCode: http.StatusOK,
				Errors: errcode.Errors{
					{Code: errcode.ErrorCodeDenied, Message: "access denied"},
				},
			},
			want: core.ErrUnauthorized,
		},
		{
			name: "NAME_UNKNOWN error code returns ErrNotFound",
			err: &errcode.ErrorResponse{
				Method:     http.MethodGet,
				URL:        &url.URL{Path: "/v2/test/manifests/latest"},
				StatusCode: http.StatusOK,
				Errors: errcode.Errors{
					{Code: errcode.ErrorCodeNameUnknown, Message: "repository not found"},
				},
			},
			want: core.ErrNotFound,
		},
		{
			name: "MANIFEST_UNKNOWN error code returns ErrNotFound",
			err: &errcode.ErrorResponse{
				Method:     http.MethodGet,
				URL:        &url.URL{Path: "/v2/test/manifests/latest"},
				StatusCode: http.StatusOK,
				Errors: errcode.Errors{
					{Code: errcode.ErrorCodeManifestUnknown, Message: "manifest not found"},
				},
			},
			want: core.ErrNotFound,
		},
		{
			name: "BLOB_UNKNOWN error code returns ErrNotFound",
			err: &errcode.ErrorResponse{
				Method:     http.MethodGet,
				URL:        &url.URL{Path: "/v2/test/blobs/sha256:abc"},
				StatusCode: http.StatusOK,
				Errors: errcode.Errors{
					{Code: errcode.ErrorCodeBlobUnknown, Message: "blob not found"},
				},
			},
			want: core.ErrNotFound,
		},
		{
			name: "unknown error code returns original error",
			err: &errcode.ErrorResponse{
				Method:     http.MethodGet,
				URL:        &url.URL{Path: "/v2/test/manifests/latest"},
				StatusCode: http.StatusInternalServerError,
				Errors: errcode.Errors{
					{Code: "UNKNOWN", Message: "something went wrong"},
				},
			},
			want: nil, // will check it returns the original
		},
		{
			name: "non-ErrorResponse error returns original",
			err:  errors.New("some other error"),
			want: nil, // will check it returns the original
		},
		{
			name: "errdef.ErrNotFound returns ErrNotFound",
			err:  errdef.ErrNotFound,
			want: core.ErrNotFound,
		},
		{
			name: "wrapped errdef.ErrNotFound returns ErrNotFound",
			err:  fmt.Errorf("fetch failed: %w", errdef.ErrNotFound),
			want: core.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := mapError(tt.err)

			if tt.wantNil {
				assert.Nil(t, got)
				return
			}

			if tt.want != nil {
				assert.ErrorIs(t, got, tt.want)
			} else {
				// Should return original error unchanged
				assert.Equal(t, tt.err, got)
			}
		})
	}
}
