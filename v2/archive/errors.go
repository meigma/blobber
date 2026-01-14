package archive

import "errors"

var (
	// ErrInvalidMagic indicates the index header magic does not match.
	ErrInvalidMagic = errors.New("invalid index magic")
	// ErrUnsupportedVersion indicates the index version is not supported.
	ErrUnsupportedVersion = errors.New("unsupported index version")
	// ErrInvalidHeader indicates the index header is malformed.
	ErrInvalidHeader = errors.New("invalid index header")
	// ErrInvalidChecksum indicates the index checksum does not match the body.
	ErrInvalidChecksum = errors.New("invalid index checksum")
	// ErrInvalidIndex indicates the index contents are malformed.
	ErrInvalidIndex = errors.New("invalid index contents")
	// ErrInvalidPath indicates a path is not normalized or is unsafe.
	ErrInvalidPath = errors.New("invalid path")
	// ErrDuplicatePath indicates a duplicate path was added.
	ErrDuplicatePath = errors.New("duplicate path")
	// ErrUnsupportedCompression indicates an unsupported compression algorithm.
	ErrUnsupportedCompression = errors.New("unsupported compression")
)
