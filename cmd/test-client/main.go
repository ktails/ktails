package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ivyascorp-net/ktails/internal/k8s"
	"github.com/ivyascorp-net/ktails/internal/tui"
)

func main() {
	fmt.Println("Testing K8s Client Setup...")

	// Create client
	client, err := k8s.NewClient("")
	if err != nil {
		fmt.Printf("❌ Failed to create client: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Client created successfully")
	//  simple tui
	s := tui.NewSimpleTui(client)

	p := tea.NewProgram(
		s,
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
	)

	// Run program
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running ktails: %v\n", err)
		os.Exit(1)
	}
}
