package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type MessageContent struct {
	ContentType string        `json:"contentType"`
	Parts       []interface{} `json:"parts"`
}

type ChatRequest struct {
	Messages  []Message `json:"messages"`
	Stream    bool      `json:"stream"`
	AgentType string    `json:"agent_type"`
}

func main() {
	fmt.Println("🧪 Direct Chat API Test - Testing Zed External Agent")
	fmt.Println("====================================================")

	// Create a chat request with agent_type: "zed_external"
	chatReq := ChatRequest{
		Messages: []Message{
			{
				Role: "user",
				Content: MessageContent{
					ContentType: "text",
					Parts:       []interface{}{"Hello Zed! This is a test message via Helix Chat API. Please respond."},
				},
			},
		},
		Stream:    false, // Use non-streaming for simplicity
		AgentType: "zed_external",
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		log.Fatalf("❌ Failed to marshal request: %v", err)
	}

	fmt.Println("📤 Sending chat request to Helix Chat API...")
	fmt.Printf("Request body: %s\n", string(reqBody))

	// Send POST request to Helix Chat API
	req, err := http.NewRequest("POST", "http://localhost:8080/api/v1/sessions/chat", bytes.NewBuffer(reqBody))
	if err != nil {
		log.Fatalf("❌ Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer oh-hallo-insecure-token")

	client := &http.Client{Timeout: 35 * time.Second} // Longer than Helix's 30s timeout
	fmt.Println("⏳ Waiting for response (up to 35 seconds)...")
	
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("❌ Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	fmt.Printf("📥 Response status: %s\n", resp.Status)
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("❌ Failed to read response: %v", err)
	}

	fmt.Printf("📥 Response body: %s\n", string(body))

	if resp.StatusCode == 200 {
		fmt.Println("🎉 SUCCESS! Helix Chat API worked with zed_external agent!")
	} else {
		fmt.Printf("❌ FAILED! Status: %d, Body: %s\n", resp.StatusCode, string(body))
	}

	fmt.Println("🔚 Test completed")
}
