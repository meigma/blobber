package cli

import (
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
	Args: cobra.ExactArgs(2),
	RunE: runCat,
}

func init() {
	rootCmd.AddCommand(catCmd)
}

func runCat(cmd *cobra.Command, args []string) error {
	panic("not implemented")
}
