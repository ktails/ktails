package config

import (
	"os"
	"path/filepath"
	"testing"
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
}

// TestLoad_RoundTripsFileContents guards the basic parsing contract: valid
// YAML on disk is read back into the matching Preferences/KubeconfigPath.
func TestLoad_RoundTripsFileContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "preferences:\n  max_log_lines: 500\n  refresh_interval: 10\nkubeconfig_path: /custom/kubeconfig\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	got := Load(path)

	want := Preferences{MaxLogLines: 500, RefreshInterval: 10}
	if got.Preferences != want {
		t.Fatalf("round-tripped Preferences = %+v, want %+v", got.Preferences, want)
	}
	if got.KubeconfigPath != "/custom/kubeconfig" {
		t.Fatalf("round-tripped KubeconfigPath = %q, want %q", got.KubeconfigPath, "/custom/kubeconfig")
	}
}

// TestLoad_FallsBackToDefaultOnInvalidMaxLogLines guards the regression this
// fix targets: a config file that fails Validate() (max_log_lines below the
// floor, here) must not stop the app from starting — Load falls back to
// DefaultConfig() instead of returning an error the caller would have to
// treat as fatal.
func TestLoad_FallsBackToDefaultOnInvalidMaxLogLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "preferences:\n  max_log_lines: 1\n  refresh_interval: 5\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	got := Load(path)

	want := DefaultConfig()
	if got.Preferences != want.Preferences {
		t.Fatalf("expected the invalid config to fall back to defaults %+v, got %+v", want.Preferences, got.Preferences)
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
