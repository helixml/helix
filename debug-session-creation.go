package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	helixAPIURL = "http://localhost:8080"
	userAPIKey  = "hl-cNtGODaSFKhKzgOuNACEZAoUEv0Mpdx_eb7xkVl7ZJo="
	appID       = "app_01k5qka10zk6fp4daw3pjwv7xz"
)

type Message struct {
	Role    string         `json:"role"`
	Content MessageContent `json:"content"`
}

type MessageContent struct {
	ContentType string        `json:"content_type"`
	Parts       []interface{} `json:"parts"`
}

type CreateSessionRequest struct {
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Mode      string    `json:"mode"`
	ModelName string    `json:"model_name"`
	AppID     string    `json:"app_id,omitempty"`
	Stream    bool      `json:"stream,omitempty"`
	AgentType string    `json:"agent_type,omitempty"`
	Messages  []Message `json:"messages"`
}

func main() {
	fmt.Println("🔍 Debug: Testing Helix session creation with external agent")

	// Create session request
	sessionReq := CreateSessionRequest{
		Name:      "Debug Test Session",
		Type:      "text",
		Mode:      "inference",
		ModelName: "claude-3.5-sonnet",
		AppID:     appID,
		Stream:    false,
		AgentType: "zed_external",
		Messages: []Message{
			{
				Role: "system",
				Content: MessageContent{
					ContentType: "text",
					Parts:       []interface{}{"Debug test message"},
				},
			},
		},
	}

	reqBody, err := json.Marshal(sessionReq)
	if err != nil {
		fmt.Printf("❌ Failed to marshal request: %v\n", err)
		return
	}

	fmt.Printf("📤 Request body: %s\n", string(reqBody))

	req, err := http.NewRequest("POST", helixAPIURL+"/api/v1/sessions/chat", bytes.NewBuffer(reqBody))
	if err != nil {
		fmt.Printf("❌ Failed to create request: %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Failed to send request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("📊 Response status: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Println("📋 Response headers:")
	for key, values := range resp.Header {
		for _, value := range values {
			fmt.Printf("   %s: %s\n", key, value)
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read response: %v\n", err)
		return
	}

	fmt.Printf("📝 Raw response body: %s\n", string(body))

	// Try to parse as JSON
	var jsonResp map[string]interface{}
	if err := json.Unmarshal(body, &jsonResp); err != nil {
		fmt.Printf("❌ JSON parse error: %v\n", err)
		fmt.Printf("🔍 First few characters: %q\n", string(body[:min(50, len(body))]))
	} else {
		fmt.Printf("✅ Successfully parsed JSON: %+v\n", jsonResp)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
