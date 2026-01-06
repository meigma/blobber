// Package cli implements the blobber command-line interface.
package cli

import (
	"github.com/spf13/cobra"
)

// Global flags.
var insecure bool

var rootCmd = &cobra.Command{
	Use:   "blobber",
	Short: "Push and pull files to OCI registries",
	Long: `Blobber is a CLI for pushing and pulling arbitrary files to OCI container registries.

It uses eStargz format to enable efficient file listing and selective retrieval
without downloading entire images.`,
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&insecure, "insecure", false, "Allow insecure registry connections")
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
