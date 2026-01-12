package oci

import (
	"errors"
	"net/http"

	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

// Sentinel errors for OCI operations.
var (
	// ErrNotFound indicates the requested resource does not exist.
	ErrNotFound = errors.New("not found")

	// ErrUnauthorized indicates authentication or authorization failure.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrInvalidReference indicates the reference string is malformed.
	ErrInvalidReference = errors.New("invalid reference")

	// ErrRangeNotSupported indicates the registry does not support range requests.
	ErrRangeNotSupported = errors.New("range requests not supported")

	// ErrClosed indicates the resource has been closed.
	ErrClosed = errors.New("closed")
)

// mapError converts ORAS errors to domain sentinel errors.
func mapError(err error) error {
	if err == nil {
		return nil
	}

	// Check ORAS sentinel errors.
	if errors.Is(err, errdef.ErrNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, errdef.ErrInvalidReference) || errors.Is(err, errdef.ErrMissingReference) {
		return ErrInvalidReference
	}

	// Check HTTP error responses.
	var errResp *errcode.ErrorResponse
	if errors.As(err, &errResp) {
		switch errResp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return ErrUnauthorized
		case http.StatusNotFound:
			return ErrNotFound
		}

		// Check registry-specific error codes.
		for _, e := range errResp.Errors {
			switch e.Code {
			case errcode.ErrorCodeUnauthorized, errcode.ErrorCodeDenied:
				return ErrUnauthorized
			case errcode.ErrorCodeNameUnknown,
				errcode.ErrorCodeManifestUnknown,
				errcode.ErrorCodeBlobUnknown:
				return ErrNotFound
			}
		}
	}

	return err
}
