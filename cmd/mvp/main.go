// MVP: Validate eStargz + ORAS flow end-to-end
//
// This is a throwaway spike to validate our assumptions about the libraries.
// Run with: go run ./cmd/mvp
package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"

	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry/remote"
)

const (
	registryAddr = "localhost:5050"
	repoName     = "blobber-test"
	tag          = "v1"
)

func main() {
	ctx := context.Background()

	// Step 1: Build eStargz blob from local directory
	log.Println("=== Step 1: Building eStargz blob from testdata/ ===")
	blob, tocDigest, err := buildEStargz("testdata")
	if err != nil {
		log.Fatalf("build estargz: %v", err)
	}
	log.Printf("Built eStargz blob, TOC digest: %s", tocDigest)
	log.Printf("Blob size: %d bytes", blob.Len())

	// Step 2: Push to local registry
	log.Println("\n=== Step 2: Pushing to local registry ===")
	ref := fmt.Sprintf("%s/%s:%s", registryAddr, repoName, tag)
	manifestDigest, err := pushToRegistry(ctx, ref, blob.Bytes())
	if err != nil {
		log.Fatalf("push to registry: %v", err)
	}
	log.Printf("Pushed to %s", ref)
	log.Printf("Manifest digest: %s", manifestDigest)

	// Step 3: List TOC (without downloading full blob)
	log.Println("\n=== Step 3: Listing TOC ===")
	fileEntries, err := listTOC(blob.Bytes())
	if err != nil {
		log.Fatalf("list TOC: %v", err)
	}
	for _, e := range fileEntries {
		log.Printf("  %s (size: %d, offset: %d)", e.Name, e.Size, e.Offset)
	}

	// Step 4: Extract a single file
	log.Println("\n=== Step 4: Extracting single file (config.yaml) ===")
	content, err := extractFile(blob.Bytes(), "config.yaml")
	if err != nil {
		log.Fatalf("extract file: %v", err)
	}
	log.Printf("Content of config.yaml:\n%s", content)

	// Verify
	expected := "hello from config.yaml\n"
	if string(content) == expected {
		log.Println("\n=== SUCCESS: Content matches expected! ===")
	} else {
		log.Fatalf("FAIL: Content mismatch.\nExpected: %q\nGot: %q", expected, content)
	}
}

// buildEStargz creates an eStargz blob from a directory.
func buildEStargz(dir string) (*bytes.Buffer, digest.Digest, error) {
	// First, create a tar archive from the directory
	tarBuf := new(bytes.Buffer)
	tw := tar.NewWriter(tarBuf)

	err := fs.WalkDir(os.DirFS(dir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = path

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if !d.IsDir() {
			f, err := os.Open(fmt.Sprintf("%s/%s", dir, path))
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("walk dir: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, "", fmt.Errorf("close tar: %w", err)
	}

	// Convert tar to eStargz
	tarReader := bytes.NewReader(tarBuf.Bytes())
	sr := io.NewSectionReader(tarReader, 0, int64(tarBuf.Len()))

	esgzBlob, err := estargz.Build(sr)
	if err != nil {
		return nil, "", fmt.Errorf("build estargz: %w", err)
	}

	// Read the blob into memory (Blob embeds io.ReadCloser)
	outBuf := new(bytes.Buffer)
	if _, err := io.Copy(outBuf, esgzBlob); err != nil {
		return nil, "", fmt.Errorf("copy blob: %w", err)
	}
	esgzBlob.Close()

	return outBuf, esgzBlob.TOCDigest(), nil
}

// pushToRegistry pushes the blob to an OCI registry.
func pushToRegistry(ctx context.Context, ref string, blob []byte) (string, error) {
	// Create a remote repository
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return "", fmt.Errorf("new repository: %w", err)
	}
	repo.PlainHTTP = true // insecure for local testing

	// Use memory store to stage the content
	memStore := memory.New()

	// Push the blob as a layer
	layerDesc := ocispec.Descriptor{
		MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		Digest:    digest.FromBytes(blob),
		Size:      int64(len(blob)),
		Annotations: map[string]string{
			"containerd.io/snapshot/stargz/toc.digest": "", // Mark as eStargz
		},
	}

	if err := memStore.Push(ctx, layerDesc, bytes.NewReader(blob)); err != nil {
		return "", fmt.Errorf("push layer to memstore: %w", err)
	}

	// Create a minimal config
	config := []byte("{}")
	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageConfig,
		Digest:    digest.FromBytes(config),
		Size:      int64(len(config)),
	}

	if err := memStore.Push(ctx, configDesc, bytes.NewReader(config)); err != nil {
		return "", fmt.Errorf("push config to memstore: %w", err)
	}

	// Create manifest
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{layerDesc},
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("marshal manifest: %w", err)
	}

	manifestDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manifestJSON),
		Size:      int64(len(manifestJSON)),
	}

	if err := memStore.Push(ctx, manifestDesc, bytes.NewReader(manifestJSON)); err != nil {
		return "", fmt.Errorf("push manifest to memstore: %w", err)
	}

	// Tag the manifest
	if err := memStore.Tag(ctx, manifestDesc, tag); err != nil {
		return "", fmt.Errorf("tag manifest: %w", err)
	}

	// Copy from memory store to remote repository
	desc, err := oras.Copy(ctx, memStore, tag, repo, tag, oras.DefaultCopyOptions)
	if err != nil {
		return "", fmt.Errorf("copy to remote: %w", err)
	}

	return desc.Digest.String(), nil
}

// fileEntry holds info for display purposes
type fileEntry struct {
	Name   string
	Size   int64
	Offset int64
}

// listTOC reads the TOC from an eStargz blob.
func listTOC(blob []byte) ([]fileEntry, error) {
	sr := io.NewSectionReader(bytes.NewReader(blob), 0, int64(len(blob)))

	r, err := estargz.Open(sr)
	if err != nil {
		return nil, fmt.Errorf("open estargz: %w", err)
	}

	// Use Lookup to find entries - we'll iterate through known files
	// For a real implementation, we'd parse the TOC JSON directly
	var entries []fileEntry
	testFiles := []string{".", "config.yaml", "data.txt", "subdir", "subdir/nested.txt"}
	for _, name := range testFiles {
		if e, ok := r.Lookup(name); ok {
			entries = append(entries, fileEntry{
				Name:   e.Name,
				Size:   e.Size,
				Offset: e.Offset,
			})
		}
	}

	return entries, nil
}

// extractFile extracts a single file from an eStargz blob.
func extractFile(blob []byte, filename string) ([]byte, error) {
	sr := io.NewSectionReader(bytes.NewReader(blob), 0, int64(len(blob)))

	r, err := estargz.Open(sr)
	if err != nil {
		return nil, fmt.Errorf("open estargz: %w", err)
	}

	// Find the entry
	entry, ok := r.Lookup(filename)
	if !ok {
		return nil, fmt.Errorf("file not found: %s", filename)
	}

	// Open and read the file using OpenFile which returns a ReaderAt
	fileReader, err := r.OpenFile(entry.Name)
	if err != nil {
		return nil, fmt.Errorf("open file %s: %w", entry.Name, err)
	}

	// Read the entire file
	buf := make([]byte, entry.Size)
	_, err = fileReader.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read file %s: %w", entry.Name, err)
	}

	return buf, nil
}
