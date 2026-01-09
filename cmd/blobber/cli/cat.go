package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

var catCmd = &cobra.Command{
	Use:   "cat <reference> <path>",
	Short: "Output a file from an OCI image",
	Long: `Cat outputs the contents of a single file from an OCI registry image.

This leverages eStargz format to download only the requested file.

Examples:
  blobber cat ghcr.io/org/config:v1 config.yaml
  blobber cat ghcr.io/org/config:v1 subdir/data.json > data.json`,
	Args:              cobra.ExactArgs(2),
	RunE:              runCat,
	ValidArgsFunction: completeImageFiles,
}

func init() {
	rootCmd.AddCommand(catCmd)
}

func runCat(_ *cobra.Command, args []string) error {
	ref := args[0]
	filePath := args[1]

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

	// Open file
	rc, err := img.Open(filePath)
	if err != nil {
		return err
	}
	defer rc.Close()

	// Stream to stdout
	_, err = io.Copy(os.Stdout, rc)
	return err
}
