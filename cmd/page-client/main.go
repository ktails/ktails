package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/pages"
	"github.com/ktails/ktails/utils"
)

// Main Program
func main() {
	// Create client
	client, err := k8s.NewClient("")
	if err != nil {
		fmt.Printf("❌ Failed to create client: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Client created successfully")

	mp := pages.NewMainPageModel(client)

	p := tea.NewProgram(mp, tea.WithAltScreen())
	if r, err := p.Run(); err != nil {
		utils.PrintJSON(r)
		panic(err)
	}
}
