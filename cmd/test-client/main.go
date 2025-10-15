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

	var dump *os.File

	if _, ok := os.LookupEnv("KTAILS_DEBUG"); ok {
		var err error
		dump, err = os.OpenFile("messages.log", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			os.Exit(1)
		}
	}

	// Create client
	client, err := k8s.NewClient("")
	if err != nil {
		fmt.Printf("❌ Failed to create client: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Client created successfully")
	//  simple tui
	s := tui.NewSimpleTui(client)
	s.Dump = dump

	p := tea.NewProgram(
		s,
		// tea.WithAltScreen(),       // Use alternate screen buffer
		// tea.WithMouseCellMotion(), // Enable mouse support
	)

	// Run program
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running ktails: %v\n", err)
		os.Exit(1)
	}

	// pods, err := client.ListPodInfo("k3s-master-1", "immich-serverc")
	// if err != nil {
	// 	fmt.Printf("❌ Failed to list pods: %v\n", err)
	// 	os.Exit(1)
	// }
	// for _, pod := range pods {
	// 	fmt.Printf("Pod: %s, Namespace: %s, Status: %s, Restarts: %d, Age: %s, Image: %s, Container: %s, Node: %s, Context: %s\n",
	// 		pod.Name, pod.Namespace, pod.Status, pod.Restarts, pod.Age, pod.Image, pod.Container, pod.Node, pod.Context)
	// }
}
