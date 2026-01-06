package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/gilmanlab/blobber"
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

func runList(_ *cobra.Command, args []string) error {
	ref := args[0]

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

	// Get file list
	entries, err := img.List()
	if err != nil {
		return err
	}

	// Print entries
	if listLong {
		printLongListing(os.Stdout, entries)
	} else {
		printShortListing(os.Stdout, entries)
	}

	return nil
}

// printShortListing prints just the file paths.
func printShortListing(w io.Writer, entries []blobber.FileEntry) {
	for _, entry := range entries {
		fmt.Fprintln(w, entry.Path())
	}
}

// printLongListing prints path, size, and mode in tabular format.
func printLongListing(w io.Writer, entries []blobber.FileEntry) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, entry := range entries {
		// Format: path    size    mode (as octal)
		fmt.Fprintf(tw, "%s\t%d\t%04o\n",
			entry.Path(),
			entry.Size(),
			entry.Mode().Perm())
	}
	tw.Flush()
}
