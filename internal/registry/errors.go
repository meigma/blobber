package registry

import (
	"errors"
	"net/http"

	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"

	"github.com/gilmanlab/blobber/core"
)

// Sentinel errors for registry operations.
var (
	// ErrRangeNotSupported is an alias to core.ErrRangeNotSupported for use within this package.
	ErrRangeNotSupported = core.ErrRangeNotSupported

	// ErrMultipleLayers indicates the manifest has multiple layers, which is unexpected for blobber images.
	ErrMultipleLayers = errors.New("manifest has multiple layers; blobber expects exactly one layer")
)

// mapError converts ORAS registry errors to blobber sentinel errors.
func mapError(err error) error {
	if err == nil {
		return nil
	}

	// Check for ORAS errdef sentinel errors first.
	if errors.Is(err, errdef.ErrNotFound) {
		return core.ErrNotFound
	}

	var errResp *errcode.ErrorResponse
	if errors.As(err, &errResp) {
		// Check HTTP status code first
		switch errResp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return core.ErrUnauthorized
		case http.StatusNotFound:
			return core.ErrNotFound
		}

		// Check specific error codes
		for _, e := range errResp.Errors {
			switch e.Code {
			case errcode.ErrorCodeUnauthorized, errcode.ErrorCodeDenied:
				return core.ErrUnauthorized
			case errcode.ErrorCodeNameUnknown,
				errcode.ErrorCodeManifestUnknown,
				errcode.ErrorCodeBlobUnknown:
				return core.ErrNotFound
			}
		}
	}

	return err
}
