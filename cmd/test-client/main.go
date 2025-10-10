package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/ivyascorp-net/ktails/internal/k8s"
	v1 "k8s.io/api/core/v1"
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

	// Get current context
	currentCtx := client.GetCurrentContext()
	fmt.Printf("📍 Current context: %s\n", currentCtx)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Stream logs from specific container
	logOpts := &v1.PodLogOptions{
		Container:  "manager", // Specify container name
		Follow:     true,
		Timestamps: true,
		TailLines:  int64Ptr(50),
	}

	stream, err := client.StreamLogs(ctx, "cnpg-system", "cnpg-controller-manager-5c94bc644d-47zsm", logOpts)
	if err != nil {
		log.Fatalf("Failed to stream logs: %v", err)
	}
	defer stream.Close()

	// Copy logs to stdout
	if _, err := io.Copy(os.Stdout, stream); err != nil {
		log.Printf("Error copying logs: %v", err)
	}
}

// Helper function
func int64Ptr(i int64) *int64 {
	return &i
}
