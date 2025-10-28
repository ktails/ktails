// Package config
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	// Preferences
	Preferences Preferences `yaml:"preferences"`

	// Recent pods for quick access
	RecentPods []RecentPod `yaml:"recent_pods"`

	// Kubeconfig path (defaults to ~/.kube/config)
	KubeconfigPath string `yaml:"kubeconfig_path"`
}

// Preferences contains user preferences
type Preferences struct {
	Theme           string `yaml:"theme"`             // "dark" or "light"
	FollowByDefault bool   `yaml:"follow_by_default"` // Auto-follow logs
	MaxLogLines     int    `yaml:"max_log_lines"`     // Max lines to keep in memory
	RefreshInterval int    `yaml:"refresh_interval"`  // Seconds between pod info refresh
	ShowTimestamps  bool   `yaml:"show_timestamps"`   // Show log timestamps
	ColorCodeLogs   bool   `yaml:"color_code_logs"`   // Color code log levels
	SyncScroll      bool   `yaml:"sync_scroll"`       // Sync scrolling between panes
}

// RecentPod represents a recently viewed pod
type RecentPod struct {
	Context   string    `yaml:"context"`
	Namespace string    `yaml:"namespace"`
	Pod       string    `yaml:"pod"`
	LastUsed  time.Time `yaml:"last_used"`
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		Preferences: Preferences{
			Theme:           "dark",
			FollowByDefault: true,
			MaxLogLines:     1000,
			RefreshInterval: 5,
			ShowTimestamps:  true,
			ColorCodeLogs:   true,
			SyncScroll:      false,
		},
		RecentPods:     make([]RecentPod, 0),
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

// EnsureConfigDir creates the config directory if it doesn't exist
func EnsureConfigDir() error {
	configPath, err := GetDefaultConfigPath()
	if err != nil {
		return err
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	return nil
}

// Load loads configuration from file
// If the file doesn't exist, returns default config
// If path is empty, uses default config path
func Load(path string) (*Config, error) {
	// Use default path if not specified
	if path == "" {
		defaultPath, err := GetDefaultConfigPath()
		if err != nil {
			return nil, fmt.Errorf("failed to get default config path: %w", err)
		}
		path = defaultPath
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// File doesn't exist, return default config
		return DefaultConfig(), nil
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// Save saves configuration to file
// If path is empty, uses default config path
func (c *Config) Save(path string) error {
	// Use default path if not specified
	if path == "" {
		defaultPath, err := GetDefaultConfigPath()
		if err != nil {
			return fmt.Errorf("failed to get default config path: %w", err)
		}
		path = defaultPath
	}

	// Ensure config directory exists
	if err := EnsureConfigDir(); err != nil {
		return err
	}

	// Marshal to YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file with proper permissions
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate checks if the config has valid values
func (c *Config) Validate() error {
	// Validate theme
	if c.Preferences.Theme != "dark" && c.Preferences.Theme != "light" {
		return fmt.Errorf("invalid theme: %s (must be 'dark' or 'light')", c.Preferences.Theme)
	}

	// Validate numeric values
	if c.Preferences.MaxLogLines < 100 {
		return fmt.Errorf("max_log_lines must be at least 100, got %d", c.Preferences.MaxLogLines)
	}

	if c.Preferences.RefreshInterval < 1 {
		return fmt.Errorf("refresh_interval must be at least 1 second, got %d", c.Preferences.RefreshInterval)
	}

	return nil
}

// AddRecentPod adds a pod to recent history
func (c *Config) AddRecentPod(context, namespace, pod string) {
	// Validate inputs
	if context == "" || namespace == "" || pod == "" {
		return
	}

	// Remove if already exists
	for i, rp := range c.RecentPods {
		if rp.Context == context && rp.Namespace == namespace && rp.Pod == pod {
			c.RecentPods = append(c.RecentPods[:i], c.RecentPods[i+1:]...)
			break
		}
	}

	// Add to front
	c.RecentPods = append([]RecentPod{
		{
			Context:   context,
			Namespace: namespace,
			Pod:       pod,
			LastUsed:  time.Now(),
		},
	}, c.RecentPods...)

	// Keep only last 20
	if len(c.RecentPods) > 20 {
		c.RecentPods = c.RecentPods[:20]
	}
}

// GetRecentPods returns recent pods, optionally filtered by context
func (c *Config) GetRecentPods(context string) []RecentPod {
	if context == "" {
		return c.RecentPods
	}

	filtered := make([]RecentPod, 0)
	for _, rp := range c.RecentPods {
		if rp.Context == context {
			filtered = append(filtered, rp)
		}
	}
	return filtered
}

// ClearRecentPods removes all recent pod entries
func (c *Config) ClearRecentPods() {
	c.RecentPods = make([]RecentPod, 0)
}

// RemoveRecentPod removes a specific pod from recent history
func (c *Config) RemoveRecentPod(context, namespace, pod string) {
	for i, rp := range c.RecentPods {
		if rp.Context == context && rp.Namespace == namespace && rp.Pod == pod {
			c.RecentPods = append(c.RecentPods[:i], c.RecentPods[i+1:]...)
			return
		}
	}
}