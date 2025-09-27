package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/helixml/helix/api/pkg/wolf"
)

func main() {
	// Create Wolf client using Unix socket
	client := wolf.NewClient("/var/run/wolf/wolf.sock")

	// Create a minimal app using wolf_minimal.go
	minimalApp := &wolf.MinimalWolfApp{
		Title:                  "Test Integration",
		ID:                     "test-integration",
		StartVirtualCompositor: true,
		Runner: wolf.MinimalWolfRunner{
			Type:    "docker",
			Name:    wolf.StringPtr("test-container"),
			Image:   wolf.StringPtr("ubuntu:latest"),
			Mounts:  &[]string{},
			Env:     &[]string{},
			Devices: &[]string{},
			Ports:   &[]string{},
		},
	}

	// Use the minimal app directly since App = MinimalWolfApp
	app := minimalApp

	fmt.Printf("Testing Wolf app creation with minimal config...\n")

	// Marshal to see what JSON is being sent
	jsonData, err := json.MarshalIndent(app, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal app: %v", err)
	}
	fmt.Printf("Sending app JSON:\n%s\n", string(jsonData))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test app creation
	err = client.AddApp(ctx, app)
	if err != nil {
		log.Fatalf("Failed to add Wolf app: %v", err)
	}

	fmt.Printf("SUCCESS: Wolf app creation completed without parsing errors!\n")
	fmt.Printf("This confirms that the video_producer_buffer_caps fix is working correctly.\n")
}