// Package cli implements the blobber command-line interface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/gilmanlab/blobber"
)

// Build information set via ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Global flags.
var (
	insecure bool
	verbose  bool
)

var rootCmd = &cobra.Command{
	Use:   "blobber",
	Short: "Push and pull files to OCI registries",
	Long: `Blobber is a CLI for pushing and pulling arbitrary files to OCI container registries.

It uses eStargz format to enable efficient file listing and selective retrieval
without downloading entire images.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&insecure, "insecure", false, "Allow insecure registry connections")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose debug logging")
	rootCmd.Version = version
}

// Execute runs the root command.
func Execute() error {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, formatError(err))
	}
	return err
}

// newClient creates a blobber client with configured options.
func newClient() (*blobber.Client, error) {
	opts := []blobber.ClientOption{
		blobber.WithInsecure(insecure),
	}
	if verbose {
		opts = append(opts, blobber.WithLogger(
			slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})),
		))
	}
	return blobber.NewClient(opts...)
}

// signalContext returns a context that is canceled on SIGINT or SIGTERM.
func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sigCh)
	}()

	return ctx, cancel
}

// formatError converts blobber errors to user-friendly messages.
func formatError(err error) string {
	if err == nil {
		return ""
	}

	switch {
	case errors.Is(err, blobber.ErrNotFound):
		return fmt.Sprintf("Error: not found: %v", err)
	case errors.Is(err, blobber.ErrUnauthorized):
		return "Error: authentication failed (check your credentials)"
	case errors.Is(err, blobber.ErrInvalidRef):
		return fmt.Sprintf("Error: invalid reference: %v", err)
	case errors.Is(err, blobber.ErrPathTraversal):
		return "Error: path traversal detected (security violation)"
	case errors.Is(err, blobber.ErrInvalidArchive):
		return "Error: invalid or corrupt archive"
	case errors.Is(err, context.Canceled):
		return "Error: operation canceled"
	default:
		return fmt.Sprintf("Error: %v", err)
	}
}
