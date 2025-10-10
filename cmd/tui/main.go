package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ivyascorp-net/ktails/internal/k8s"
	"github.com/ivyascorp-net/ktails/internal/tui"
)

func main() {
	// Create K8s client
	client, err := k8s.NewClient("")
	if err != nil {
		fmt.Printf("❌ Failed to initialize Kubernetes client:\n%v\n\n", err)
		fmt.Println("Please ensure:")
		fmt.Println("  • kubeconfig exists at ~/.kube/config or $KUBECONFIG")
		fmt.Println("  • You have access to a Kubernetes cluster")
		os.Exit(1)
	}

	// Create TUI model with K8s client
	m := tui.NewModel(client)

	// Set global client for commands that run in background
	tui.SetK8sClient(client)

	// Create program with options
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
	)

	// Run program
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running ktails: %v\n", err)
		os.Exit(1)
	}
}
