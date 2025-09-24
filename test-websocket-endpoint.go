package main

import (
	"fmt"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	// Test WebSocket connection to Helix external agents endpoint
	u := url.URL{
		Scheme: "ws",
		Host:   "localhost:8080",
		Path:   "/api/v1/external-agents/sync",
	}
	
	// Add query parameters
	q := u.Query()
	q.Set("agent_id", "test-agent-12345")
	u.RawQuery = q.Encode()
	
	fmt.Printf("🔌 Connecting to: %s\n", u.String())
	
	// Set up headers
	header := make(map[string][]string)
	header["Authorization"] = []string{"Bearer oh-hallo-insecure-token"}
	
	// Connect to WebSocket
	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		fmt.Printf("❌ WebSocket connection failed: %v\n", err)
		if resp != nil {
			fmt.Printf("   HTTP Status: %s\n", resp.Status)
		}
		return
	}
	defer conn.Close()
	
	fmt.Printf("✅ WebSocket connected successfully!\n")
	fmt.Printf("   Response Status: %s\n", resp.Status)
	
	// Send a test message
	testMsg := map[string]interface{}{
		"type": "ping",
		"data": map[string]interface{}{
			"message": "test connection",
		},
	}
	
	if err := conn.WriteJSON(testMsg); err != nil {
		fmt.Printf("❌ Failed to send message: %v\n", err)
		return
	}
	
	fmt.Printf("📤 Sent test message\n")
	
	// Wait for response or timeout
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var response map[string]interface{}
	if err := conn.ReadJSON(&response); err != nil {
		fmt.Printf("⏰ No response received (timeout or error): %v\n", err)
	} else {
		fmt.Printf("📥 Received response: %+v\n", response)
	}
	
	fmt.Printf("🔌 Test completed\n")
}
