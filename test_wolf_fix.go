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
	fmt.Println("Testing Wolf pipeline fix with video_producer_buffer_caps...")

	// Create Wolf client using Unix socket (from container perspective)
	client := wolf.NewClient("/var/run/wolf/wolf.sock")

	// Test app creation using our fixed NewMinimalDockerApp
	app := wolf.NewMinimalDockerApp(
		"test-pipeline-fix",                 // ID
		"Pipeline Fix Test",                 // Title
		"PipelineTest",                      // Name
		"ubuntu:latest",                     // Image
		[]string{"TEST_ENV=pipeline_fix"},   // Env
		[]string{},                          // Mounts
		"{}",                                // BaseCreateJSON
	)

	fmt.Printf("Testing app creation with video_producer_buffer_caps field...\n")

	jsonData, err := json.MarshalIndent(app, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal app: %v", err)
	}
	fmt.Printf("App JSON (should include video_producer_buffer_caps):\n%s\n\n", string(jsonData))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test app creation
	err = client.AddApp(ctx, app)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		fmt.Printf("This suggests there's still an issue with the Wolf integration\n")
	} else {
		fmt.Printf("SUCCESS: Wolf app creation worked!\n")
		fmt.Printf("The video_producer_buffer_caps field should now be included and prevent GStreamer syntax errors\n")
	}
}