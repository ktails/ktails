package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/ktails/ktails/internal/config"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/pages"
	"github.com/ktails/ktails/utils"
)

// Set via -ldflags "-X main.version=... -X main.commit=... -X main.date=..." by goreleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// setupLogging routes the standard log package's output away from
// os.Stderr, which the bubbletea program shares with the terminal it's
// rendering into. log's default output writes straight there, outside
// bubbletea's alt-screen render loop — any log.Printf call (e.g.
// pages.logSlowUpdate) would otherwise bleed raw text into the TUI and
// corrupt the frame. Debug logging (KTAILS_DEBUG=1) goes to a file instead;
// without it, log output is discarded entirely.
func setupLogging() (close func()) {
	if os.Getenv("KTAILS_DEBUG") == "" {
		log.SetOutput(io.Discard)
		return func() {}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		log.SetOutput(io.Discard)
		return func() {}
	}

	logDir := filepath.Join(home, ".config", "ktails")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.SetOutput(io.Discard)
		return func() {}
	}

	logPath := filepath.Join(logDir, "debug.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.SetOutput(io.Discard)
		return func() {}
	}

	log.SetOutput(f)
	return func() { f.Close() }
}

// Main Program
func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("ktails %s (commit %s, built %s)\n", version, commit, date)
			return
		}
	}

	kubeconfigFlag := flag.String("kubeconfig", "",
		"path to the kubeconfig file to use (overrides $KUBECONFIG, ~/.kube/config, and the config file's kubeconfig_path)")
	contextFlag := flag.String("context", "",
		"kubeconfig context to select on startup (overrides the kubeconfig's own current-context)")
	flag.Parse()

	closeLog := setupLogging()
	defer closeLog()

	// Config is loaded before the client so its kubeconfig_path can feed
	// into k8s.NewClient below — config.Load never fails (a broken config
	// file falls back to defaults with a logged warning), so there's
	// nothing to check here.
	cfg := config.Load("")

	// Precedence: --kubeconfig flag > config file's kubeconfig_path > k8s.
	// NewClient's own default resolution ($KUBECONFIG, then ~/.kube/config).
	kubeconfigPath := *kubeconfigFlag
	if kubeconfigPath == "" {
		kubeconfigPath = cfg.KubeconfigPath
	}

	client, err := k8s.NewClient(kubeconfigPath)
	if err != nil {
		fmt.Printf("❌ Failed to load kubeconfig: %v\n", err)
		os.Exit(1)
	}

	if *contextFlag != "" {
		if err := client.SetCurrentContext(*contextFlag); err != nil {
			fmt.Printf("❌ %v\n", err)
			os.Exit(1)
		}
	}

	mp := pages.NewMainPageModel(client, cfg.Preferences.RefreshInterval)

	p := tea.NewProgram(mp)
	if r, err := p.Run(); err != nil {
		utils.PrintJSON(r)
		panic(err)
	}
}
