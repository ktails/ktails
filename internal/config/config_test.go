package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoad_DefaultOnMissingFile guards the expected, common case: no config
// file has ever been written yet, and Load must return DefaultConfig()
// rather than an error.
func TestLoad_DefaultOnMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	got := Load(path)

	want := DefaultConfig()
	if got.Preferences != want.Preferences {
		t.Fatalf("Load(missing file) = %+v, want default %+v", got.Preferences, want.Preferences)
	}
	if len(got.RecentPods) != 0 {
		t.Fatalf("expected no recent pods in the default config, got %+v", got.RecentPods)
	}
}

// TestLoad_RoundTripsSaveAndLoad guards the basic persistence contract:
// whatever Save writes, Load must read back unchanged.
func TestLoad_RoundTripsSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	original := DefaultConfig()
	original.Preferences.Theme = "light"
	original.Preferences.MaxLogLines = 500
	original.Preferences.RefreshInterval = 10
	original.KubeconfigPath = "/custom/kubeconfig"
	original.AddRecentPod("ctx1", "default", "pod-a")

	if err := original.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got := Load(path)

	if got.Preferences != original.Preferences {
		t.Fatalf("round-tripped Preferences = %+v, want %+v", got.Preferences, original.Preferences)
	}
	if got.KubeconfigPath != original.KubeconfigPath {
		t.Fatalf("round-tripped KubeconfigPath = %q, want %q", got.KubeconfigPath, original.KubeconfigPath)
	}
	if len(got.RecentPods) != 1 || got.RecentPods[0].Pod != "pod-a" {
		t.Fatalf("round-tripped RecentPods = %+v, want one entry for pod-a", got.RecentPods)
	}
}

// TestLoad_FallsBackToDefaultOnInvalidTheme guards the regression this fix
// targets: a config file that fails Validate() (an unknown theme, here)
// must not stop the app from starting — Load falls back to
// DefaultConfig() instead of returning an error the caller would have to
// treat as fatal.
func TestLoad_FallsBackToDefaultOnInvalidTheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "preferences:\n  theme: not-a-real-theme\n  max_log_lines: 1000\n  refresh_interval: 5\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	got := Load(path)

	want := DefaultConfig()
	if got.Preferences.Theme != want.Preferences.Theme {
		t.Fatalf("expected the invalid theme to fall back to default %q, got %q", want.Preferences.Theme, got.Preferences.Theme)
	}
}

// TestLoad_FallsBackToDefaultOnUnparseableYAML guards the same
// never-brick-startup contract for a corrupted (not just semantically
// invalid) config file.
func TestLoad_FallsBackToDefaultOnUnparseableYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("{ not: valid: yaml: at: all"), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	got := Load(path)

	want := DefaultConfig()
	if got.Preferences != want.Preferences {
		t.Fatalf("expected unparseable YAML to fall back to defaults, got %+v", got.Preferences)
	}
}

// TestAddRecentPod_LRUBehavior guards the recent-pods list's three
// invariants: newest first, re-adding an existing entry moves it to the
// front rather than duplicating it, and the list is capped at 20.
func TestAddRecentPod_LRUBehavior(t *testing.T) {
	c := DefaultConfig()

	c.AddRecentPod("ctx1", "default", "pod-a")
	c.AddRecentPod("ctx1", "default", "pod-b")
	if len(c.RecentPods) != 2 || c.RecentPods[0].Pod != "pod-b" || c.RecentPods[1].Pod != "pod-a" {
		t.Fatalf("expected [pod-b, pod-a] (newest first), got %+v", c.RecentPods)
	}

	// Re-adding pod-a must move it to the front, not duplicate it.
	c.AddRecentPod("ctx1", "default", "pod-a")
	if len(c.RecentPods) != 2 || c.RecentPods[0].Pod != "pod-a" {
		t.Fatalf("expected re-adding pod-a to move it to the front without duplicating, got %+v", c.RecentPods)
	}

	// Cap at 20: adding 25 distinct pods must leave only the 20 most recent.
	c = DefaultConfig()
	for i := 0; i < 25; i++ {
		c.AddRecentPod("ctx1", "default", "pod-"+string(rune('a'+i)))
	}
	if len(c.RecentPods) != 20 {
		t.Fatalf("expected the recent-pods list capped at 20, got %d", len(c.RecentPods))
	}
	// The most recently added (pod index 24, the 25th) must be first.
	wantNewest := "pod-" + string(rune('a'+24))
	if c.RecentPods[0].Pod != wantNewest {
		t.Fatalf("expected %q first after the cap, got %q", wantNewest, c.RecentPods[0].Pod)
	}

	if !c.RecentPods[0].LastUsed.After(time.Time{}) {
		t.Fatal("expected LastUsed to be set to a real timestamp")
	}
}
