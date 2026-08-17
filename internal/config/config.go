// Package config
package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	// Preferences
	Preferences Preferences `yaml:"preferences"`

	// Kubeconfig path (defaults to ~/.kube/config)
	KubeconfigPath string `yaml:"kubeconfig_path"`
}

// Preferences contains user preferences actually consumed by the app —
// only fields a caller reads belong here; see cmd/page-client/main.go
// (RefreshInterval) and NewMainPageModel (MaxLogLines).
type Preferences struct {
	MaxLogLines     int `yaml:"max_log_lines"`    // Max lines to keep in memory per log source
	RefreshInterval int `yaml:"refresh_interval"` // Seconds between pod info refresh
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		Preferences: Preferences{
			MaxLogLines:     1000,
			RefreshInterval: 5,
		},
		KubeconfigPath: "", // Will use default
	}
}

// GetDefaultConfigPath returns the default config file path
func GetDefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(home, ".config", "ktails")
	return filepath.Join(configDir, "config.yaml"), nil
}

// Load loads configuration from file. A missing file is expected and
// returns DefaultConfig() silently; anything else that goes wrong — the
// file can't be read (permissions, some other non-existence stat error), it
// doesn't parse as YAML, or it parses but fails Validate() (e.g. an unknown
// theme) — also falls back to DefaultConfig(), logging a warning via the
// standard log package instead of failing. A config file, corrupted or
// merely outdated, must never be able to stop the app from starting; the
// user can always fix or delete it once they're actually running and can
// see the warning (KTAILS_DEBUG=1; see main.go's setupLogging). If path is
// empty, uses the default config path.
func Load(path string) *Config {
	if path == "" {
		defaultPath, err := GetDefaultConfigPath()
		if err != nil {
			log.Printf("config: failed to determine default config path, using defaults: %v", err)
			return DefaultConfig()
		}
		path = defaultPath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("config: failed to read config file %s, using defaults: %v", path, err)
		}
		return DefaultConfig()
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Printf("config: failed to parse config file %s, using defaults: %v", path, err)
		return DefaultConfig()
	}

	if err := cfg.Validate(); err != nil {
		log.Printf("config: invalid config in %s, using defaults: %v", path, err)
		return DefaultConfig()
	}

	return &cfg
}

// Validate checks if the config has valid values
func (c *Config) Validate() error {
	if c.Preferences.MaxLogLines < 100 {
		return fmt.Errorf("max_log_lines must be at least 100, got %d", c.Preferences.MaxLogLines)
	}

	if c.Preferences.RefreshInterval < 1 {
		return fmt.Errorf("refresh_interval must be at least 1 second, got %d", c.Preferences.RefreshInterval)
	}

	return nil
}
