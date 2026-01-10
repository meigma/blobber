package blobber

import (
	"context"
	"io"
	"io/fs"

	"github.com/meigma/blobber/core"
)

type registry interface {
	Push(ctx context.Context, ref string, layer io.Reader, opts *core.RegistryPushOptions) (string, error)
	ResolveLayer(ctx context.Context, ref string) (core.LayerDescriptor, error)
	FetchBlob(ctx context.Context, ref string, desc core.LayerDescriptor) (io.ReadCloser, error)
	FetchBlobRange(ctx context.Context, ref string, desc core.LayerDescriptor, offset, length int64) (io.ReadCloser, error)
	PushReferrer(ctx context.Context, ref, subjectDigest string, data []byte, opts *core.ReferrerPushOptions) (string, error)
	FetchReferrers(ctx context.Context, ref, subjectDigest, artifactType string) ([]core.Referrer, error)
	FetchReferrer(ctx context.Context, ref, referrerDigest string) ([]byte, error)
	FetchManifest(ctx context.Context, ref string) ([]byte, string, error)
	ListTags(ctx context.Context, repository string) ([]string, error)
}

type archiveBuilder interface {
	Build(ctx context.Context, src fs.FS, compression core.Compression) (*core.BuildResult, error)
}

type archiveReader interface {
	ReadTOC(ra io.ReaderAt, size int64) (*core.TOC, error)
	OpenFile(ra io.ReaderAt, size int64, entry core.TOCEntry) (io.Reader, error)
}

type pathValidator interface {
	ValidatePath(path string) error
	ValidateExtraction(destDir string, entries []core.TOCEntry, limits core.ExtractLimits) error
	ValidateSymlink(destDir, linkPath, target string) error
}
