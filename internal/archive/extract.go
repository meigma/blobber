package archive

import (
	"context"
	"io"

	"github.com/gilmanlab/blobber"
)

// Extract extracts an eStargz blob to the destination directory.
func Extract(ctx context.Context, r io.Reader, destDir string, validator blobber.PathValidator, limits blobber.ExtractLimits) error {
	panic("not implemented")
}
