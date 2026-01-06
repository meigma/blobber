package archive

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Compile-time interface implementation checks.
var (
	_ fs.FS        = (*osFS)(nil)
	_ fs.ReadDirFS = (*osFS)(nil)
	_ fs.StatFS    = (*osFS)(nil)
	_ lstatFS      = (*osFS)(nil)
	_ readLinkFS   = (*osFS)(nil)
)

// OSFS returns a filesystem rooted at the given directory path.
// Unlike os.DirFS, it implements ReadLink and Lstat for proper symlink handling.
func OSFS(root string) *osFS {
	return &osFS{root: root}
}

// osFS is an fs.FS implementation backed by the OS filesystem.
// It supports symlinks via ReadLink and Lstat.
type osFS struct {
	root string
}

// Open implements fs.FS.
//
//nolint:gosec // G304: Path is validated by fs.ValidPath and rooted to o.root
func (o *osFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return os.Open(filepath.Join(o.root, name))
}

// ReadDir implements fs.ReadDirFS.
func (o *osFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	return os.ReadDir(filepath.Join(o.root, name))
}

// ReadLink returns the destination of the named symbolic link.
func (o *osFS) ReadLink(name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrInvalid}
	}
	return os.Readlink(filepath.Join(o.root, name))
}

// Lstat returns FileInfo for the named file without following symlinks.
func (o *osFS) Lstat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: fs.ErrInvalid}
	}
	return os.Lstat(filepath.Join(o.root, name))
}

// Stat implements fs.StatFS.
func (o *osFS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	return os.Stat(filepath.Join(o.root, name))
}
