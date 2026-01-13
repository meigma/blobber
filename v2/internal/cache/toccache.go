package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/opencontainers/go-digest"

	"github.com/meigma/blobber/v2/internal/estargz"
)

// TOCCache stores TOC bytes alongside file cache entries.
type TOCCache struct {
	fileCache FileCache
}

// NewTOCCache creates a TOC cache backed by a FileCache.
func NewTOCCache(fileCache FileCache) *TOCCache {
	if fileCache == nil {
		return nil
	}
	return &TOCCache{fileCache: fileCache}
}

var _ estargz.TOCCache = (*TOCCache)(nil)

type tocEntry struct {
	Digest string `json:"digest"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
}

// Load returns cached TOC bytes for the given digest.
func (c *TOCCache) Load(d digest.Digest) (estargz.TOCEntry, []byte, bool, error) {
	tocPath, metaPath, err := c.paths(d)
	if err != nil {
		return estargz.TOCEntry{}, nil, false, err
	}

	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return estargz.TOCEntry{}, nil, false, nil
		}
		return estargz.TOCEntry{}, nil, false, err
	}

	var meta tocEntry
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return estargz.TOCEntry{}, nil, false, err
	}

	tocBytes, err := os.ReadFile(tocPath)
	if err != nil {
		if os.IsNotExist(err) {
			return estargz.TOCEntry{}, nil, false, nil
		}
		return estargz.TOCEntry{}, nil, false, err
	}

	if meta.Size <= 0 || int64(len(tocBytes)) != meta.Size {
		return estargz.TOCEntry{}, nil, false, nil
	}

	return estargz.TOCEntry{Offset: meta.Offset, Size: meta.Size}, tocBytes, true, nil
}

// Store records TOC bytes for the given digest.
func (c *TOCCache) Store(d digest.Digest, entry estargz.TOCEntry, toc []byte) error {
	if entry.Size <= 0 {
		return fmt.Errorf("toc size must be positive")
	}
	if int64(len(toc)) != entry.Size {
		return fmt.Errorf("toc size mismatch: got %d want %d", len(toc), entry.Size)
	}

	tocPath, metaPath, err := c.paths(d)
	if err != nil {
		return err
	}

	meta := tocEntry{
		Digest: d.String(),
		Offset: entry.Offset,
		Size:   entry.Size,
	}

	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	if err := atomicWrite(tocPath, toc); err != nil {
		return err
	}
	return atomicWrite(metaPath, metaBytes)
}

// LoadFooter returns cached footer bytes for the given digest.
func (c *TOCCache) LoadFooter(d digest.Digest) ([]byte, bool, error) {
	footerPath, err := c.footerPath(d)
	if err != nil {
		return nil, false, err
	}

	footerBytes, err := os.ReadFile(footerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if len(footerBytes) == 0 {
		return nil, false, nil
	}
	return footerBytes, true, nil
}

// StoreFooter writes footer bytes for the given digest.
func (c *TOCCache) StoreFooter(d digest.Digest, footer []byte) error {
	if len(footer) == 0 {
		return fmt.Errorf("footer must not be empty")
	}

	footerPath, err := c.footerPath(d)
	if err != nil {
		return err
	}

	return atomicWrite(footerPath, footer)
}

func (c *TOCCache) paths(d digest.Digest) (string, string, error) {
	dir, err := c.fileCache.Dir(d)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(dir, "toc.bin"), filepath.Join(dir, "toc.json"), nil
}

func (c *TOCCache) footerPath(d digest.Digest) (string, error) {
	dir, err := c.fileCache.Dir(d)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "footer.bin"), nil
}
