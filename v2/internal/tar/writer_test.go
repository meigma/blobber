package tar

import (
	"archive/tar"
	"bytes"
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWriter(t *testing.T) {
	t.Parallel()

	t.Run("returns Writer interface", func(t *testing.T) {
		t.Parallel()

		w := NewWriter()
		require.NotNil(t, w)
	})

	t.Run("accepts logger option", func(t *testing.T) {
		t.Parallel()

		w := NewWriter(WithWriterLogger(nil))
		require.NotNil(t, w)
	})
}

func TestWriteTo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		src         fs.FS
		wantNames   []string
		wantContent map[string]string // name -> content for files
	}{
		{
			name:      "empty filesystem",
			src:       fstest.MapFS{},
			wantNames: nil,
		},
		{
			name: "single file",
			src: fstest.MapFS{
				"hello.txt": &fstest.MapFile{
					Data:    []byte("hello world"),
					Mode:    0o644,
					ModTime: time.Now(),
				},
			},
			wantNames:   []string{"hello.txt"},
			wantContent: map[string]string{"hello.txt": "hello world"},
		},
		{
			name: "nested structure",
			src: fstest.MapFS{
				"dir":              &fstest.MapFile{Mode: fs.ModeDir | 0o755},
				"dir/subdir":       &fstest.MapFile{Mode: fs.ModeDir | 0o755},
				"dir/subdir/a.txt": &fstest.MapFile{Data: []byte("a"), Mode: 0o644},
				"dir/b.txt":        &fstest.MapFile{Data: []byte("b"), Mode: 0o644},
				"root.txt":         &fstest.MapFile{Data: []byte("root"), Mode: 0o644},
			},
			wantNames: []string{"dir", "dir/subdir", "dir/subdir/a.txt", "dir/b.txt", "root.txt"},
			wantContent: map[string]string{
				"dir/subdir/a.txt": "a",
				"dir/b.txt":        "b",
				"root.txt":         "root",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			w := NewWriter()
			err := w.WriteTo(context.Background(), &buf, tt.src)
			require.NoError(t, err)

			entries := readTarEntries(t, &buf)
			names := extractNames(entries)

			if tt.wantNames == nil {
				assert.Empty(t, entries)
			} else {
				for _, wantName := range tt.wantNames {
					assert.Contains(t, names, wantName)
				}
			}

			for name, wantContent := range tt.wantContent {
				for _, e := range entries {
					if e.name == name {
						assert.Equal(t, wantContent, e.content)
						break
					}
				}
			}
		})
	}
}

func TestWriteTo_ContextCancellation(t *testing.T) {
	t.Parallel()

	src := fstest.MapFS{
		"file.txt": &fstest.MapFile{Data: []byte("data"), Mode: 0o644},
	}
	var buf bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	w := NewWriter()
	err := w.WriteTo(ctx, &buf, src)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestWriteTo_LargeFile(t *testing.T) {
	t.Parallel()

	content := bytes.Repeat([]byte("x"), 256*1024) // 256KB
	src := fstest.MapFS{
		"large.bin": &fstest.MapFile{Data: content, Mode: 0o644},
	}
	var buf bytes.Buffer

	w := NewWriter()
	err := w.WriteTo(context.Background(), &buf, src)
	require.NoError(t, err)

	entries := readTarEntries(t, &buf)
	require.Len(t, entries, 1)
	assert.Equal(t, content, []byte(entries[0].content))
}

func TestWriteTo_Symlinks(t *testing.T) {
	t.Parallel()

	t.Run("symlink entry", func(t *testing.T) {
		t.Parallel()

		src := &symlinkFS{
			files: map[string]fileEntry{
				"target.txt": {data: []byte("target content"), mode: 0o644},
				"link":       {mode: fs.ModeSymlink, linkTarget: "target.txt"},
			},
		}
		var buf bytes.Buffer

		w := NewWriter()
		err := w.WriteTo(context.Background(), &buf, src)
		require.NoError(t, err)

		entries := readTarEntries(t, &buf)
		require.Len(t, entries, 2)

		// Find the symlink entry.
		var linkEntry *tarEntry
		for i := range entries {
			if entries[i].name == "link" {
				linkEntry = &entries[i]
				break
			}
		}
		require.NotNil(t, linkEntry, "symlink entry not found")
		assert.Equal(t, byte(tar.TypeSymlink), linkEntry.typeflag)
		assert.Equal(t, "target.txt", linkEntry.linkname)
	})

	t.Run("error without ReadLink support", func(t *testing.T) {
		t.Parallel()

		// Use a filesystem that reports symlinks but doesn't support ReadLink.
		src := &noReadLinkFS{
			files: map[string]fileEntry{
				"link": {mode: fs.ModeSymlink, linkTarget: "target"},
			},
		}
		var buf bytes.Buffer

		w := NewWriter()
		err := w.WriteTo(context.Background(), &buf, src)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ReadLink")
	})

	t.Run("symlink type without ReadLink support", func(t *testing.T) {
		t.Parallel()

		src := &symlinkTypeOnlyFS{}
		var buf bytes.Buffer

		w := NewWriter()
		err := w.WriteTo(context.Background(), &buf, src)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ReadLink")
		assert.False(t, src.opened, "expected not to open symlink target")
	})
}
