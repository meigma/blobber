package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/meigma/blobber"
)

var pullOverwrite bool

var pullCmd = &cobra.Command{
	Use:   "pull <reference> <directory>",
	Short: "Pull an image from an OCI registry",
	Long: `Pull downloads all files from an OCI registry image to a local directory.

By default, files are merged into the destination directory. If a file already
exists, the operation fails. Use --overwrite to replace existing files.

Use --verify to verify the artifact's Sigstore signature before pulling.
Specify --verify-issuer and --verify-subject to require a specific signer identity,
or use --verify-unsafe to accept any valid signer identity (unsafe).

Examples:
  blobber pull ghcr.io/org/config:v1 ./config
  blobber pull ghcr.io/org/data:latest ./data --overwrite
  blobber pull ghcr.io/org/data:latest ./data --verify --verify-issuer https://accounts.google.com --verify-subject user@example.com
  blobber pull ghcr.io/org/data:latest ./data --verify --verify-unsafe`,
	Args: cobra.ExactArgs(2),
	RunE: runPull,
}

func init() {
	pullCmd.Flags().BoolVar(&pullOverwrite, "overwrite", false, "Overwrite existing files")
	rootCmd.AddCommand(pullCmd)
}

func runPull(_ *cobra.Command, args []string) error {
	ref := args[0]
	destDir := args[1]

	// Create client
	client, err := newClient()
	if err != nil {
		return err
	}

	// Set up signal handling
	ctx, cancel := signalContext()
	defer cancel()

	// Handle file conflicts
	if err := handlePullConflicts(ctx, client, ref, destDir, pullOverwrite); err != nil {
		return err
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("cannot create destination directory: %w", err)
	}

	// Pull
	return client.Pull(ctx, ref, destDir)
}

// handlePullConflicts checks for and handles file conflicts.
// If overwrite is false, returns an error if conflicts exist.
// If overwrite is true, removes conflicting files.
func handlePullConflicts(ctx context.Context, client *blobber.Client, ref, destDir string, overwrite bool) error {
	// Only check if destination directory exists
	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		return nil
	}

	// Open image to get file list
	img, err := client.OpenImage(ctx, ref)
	if err != nil {
		return err
	}
	defer img.Close()

	entries, err := img.List()
	if err != nil {
		return err
	}

	// Find conflicts
	var conflicts []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(destDir, entry.Path())
		if _, statErr := os.Stat(fullPath); statErr == nil {
			conflicts = append(conflicts, fullPath)
		}
	}

	if len(conflicts) == 0 {
		return nil
	}

	if !overwrite {
		if len(conflicts) == 1 {
			return fmt.Errorf("file already exists: %s (use --overwrite to replace)", filepath.Base(conflicts[0]))
		}
		return fmt.Errorf("%d files already exist (use --overwrite to replace)", len(conflicts))
	}

	// Remove conflicting files
	for _, path := range conflicts {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot remove existing file %s: %w", path, err)
		}
	}

	return nil
}
