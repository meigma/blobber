package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/meigma/blobber/cmd/blobber/cli/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage blobber configuration",
	Long: `View and modify blobber configuration.

Without arguments, displays the current effective configuration.
Use subcommands to view the config path, initialize a config file,
or set configuration values.`,
	RunE: runConfigShow,
}

func init() {
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show configuration file path",
	RunE: func(_ *cobra.Command, _ []string) error {
		configDir, err := config.Dir()
		if err != nil {
			return err
		}
		fmt.Println(filepath.Join(configDir, "config.yaml"))
		return nil
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create default configuration file",
	Long: `Create a default configuration file at the XDG config path.

The file will be created at ~/.config/blobber/config.yaml (or
$XDG_CONFIG_HOME/blobber/config.yaml if set).`,
	RunE: runConfigInit,
}

func runConfigInit(_ *cobra.Command, _ []string) error {
	configDir, err := config.Dir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(configDir, "config.yaml")

	// Check if already exists
	if _, statErr := os.Stat(configPath); statErr == nil {
		return fmt.Errorf("config file already exists: %s", configPath)
	}

	// Create directory and write default config
	if mkdirErr := os.MkdirAll(configDir, 0o750); mkdirErr != nil {
		return mkdirErr
	}

	defaultConfig := map[string]any{
		"cache": map[string]any{
			"enabled": true,
			"verify":  false,
		},
		"sign": map[string]any{
			"enabled": false,
			// key, password omitted - typically set via flags or env vars for security
			"fulcio": defaultFulcioURL,
			"rekor":  defaultRekorURL,
		},
		"verify": map[string]any{
			"enabled": false,
			// issuer, subject, trusted-root omitted - must be explicitly configured
		},
	}
	data, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if writeErr := os.WriteFile(configPath, data, 0o600); writeErr != nil {
		return writeErr
	}

	fmt.Printf("Created config file: %s\n", configPath)
	return nil
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value in the config file.

Examples:
  blobber config set cache.enabled false
  blobber config set cache.dir /custom/cache/path`,
	Args: cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		key, value := args[0], args[1]

		// Parse boolean values
		var parsedValue any
		switch value {
		case "true":
			parsedValue = true
		case "false":
			parsedValue = false
		default:
			parsedValue = value
		}

		// Set in Viper
		viper.Set(key, parsedValue)

		// Write to config file
		configDir, err := config.Dir()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(configDir, 0o750); err != nil {
			return err
		}

		configPath := filepath.Join(configDir, "config.yaml")
		if err := viper.WriteConfigAs(configPath); err != nil {
			return fmt.Errorf("write config: %w", err)
		}

		fmt.Printf("Updated %s = %v\n", key, parsedValue)
		return nil
	},
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	// Show all settings with their effective values
	settings := viper.AllSettings()
	data, err := yaml.Marshal(settings)
	if err != nil {
		return err
	}
	fmt.Print(string(data))
	return nil
}
