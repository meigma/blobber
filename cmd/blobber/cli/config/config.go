package config

import "time"

// Config represents the blobber CLI configuration.
// Use mapstructure tags for Viper unmarshaling.
type Config struct {
	Cache CacheConfig `mapstructure:"cache"`
}

// CacheConfig holds cache-related settings.
type CacheConfig struct {
	Enabled bool          `mapstructure:"enabled"`
	Dir     string        `mapstructure:"dir"`
	TTL     time.Duration `mapstructure:"ttl"`
}
