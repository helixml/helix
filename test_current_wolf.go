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
	fmt.Println("Testing current Wolf API behavior...")

	// Create Wolf client using Unix socket
	client := wolf.NewClient("/var/run/wolf/wolf.sock")

	// Test the minimal app that should work if video_producer_buffer_caps is properly handled
	minimalApp := &wolf.MinimalWolfApp{
		Title:                  "API Test App",
		ID:                     "api-test-app",
		StartVirtualCompositor: true,
		Runner: wolf.MinimalWolfRunner{
			Type:    "docker",
			Name:    wolf.StringPtr("api-test"),
			Image:   wolf.StringPtr("ubuntu:latest"),
			Mounts:  &[]string{},
			Env:     &[]string{},
			Devices: &[]string{},
			Ports:   &[]string{},
		},
	}

	fmt.Printf("Testing Wolf API with app that would trigger video_producer_buffer_caps validation...\n")

	jsonData, err := json.MarshalIndent(minimalApp, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal app: %v", err)
	}
	fmt.Printf("Sending app JSON:\n%s\n", string(jsonData))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test app creation - this will show if video_producer_buffer_caps validation is working
	err = client.AddApp(ctx, minimalApp)
	if err != nil {
		// If this fails with "video_producer_buffer_caps not found", then our fixes aren't compiled in
		fmt.Printf("FAILED: %v\n", err)
		if jsonErr, ok := err.(*json.SyntaxError); ok {
			fmt.Printf("JSON parsing error at offset %d\n", jsonErr.Offset)
		}
		fmt.Printf("This suggests our source code fixes are NOT compiled into the Wolf binary\n")
	} else {
		fmt.Printf("SUCCESS: Wolf app creation worked!\n")
		fmt.Printf("This suggests either:\n")
		fmt.Printf("1. Our fixes ARE compiled in, OR\n")
		fmt.Printf("2. The original Wolf binary already handles this case somehow\n")
	}
}