package cli

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/gilmanlab/blobber"
)

// Cache command flags
var (
	cacheDir     string
	cacheLong    bool
	pruneMaxSize string
	pruneMaxAge  string
	clearConfirm bool
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the blob cache",
	Long: `Manage the local blob cache.

The cache stores downloaded blobs locally for faster subsequent access.
Use subcommands to inspect, clear, or prune the cache.

The cache directory can be specified with --dir. If not specified,
the default location is ~/.blobber/cache.`,
}

var cacheInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show cache statistics",
	Long: `Display information about the blob cache.

Shows the total size, entry count, and optionally detailed information
about each cached blob.

Examples:
  blobber cache info
  blobber cache info --long
  blobber cache info --dir /path/to/cache`,
	Args: cobra.NoArgs,
	RunE: runCacheInfo,
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove all cached blobs",
	Long: `Remove all entries from the blob cache.

This permanently deletes all cached blobs. Use --yes to skip confirmation.

Examples:
  blobber cache clear
  blobber cache clear --yes`,
	Args: cobra.NoArgs,
	RunE: runCacheClear,
}

var cachePruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove old or excess cache entries",
	Long: `Prune the blob cache based on age and/or size limits.

Entries are evicted based on their last access time. Oldest entries
(least recently used) are removed first.

Size can be specified with units: B, KB, MB, GB, TB.
Age can be specified with units: s, m, h (e.g., 24h, 7d).

Examples:
  blobber cache prune --max-size 1GB
  blobber cache prune --max-age 24h
  blobber cache prune --max-size 500MB --max-age 7d`,
	Args: cobra.NoArgs,
	RunE: runCachePrune,
}

func init() {
	// Common cache directory flag
	cacheCmd.PersistentFlags().StringVar(&cacheDir, "dir", defaultCacheDir(), "Cache directory path")

	// Cache info flags
	cacheInfoCmd.Flags().BoolVarP(&cacheLong, "long", "l", false, "Show detailed entry information")

	// Cache clear flags
	cacheClearCmd.Flags().BoolVarP(&clearConfirm, "yes", "y", false, "Skip confirmation prompt")

	// Cache prune flags
	cachePruneCmd.Flags().StringVar(&pruneMaxSize, "max-size", "", "Maximum cache size (e.g., 1GB)")
	cachePruneCmd.Flags().StringVar(&pruneMaxAge, "max-age", "", "Maximum entry age (e.g., 24h, 7d)")

	// Register commands
	cacheCmd.AddCommand(cacheInfoCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cachePruneCmd)
	rootCmd.AddCommand(cacheCmd)
}

func runCacheInfo(_ *cobra.Command, _ []string) error {
	info, err := blobber.CacheStats(cacheDir)
	if err != nil {
		return err
	}

	if info.EntryCount == 0 {
		fmt.Println("Cache is empty")
		return nil
	}

	fmt.Printf("Cache: %s\n", info.Path)
	fmt.Printf("Size:  %s (%d bytes)\n", humanize.Bytes(safeUint64(info.TotalSize)), info.TotalSize)
	fmt.Printf("Entries: %d\n", info.EntryCount)

	if cacheLong && len(info.Entries) > 0 {
		fmt.Println()
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "DIGEST\tSIZE\tLAST ACCESSED\tCOMPLETE")
		for _, e := range info.Entries {
			complete := "yes"
			if !e.Complete {
				complete = "no"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				truncateDigest(e.Digest),
				humanize.Bytes(safeUint64(e.Size)),
				humanize.Time(e.LastAccessed),
				complete)
		}
		tw.Flush()
	}

	return nil
}

func runCacheClear(_ *cobra.Command, _ []string) error {
	// Get cache stats first
	info, err := blobber.CacheStats(cacheDir)
	if err != nil {
		return err
	}

	if info.EntryCount == 0 {
		fmt.Println("Cache is already empty")
		return nil
	}

	// Confirm unless --yes is specified
	if !clearConfirm {
		fmt.Printf("This will remove %d entries (%s) from the cache.\n",
			info.EntryCount, humanize.Bytes(safeUint64(info.TotalSize)))
		fmt.Print("Continue? [y/N] ")

		var response string
		//nolint:errcheck // Empty input or EOF is treated as "no" - not an error
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Aborted")
			return nil
		}
	}

	if err := blobber.CacheClear(cacheDir); err != nil {
		return err
	}

	fmt.Printf("Cleared %d entries (%s)\n",
		info.EntryCount, humanize.Bytes(safeUint64(info.TotalSize)))
	return nil
}

func runCachePrune(_ *cobra.Command, _ []string) error {
	opts := blobber.CachePruneOptions{}

	// Parse max-size flag
	if pruneMaxSize != "" {
		size, err := humanize.ParseBytes(pruneMaxSize)
		if err != nil {
			return fmt.Errorf("invalid --max-size: %w", err)
		}
		opts.MaxSize = safeInt64(size)
	}

	// Parse max-age flag
	if pruneMaxAge != "" {
		age, err := parseDuration(pruneMaxAge)
		if err != nil {
			return fmt.Errorf("invalid --max-age: %w", err)
		}
		opts.MaxAge = age
	}

	// Require at least one option
	if opts.MaxSize == 0 && opts.MaxAge == 0 {
		return errors.New("at least one of --max-size or --max-age is required")
	}

	ctx, cancel := signalContext()
	defer cancel()

	result, err := blobber.CachePrune(ctx, cacheDir, opts)
	if err != nil {
		return err
	}

	if result.EntriesRemoved == 0 {
		fmt.Println("No entries to prune")
	} else {
		fmt.Printf("Removed %d entries (%s)\n",
			result.EntriesRemoved, humanize.Bytes(safeUint64(result.BytesRemoved)))
	}

	if result.EntriesRemaining > 0 {
		fmt.Printf("Remaining: %d entries (%s)\n",
			result.EntriesRemaining, humanize.Bytes(safeUint64(result.BytesRemaining)))
	}

	return nil
}

// defaultCacheDir returns the default cache directory path.
func defaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".blobber/cache"
	}
	return home + "/.blobber/cache"
}

// truncateDigest shortens a digest for display.
// sha256:abc123... -> sha256:abc123...
func truncateDigest(digest string) string {
	if len(digest) <= 19 {
		return digest
	}
	return digest[:19] + "..."
}

// parseDuration parses a duration string with support for days (d).
func parseDuration(s string) (time.Duration, error) {
	// Handle days suffix
	if s != "" && s[len(s)-1] == 'd' {
		days, err := parseInt(s[:len(s)-1])
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// parseInt parses an integer from a string.
func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid integer: %s", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// safeUint64 converts int64 to uint64, clamping negative values to 0.
func safeUint64(n int64) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}

// safeInt64 converts uint64 to int64, clamping to max int64 if overflow.
func safeInt64(n uint64) int64 {
	const maxInt64 = int64(^uint64(0) >> 1)
	if n > uint64(maxInt64) {
		return maxInt64
	}
	return int64(n)
}
