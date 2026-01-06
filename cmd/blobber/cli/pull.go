package cli

import (
	"github.com/spf13/cobra"
)

var pullOverwrite bool

var pullCmd = &cobra.Command{
	Use:   "pull <reference> <directory>",
	Short: "Pull an image from an OCI registry",
	Long: `Pull downloads all files from an OCI registry image to a local directory.

Examples:
  blobber pull ghcr.io/org/config:v1 ./config
  blobber pull ghcr.io/org/data:latest ./data --overwrite`,
	Args: cobra.ExactArgs(2),
	RunE: runPull,
}

func init() {
	pullCmd.Flags().BoolVar(&pullOverwrite, "overwrite", false, "Overwrite existing files")
	rootCmd.AddCommand(pullCmd)
}

func runPull(cmd *cobra.Command, args []string) error {
	panic("not implemented")
}
