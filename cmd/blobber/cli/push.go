package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/meigma/blobber"
)

var pushCompression string

var pushCmd = &cobra.Command{
	Use:     "push <directory> <reference>",
	Short:   "Push a directory to an OCI registry",
	GroupID: "core",
	Long: `Push uploads a directory of files to an OCI registry as an eStargz image.

Use --sign to sign the artifact with Sigstore (keyless). This requires OIDC
authentication (e.g., via browser or OIDC token).

Examples:
  blobber push ./config ghcr.io/org/config:v1
  blobber push ./data ghcr.io/org/data:latest --compression zstd
  blobber push ./data ghcr.io/org/data:latest --sign`,
	Args:              cobra.ExactArgs(2),
	RunE:              runPush,
	ValidArgsFunction: completePushArgs,
}

func init() {
	pushCmd.Flags().StringVar(&pushCompression, "compression", "gzip", "Compression algorithm (gzip, zstd)")
	rootCmd.AddCommand(pushCmd)
}

func runPush(_ *cobra.Command, args []string) error {
	dir := args[0]
	ref := args[1]

	// Validate directory exists and is a directory
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory not found: %s", dir)
		}
		return fmt.Errorf("cannot access directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", dir)
	}

	// Parse compression option
	compression, err := parseCompression(pushCompression)
	if err != nil {
		return err
	}

	// Create client
	client, err := newClient()
	if err != nil {
		return err
	}

	// Set up signal handling
	ctx, cancel := signalContext()
	defer cancel()

	// Push
	digest, err := client.Push(ctx, ref, os.DirFS(dir), blobber.WithCompression(compression))
	if err != nil {
		return err
	}

	// Output digest on success
	fmt.Println(digest)
	return nil
}

// parseCompression converts the --compression flag value to a Compression.
func parseCompression(s string) (blobber.Compression, error) {
	switch strings.ToLower(s) {
	case "gzip":
		return blobber.GzipCompression(), nil
	case "zstd":
		return blobber.ZstdCompression(), nil
	default:
		return nil, fmt.Errorf("invalid compression: %q (must be gzip or zstd)", s)
	}
}
