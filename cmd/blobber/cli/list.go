package cli

import (
	"github.com/spf13/cobra"
)

var listLong bool

var listCmd = &cobra.Command{
	Use:     "list <reference>",
	Aliases: []string{"ls"},
	Short:   "List files in an OCI image",
	Long: `List displays the files in an OCI registry image without downloading it.

This leverages eStargz format to read only the table of contents.

Examples:
  blobber list ghcr.io/org/config:v1
  blobber ls ghcr.io/org/config:v1 --long`,
	Args: cobra.ExactArgs(1),
	RunE: runList,
}

func init() {
	listCmd.Flags().BoolVarP(&listLong, "long", "l", false, "Use long listing format")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	panic("not implemented")
}
