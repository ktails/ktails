package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
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

// Main Program
func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("ktails %s (commit %s, built %s)\n", version, commit, date)
			return
		}
	}

	// Create client
	client, err := k8s.NewClient("")
	if err != nil {
		fmt.Printf("❌ Failed to create client: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Client created successfully")

	cfg, err := config.Load("")
	if err != nil {
		fmt.Printf("❌ Failed to load config: %v\n", err)
		os.Exit(1)
	}

	mp := pages.NewMainPageModel(client, cfg.Preferences.RefreshInterval)

	p := tea.NewProgram(mp, tea.WithAltScreen())
	if r, err := p.Run(); err != nil {
		utils.PrintJSON(r)
		panic(err)
	}
}
