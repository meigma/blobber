package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/meigma/blobber"
)

var (
	listLong  bool
	listHuman bool
)

var listCmd = &cobra.Command{
	Use:     "ls <reference>",
	Aliases: []string{"list"},
	Short:   "List files in an OCI image",
	GroupID: "core",
	Long: `Ls displays the files in an OCI registry image without downloading it.

This leverages eStargz format to read only the table of contents.

Examples:
  blobber ls ghcr.io/org/config:v1
  blobber ls -l ghcr.io/org/config:v1
  blobber ls -lH ghcr.io/org/config:v1`,
	Args:              cobra.ExactArgs(1),
	RunE:              runList,
	ValidArgsFunction: completeImageRef,
}

func init() {
	listCmd.Flags().BoolVarP(&listLong, "long", "l", false, "Use long listing format")
	listCmd.Flags().BoolVarP(&listHuman, "human-readable", "H", false, "Print sizes in human-readable format")
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

// printLongListing prints mode, size, and path in ls -l style format.
func printLongListing(w io.Writer, entries []blobber.FileEntry) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, entry := range entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\n",
			formatMode(entry.Mode()),
			formatSize(entry),
			entry.Path())
	}
	tw.Flush()
}

// formatMode converts fs.FileMode to symbolic format (e.g., "-rw-r--r--").
func formatMode(mode fs.FileMode) string {
	buf := make([]byte, 10)

	// Type indicator
	switch {
	case mode.IsDir():
		buf[0] = 'd'
	case mode&fs.ModeSymlink != 0:
		buf[0] = 'l'
	case mode&fs.ModeNamedPipe != 0:
		buf[0] = 'p'
	case mode&fs.ModeSocket != 0:
		buf[0] = 's'
	case mode&fs.ModeDevice != 0:
		if mode&fs.ModeCharDevice != 0 {
			buf[0] = 'c'
		} else {
			buf[0] = 'b'
		}
	default:
		buf[0] = '-'
	}

	// Permission bits
	const rwx = "rwx"
	for i := range 3 {
		for j := range 3 {
			//nolint:gosec // G115: i and j are in range 0-2, no overflow possible
			if mode&(1<<uint(8-i*3-j)) != 0 {
				buf[1+i*3+j] = rwx[j]
			} else {
				buf[1+i*3+j] = '-'
			}
		}
	}

	return string(buf)
}

// formatSize formats file size for display.
func formatSize(entry blobber.FileEntry) string {
	if entry.IsDir() {
		return "-"
	}
	size := entry.Size()
	if listHuman {
		//nolint:gosec // G115: size is from trusted archive metadata
		return humanize.IBytes(uint64(size))
	}
	return strconv.FormatInt(size, 10)
}
