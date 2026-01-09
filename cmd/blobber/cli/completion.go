package cli

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/meigma/blobber"
	"github.com/meigma/blobber/cmd/blobber/cli/config"
)

// completionTimeout is the maximum time allowed for completion requests.
// Kept short to avoid blocking the shell.
const completionTimeout = 3 * time.Second

// completeImageFiles returns a completion function that suggests file paths
// from inside an OCI image. This is useful for commands like `cat` that take
// an image reference followed by a file path within that image.
//
// The function expects the image reference to be the first argument (args[0]).
// It queries the image's table of contents and returns matching file paths.
func completeImageFiles(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// We need at least one arg (the image reference) to complete file paths
	if len(args) < 1 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// If we already have 2 args, no more completions needed
	if len(args) >= 2 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ref := args[0]

	// Create a client for completion (with minimal options for speed)
	client, err := newCompletionClient()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	// Use a timeout context to avoid blocking the shell
	ctx, cancel := context.WithTimeout(context.Background(), completionTimeout)
	defer cancel()

	// Open the image
	img, err := client.OpenImage(ctx, ref)
	if err != nil {
		// Don't show error to user during completion - just return no suggestions
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer img.Close()

	// Get file list
	entries, err := img.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Filter entries that match the prefix being typed
	var completions []string
	for _, entry := range entries {
		path := entry.Path()
		if strings.HasPrefix(path, toComplete) {
			completions = append(completions, path)
		}
	}

	// NoFileComp prevents falling back to local file completion
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// newCompletionClient creates a minimal blobber client for completion requests.
// It uses cache when available but skips verbose logging and signing/verification.
func newCompletionClient() (*blobber.Client, error) {
	var opts []blobber.ClientOption

	// Use cache if available (speeds up repeated completions)
	cacheDir, err := config.CacheDir()
	if err == nil && cacheDir != "" {
		opts = append(opts, blobber.WithCacheDir(cacheDir))
	}

	return blobber.NewClient(opts...)
}

// completePullArgs provides completion for the pull command arguments:
// - First arg: image reference (no completion - user must type it)
// - Second arg: local directory (filesystem directory completion)
func completePullArgs(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		// First arg is the image reference - no automatic completion
		return nil, cobra.ShellCompDirectiveNoFileComp
	case 1:
		// Second arg is the destination directory
		return nil, cobra.ShellCompDirectiveFilterDirs
	default:
		// No more args expected
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

// completePushArgs provides completion for the push command arguments:
// - First arg: local directory (filesystem directory completion)
// - Second arg: image reference (no completion - user must type it)
func completePushArgs(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		// First arg is the source directory
		return nil, cobra.ShellCompDirectiveFilterDirs
	case 1:
		// Second arg is the image reference - no automatic completion
		return nil, cobra.ShellCompDirectiveNoFileComp
	default:
		// No more args expected
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}
