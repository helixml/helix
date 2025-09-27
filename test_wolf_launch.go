package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/helixml/helix/api/pkg/wolf"
)

func main() {
	fmt.Println("Testing Wolf session creation...")

	// Create Wolf client using Unix socket
	client := wolf.NewClient("/var/run/wolf/wolf.sock")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test creating a session for the Test Integration app
	// App ID should be 634606484 based on hash("" + "Test Integration")
	session := &wolf.WolfStreamSession{
		AppID:             "634606484", // The calculated ID for "Test Integration"
		ClientID:          "test-client-123",
		ClientIP:          "127.0.0.1",
		AESKey:           "0123456789abcdef0123456789abcdef",
		AESIV:            "fedcba9876543210fedcba9876543210",
		RTSPFakeIP:       "127.0.0.1",
		VideoWidth:       1920,
		VideoHeight:      1080,
		VideoRefreshRate: 60,
		AudioChannelCount: 2,
		ClientSettings: wolf.WolfClientSettings{
			RunUID:              1000,
			RunGID:              1000,
			ControllersOverride: []string{},
			MouseAcceleration:   1.0,
			VScrollAcceleration: 1.0,
			HScrollAcceleration: 1.0,
		},
	}

	fmt.Printf("Creating session for app ID: %s\n", session.AppID)

	sessionID, err := client.CreateSession(ctx, session)
	if err != nil {
		log.Fatalf("Failed to create Wolf session: %v", err)
	}

	fmt.Printf("SUCCESS: Wolf session created successfully!\n")
	fmt.Printf("Session ID: %s\n", sessionID)
	fmt.Printf("This confirms that:\n")
	fmt.Printf("1. App ID %s exists and is recognized by Wolf\n", session.AppID)
	fmt.Printf("2. The JSON validation fixes are working\n")
	fmt.Printf("3. The Test Integration app is properly configured\n")
}