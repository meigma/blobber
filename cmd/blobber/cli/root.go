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

	"github.com/meigma/blobber"
	"github.com/meigma/blobber/cmd/blobber/cli/config"
	"github.com/meigma/blobber/sigstore"
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

// Default Sigstore URLs.
const (
	defaultFulcioURL = "https://fulcio.sigstore.dev"
	defaultRekorURL  = "https://rekor.sigstore.dev"
)

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	rootCmd.PersistentFlags().Bool("insecure", false, "Allow insecure registry connections")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose debug logging")
	rootCmd.PersistentFlags().Bool("no-cache", false, "Bypass cache for this request")
	rootCmd.PersistentFlags().Duration("cache-ttl", 0, "TTL for cache validation (e.g., 5m, 1h)")

	// Signing flags
	rootCmd.PersistentFlags().Bool("sign", false, "Sign artifacts using Sigstore (keyless)")
	rootCmd.PersistentFlags().String("fulcio-url", defaultFulcioURL, "Fulcio CA URL for keyless signing")
	rootCmd.PersistentFlags().String("rekor-url", defaultRekorURL, "Rekor transparency log URL")

	// Verification flags
	rootCmd.PersistentFlags().Bool("verify", false, "Verify artifact signatures")
	rootCmd.PersistentFlags().String("verify-identity", "", "Required identity (format: issuer,subject)")
	rootCmd.PersistentFlags().String("trusted-root", "", "Path to trusted root JSON file")

	// Bind flags to Viper (errors only occur if flag doesn't exist, which can't happen here)
	//nolint:errcheck // flags are defined above, so Lookup will never return nil
	viper.BindPFlag("insecure", rootCmd.PersistentFlags().Lookup("insecure"))
	//nolint:errcheck
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	//nolint:errcheck
	viper.BindPFlag("no-cache", rootCmd.PersistentFlags().Lookup("no-cache"))
	//nolint:errcheck
	viper.BindPFlag("cache.ttl", rootCmd.PersistentFlags().Lookup("cache-ttl"))
	//nolint:errcheck
	viper.BindPFlag("sign", rootCmd.PersistentFlags().Lookup("sign"))
	//nolint:errcheck
	viper.BindPFlag("fulcio-url", rootCmd.PersistentFlags().Lookup("fulcio-url"))
	//nolint:errcheck
	viper.BindPFlag("rekor-url", rootCmd.PersistentFlags().Lookup("rekor-url"))
	//nolint:errcheck
	viper.BindPFlag("verify", rootCmd.PersistentFlags().Lookup("verify"))
	//nolint:errcheck
	viper.BindPFlag("verify-identity", rootCmd.PersistentFlags().Lookup("verify-identity"))
	//nolint:errcheck
	viper.BindPFlag("trusted-root", rootCmd.PersistentFlags().Lookup("trusted-root"))

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
	cacheTTL := viper.GetDuration("cache.ttl")

	// Mutual exclusion check
	if noCache && cacheTTL > 0 {
		return nil, errors.New("--no-cache and --cache-ttl are mutually exclusive")
	}

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

		// Apply TTL if configured
		if cacheTTL > 0 {
			opts = append(opts, blobber.WithCacheTTL(cacheTTL))
		}
	}

	// Configure signer if --sign flag is set
	if viper.GetBool("sign") {
		signer, err := createSigner()
		if err != nil {
			return nil, fmt.Errorf("configure signer: %w", err)
		}
		opts = append(opts, blobber.WithSigner(signer))
	}

	// Configure verifier if --verify flag is set
	if viper.GetBool("verify") {
		verifier, err := createVerifier()
		if err != nil {
			return nil, fmt.Errorf("configure verifier: %w", err)
		}
		opts = append(opts, blobber.WithVerifier(verifier))
	}

	return blobber.NewClient(opts...)
}

// createSigner creates a sigstore signer with configured options.
func createSigner() (blobber.Signer, error) {
	fulcioURL := viper.GetString("fulcio-url")
	rekorURL := viper.GetString("rekor-url")

	return sigstore.NewSigner(
		sigstore.WithEphemeralKey(),
		sigstore.WithFulcio(fulcioURL),
		sigstore.WithRekor(rekorURL),
	)
}

// createVerifier creates a sigstore verifier with configured options.
func createVerifier() (blobber.Verifier, error) {
	var opts []sigstore.VerifierOption

	// Load custom trusted root if specified
	if trustedRoot := viper.GetString("trusted-root"); trustedRoot != "" {
		opts = append(opts, sigstore.WithTrustedRootFile(trustedRoot))
	}

	// Parse identity requirement if specified
	if identity := viper.GetString("verify-identity"); identity != "" {
		parts := strings.SplitN(identity, ",", 2)
		if len(parts) != 2 {
			return nil, errors.New("--verify-identity must be in format: issuer,subject")
		}
		opts = append(opts, sigstore.WithIdentity(parts[0], parts[1]))
	}

	return sigstore.NewVerifier(opts...)
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
	case errors.Is(err, blobber.ErrSignatureInvalid):
		return "Error: signature verification failed (artifact may be tampered)"
	case errors.Is(err, blobber.ErrNoSignature):
		return "Error: no signature found (use --verify with signed artifacts)"
	case errors.Is(err, context.Canceled):
		return "Error: operation canceled"
	default:
		return fmt.Sprintf("Error: %v", err)
	}
}
