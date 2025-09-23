package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	fmt.Println("🧪 Direct WebSocket Test - Testing Zed Response")
	fmt.Println("===============================================")

	// Connect to Helix WebSocket as external agent
	wsURL := "ws://localhost:8080/api/v1/external-agents/sync?session_id=test-session-123"
	header := make(http.Header)
	header.Set("Authorization", "Bearer oh-hallo-insecure-token")

	fmt.Printf("🔗 Connecting to: %s\n", wsURL)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		log.Fatalf("❌ Failed to connect: %v", err)
	}
	defer conn.Close()

	fmt.Println("✅ Connected to Helix WebSocket")
	time.Sleep(2 * time.Second)

	// Send a chat_message command
	command := map[string]interface{}{
		"command_type": "chat_message",
		"data": map[string]interface{}{
			"request_id": "test-req-123",
			"message":    "Hello Zed! This is a direct test message.",
			"role":       "user",
		},
	}

	fmt.Println("📤 Sending chat_message command to Zed...")
	commandJSON, _ := json.MarshalIndent(command, "", "  ")
	fmt.Printf("Command: %s\n", commandJSON)

	if err := conn.WriteJSON(command); err != nil {
		log.Fatalf("❌ Failed to send command: %v", err)
	}

	fmt.Println("✅ Command sent, waiting for response...")

	// Listen for response with timeout
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for i := 0; i < 5; i++ {
		var response map[string]interface{}
		if err := conn.ReadJSON(&response); err != nil {
			log.Printf("❌ Failed to read response: %v", err)
			break
		}

		fmt.Printf("📥 Response %d received:\n", i+1)
		responseJSON, _ := json.MarshalIndent(response, "", "  ")
		fmt.Printf("%s\n", responseJSON)
		fmt.Println("---")

		// Check if this is the done signal
		if eventType, ok := response["event_type"].(string); ok && eventType == "chat_response_done" {
			fmt.Println("🎉 Received chat_response_done - conversation complete!")
			break
		}
	}

	fmt.Println("🔚 Test completed")
}
