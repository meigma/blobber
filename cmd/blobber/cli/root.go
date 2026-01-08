// Package cli implements the blobber command-line interface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/gilmanlab/blobber"
	"github.com/gilmanlab/blobber/cmd/blobber/cli/config"
)

// Build information set via ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// cfgFile is the path to the config file (set via --config flag).
var cfgFile string

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
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	rootCmd.PersistentFlags().Bool("insecure", false, "Allow insecure registry connections")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose debug logging")
	rootCmd.PersistentFlags().Bool("no-cache", false, "Bypass cache for this request")

	// Bind flags to Viper (errors only occur if flag doesn't exist, which can't happen here)
	//nolint:errcheck // flags are defined above, so Lookup will never return nil
	viper.BindPFlag("insecure", rootCmd.PersistentFlags().Lookup("insecure"))
	//nolint:errcheck
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	//nolint:errcheck
	viper.BindPFlag("no-cache", rootCmd.PersistentFlags().Lookup("no-cache"))

	// Set defaults
	viper.SetDefault("cache.enabled", true)
	viper.SetDefault("cache.dir", "") // Empty means use XDG default

	rootCmd.Version = version
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		configDir, err := config.Dir()
		if err == nil {
			viper.AddConfigPath(configDir)
		}
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	// Environment variables: BLOBBER_CACHE_ENABLED, BLOBBER_INSECURE, etc.
	viper.SetEnvPrefix("BLOBBER")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	// Config file is optional - don't fail if missing
	if err := viper.ReadInConfig(); err == nil {
		if viper.GetBool("verbose") {
			fmt.Fprintln(os.Stderr, "Using config:", viper.ConfigFileUsed())
		}
	}
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
		blobber.WithInsecure(viper.GetBool("insecure")),
	}

	if viper.GetBool("verbose") {
		opts = append(opts, blobber.WithLogger(
			slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})),
		))
	}

	// Cache is enabled unless:
	// 1. cache.enabled is false in config/env, OR
	// 2. --no-cache flag was passed
	noCache := viper.GetBool("no-cache")
	cacheEnabled := viper.GetBool("cache.enabled")

	if cacheEnabled && !noCache {
		cacheDir := viper.GetString("cache.dir")
		if cacheDir == "" {
			var err error
			cacheDir, err = config.CacheDir()
			if err != nil {
				return nil, fmt.Errorf("determine cache directory: %w", err)
			}
		}
		opts = append(opts, blobber.WithCacheDir(cacheDir))
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
