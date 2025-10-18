package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/pages"
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

	mp := pages.NewMainModel(client)

	p := tea.NewProgram(mp)
	if _, err := p.Run(); err != nil {
		panic(err)
	}
}
