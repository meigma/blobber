package cli

import (
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var cpCmd = &cobra.Command{
	Use:     "cp <reference> <path> <destination>",
	Short:   "Copy a file from an OCI image to a local path",
	GroupID: "core",
	Long: `Cp copies a single file from an OCI registry image to a local path.

This leverages eStargz format to download only the requested file.
Parent directories are created automatically if they don't exist.
If the destination is an existing directory, the file is placed inside it.

Examples:
  blobber cp ghcr.io/org/config:v1 config.yaml ./config.yaml
  blobber cp ghcr.io/org/config:v1 subdir/data.json ./output/data.json
  blobber cp ghcr.io/org/config:v1 config.yaml /tmp/  # creates /tmp/config.yaml`,
	Args:              cobra.ExactArgs(3),
	RunE:              runCp,
	ValidArgsFunction: completeCpArgs,
}

func init() {
	rootCmd.AddCommand(cpCmd)
}

func runCp(_ *cobra.Command, args []string) error {
	ref := args[0]
	filePath := args[1]
	destPath := args[2]

	// Create client
	client, err := newClient()
	if err != nil {
		return err
	}

	// Set up signal handling
	ctx, cancel := signalContext()
	defer cancel()

	// Open image
	img, err := client.OpenImage(ctx, ref)
	if err != nil {
		return err
	}
	defer img.Close()

	// Open file from image
	rc, err := img.Open(filePath)
	if err != nil {
		return err
	}
	defer rc.Close()

	// If destination is a directory, append the source filename (like real cp)
	if info, statErr := os.Stat(destPath); statErr == nil && info.IsDir() {
		destPath = filepath.Join(destPath, filepath.Base(filePath))
	}

	// Create parent directories if needed
	if dir := filepath.Dir(destPath); dir != "." {
		if mkdirErr := os.MkdirAll(dir, 0o750); mkdirErr != nil {
			return mkdirErr
		}
	}

	// Create destination file (user-controlled path is intentional)
	destFile, err := os.Create(destPath) //nolint:gosec // G304: destPath is user-provided CLI argument
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy contents
	_, err = io.Copy(destFile, rc)
	return err
}

// completeCpArgs provides shell completion for cp arguments.
func completeCpArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		// First arg is reference - provide tag completion
		return completeImageRef(cmd, args, toComplete)
	case 1:
		// Second arg is file path in image - use image file completion
		return completeImageFiles(cmd, args, toComplete)
	default:
		// Third arg is destination - use filesystem completion
		return nil, cobra.ShellCompDirectiveDefault
	}
}
