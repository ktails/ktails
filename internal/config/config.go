// Package config
package config

import (
	"time"
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

// Load loads configuration from file
func Load(path string) (*Config, error) {
	// TODO: Implement YAML loading
	return DefaultConfig(), nil
}

// Save saves configuration to file
func (c *Config) Save(path string) error {
	// TODO: Implement YAML saving
	return nil
}

// AddRecentPod adds a pod to recent history
func (c *Config) AddRecentPod(context, namespace, pod string) {
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
