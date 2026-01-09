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
// For the first argument (image reference), it provides tag completion.
// For the second argument (file path), it queries the image's table of contents.
func completeImageFiles(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// First arg is image reference - provide tag completion
	if len(args) < 1 {
		return completeImageRef(cmd, args, toComplete)
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
	// Limit to 50 completions to avoid overwhelming the shell
	const maxCompletions = 50
	var completions []string
	for _, entry := range entries {
		path := entry.Path()
		if strings.HasPrefix(path, toComplete) {
			completions = append(completions, path)
			if len(completions) >= maxCompletions {
				break
			}
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
		opts = append(opts,
			blobber.WithCacheDir(cacheDir),
			// Use TTL to avoid network calls for recently resolved refs
			blobber.WithCacheTTL(blobber.DefaultTagListTTL),
		)
	}

	return blobber.NewClient(opts...)
}

// completePullArgs provides completion for the pull command arguments:
// - First arg: image reference (tag completion)
// - Second arg: local directory (filesystem directory completion)
func completePullArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		// First arg is the image reference - provide tag completion
		return completeImageRef(cmd, args, toComplete)
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

// completeImageRef returns a completion function that suggests tags for an
// OCI image reference. This is useful for commands that take an image reference.
//
// The function looks for a tag separator (:) after the last path separator (/)
// and if found, fetches available tags from the registry and filters them by prefix.
//
// Example: "ghcr.io/org/repo:v1" -> suggests tags starting with "v1"
func completeImageRef(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Only complete the first argument (image reference)
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Look for tag separator - must be after the last /
	lastSlash := strings.LastIndex(toComplete, "/")
	colonIdx := strings.LastIndex(toComplete, ":")

	// No tag separator yet, or colon is part of the registry (before last /)
	if colonIdx == -1 || colonIdx < lastSlash {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Extract repository and partial tag
	repository := toComplete[:colonIdx]
	partialTag := toComplete[colonIdx+1:]

	// Need at least a repository to complete tags
	if repository == "" {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Create a client for completion
	client, err := newCompletionClient()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	// Use a timeout context to avoid blocking the shell
	ctx, cancel := context.WithTimeout(context.Background(), completionTimeout)
	defer cancel()

	// Fetch tags from the registry (uses cache if available)
	tags, err := client.ListTags(ctx, repository)
	if err != nil {
		// Don't show error to user during completion - just return no suggestions
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Filter tags that match the prefix being typed
	// Limit to 50 completions to avoid overwhelming the shell
	const maxCompletions = 50
	var completions []string
	for _, tag := range tags {
		if strings.HasPrefix(tag, partialTag) {
			// Must return full reference since shell replaces the entire argument
			completions = append(completions, repository+":"+tag)
			if len(completions) >= maxCompletions {
				break
			}
		}
	}

	// NoFileComp prevents falling back to local file completion
	return completions, cobra.ShellCompDirectiveNoFileComp
}
