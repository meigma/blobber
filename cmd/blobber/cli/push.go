package cli

import (
	"github.com/spf13/cobra"
)

var pushCompression string

var pushCmd = &cobra.Command{
	Use:   "push <directory> <reference>",
	Short: "Push a directory to an OCI registry",
	Long: `Push uploads a directory of files to an OCI registry as an eStargz image.

Examples:
  blobber push ./config ghcr.io/org/config:v1
  blobber push ./data ghcr.io/org/data:latest --compression zstd`,
	Args: cobra.ExactArgs(2),
	RunE: runPush,
}

func init() {
	pushCmd.Flags().StringVar(&pushCompression, "compression", "gzip", "Compression algorithm (gzip, zstd)")
	rootCmd.AddCommand(pushCmd)
}

func runPush(cmd *cobra.Command, args []string) error {
	panic("not implemented")
}
