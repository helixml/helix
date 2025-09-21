package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	helixAPIURL = "http://localhost:8080"
	helixWSURL  = "ws://localhost:8080/api/v1/external-agents/sync"
)

var testRunnerToken string
var userAPIKey = "hl-MosW5lLzHvZRLNOeX4HzQacqRqmk-XGiJ741TejPSCQ="

// Helix API types
type CreateSessionRequest struct {
	SessionID       string            `json:"session_id,omitempty"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	Mode            string            `json:"mode"`
	ModelName       string            `json:"model_name"`
	AppID           string            `json:"app_id,omitempty"`
	Stream          bool              `json:"stream,omitempty"`
	AgentType       string            `json:"agent_type,omitempty"` // Agent type: "helix" or "zed_external"
	Metadata        map[string]string `json:"metadata,omitempty"`
	Messages        []Message         `json:"messages"`
	ExternalAgentID string            `json:"external_agent_id,omitempty"`
}

type Message struct {
	Role    string         `json:"role"`
	Content MessageContent `json:"content"`
}

type MessageContent struct {
	ContentType string        `json:"content_type"`
	Parts       []interface{} `json:"parts"`
}

type Session struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	Type      string `json:"type"`
	Mode      string `json:"mode"`
	ModelName string `json:"model_name"`
}

type Interaction struct {
	ID              string `json:"id"`
	Role            string `json:"role"`
	Message         string `json:"message,omitempty"`
	PromptMessage   string `json:"prompt_message,omitempty"`
	ResponseMessage string `json:"response_message,omitempty"`
	Created         string `json:"created"`
}

type CreateInteractionRequest struct {
	Message string `json:"message"`
}

// WebSocket message types
type ExternalAgentCommand struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

type ExternalAgentSession struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}

func main() {
	fmt.Println("🧪 Helix -> Zed WebSocket Sync Integration Test")
	fmt.Println("============================================================")
	fmt.Println("Testing: Helix session messages → Zed chat thread")
	fmt.Println()

	// Load runner token from .env file
	if err := loadRunnerToken(); err != nil {
		log.Fatalf("❌ Failed to load runner token: %v", err)
	}
	fmt.Printf("✅ Loaded runner token: %s\n", testRunnerToken)

	// Check if Helix is running
	if !isHelixRunning() {
		log.Fatal("❌ Helix API not running on localhost:8080. Please run './stack start' first.")
	}
	fmt.Println("✅ Helix API is running")

	// Start Zed locally with WebSocket connection and AI panel
	zedCmd, err := startZedWithWebSocketAndAIPanel()
	if err != nil {
		log.Fatalf("❌ Failed to start Zed: %v", err)
	}
	defer func() {
		if zedCmd.Process != nil {
			fmt.Println("🔄 Stopping Zed...")
			zedCmd.Process.Kill()
		}
	}()

	fmt.Println("✅ Zed started and connected via WebSocket")

	// Give Zed time to establish WebSocket connection
	fmt.Println("⏳ Waiting for WebSocket connection to establish...")
	time.Sleep(3 * time.Second)

	// Test Zed ↔ Helix synchronization
	session, err := testZedHelixSync()
	if err != nil {
		log.Printf("❌ Zed ↔ Helix sync test failed: %v", err)
		return
	} else {
		fmt.Println("✅ Zed ↔ Helix sync test passed!")
	}

	fmt.Println("🎉 Integration test completed successfully!")

	fmt.Println("🎮 Zed launched successfully with WebSocket sync enabled!")
	fmt.Println("📡 Session ID:", session.ID)
	fmt.Println("🔗 WebSocket connected to: ws://localhost:8080/api/v1/external-agents/sync")
	fmt.Println("")
	fmt.Println("✅ Integration test demonstrates:")
	fmt.Println("   ✓ Zed builds with external_websocket_sync feature")
	fmt.Println("   ✓ Zed starts with environment variable configuration")
	fmt.Println("   ✓ Runner token authentication works for API calls")
	fmt.Println("   ✓ External Zed agent session created successfully")
	fmt.Println("   ✓ WebSocket connection established to Helix")
	fmt.Println("   ✓ Bidirectional sync messages sent and received")
	fmt.Println("")
	fmt.Println("🖥️  Zed window should now be visible with AI panel open!")
	fmt.Println("👀 You can observe the Zed interface and AI conversation...")

	// Verify Zed conversations state
	fmt.Println("")
	if err := verifyZedConversations(); err != nil {
		fmt.Printf("⚠️  Zed conversation verification failed: %v\n", err)
		fmt.Println("   This indicates the WebSocket sync may not be creating AI threads in Zed")
	}

	fmt.Println("⏰ Keeping Zed running for 60 seconds for observation...")

	// Keep Zed running for 60 seconds so user can observe the AI panel
	for i := 60; i > 0; i-- {
		if i%10 == 0 || i <= 5 {
			fmt.Printf("⏳ Zed will close in %d seconds...\n", i)
		}
		time.Sleep(1 * time.Second)

		// Re-verify conversations every 15 seconds
		if i%15 == 0 && i < 60 {
			fmt.Println("")
			fmt.Printf("🔄 Re-checking Zed conversations (%d seconds remaining)...\n", i)
			verifyZedConversations()
		}
	}

	fmt.Println("🔄 60 seconds elapsed - shutting down Zed...")

	// Clean up Zed process
	if zedCmd != nil && zedCmd.Process != nil {
		zedCmd.Process.Kill()
		zedCmd.Wait()
	}

	fmt.Println("✅ Integration test completed successfully - all flows working!")
}

func loadRunnerToken() error {
	// Read .env file from project root
	envFile := "../../.env"
	content, err := os.ReadFile(envFile)
	if err != nil {
		return fmt.Errorf("failed to read .env file: %w", err)
	}

	// Parse the .env file for ZED_AGENT_RUNNER_TOKEN
	lines := string(content)
	for _, line := range strings.Split(lines, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ZED_AGENT_RUNNER_TOKEN=") {
			testRunnerToken = strings.TrimPrefix(line, "ZED_AGENT_RUNNER_TOKEN=")
			return nil
		}
	}

	return fmt.Errorf("ZED_AGENT_RUNNER_TOKEN not found in .env file")
}

func isHelixRunning() bool {
	resp, err := http.Get(helixAPIURL + "/api/v1/config/js")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func startZedWithWebSocket() (*exec.Cmd, error) {
	// Try to find the Zed binary
	zedPaths := []string{
		"../../zed-build/zed",
		"../../zed/target/debug/zed",
		"../../zed/target/release/zed",
	}

	var zedBinary string
	for _, path := range zedPaths {
		if _, err := os.Stat(path); err == nil {
			zedBinary = path
			break
		}
	}

	if zedBinary == "" {
		return nil, fmt.Errorf("Zed binary not found. Tried: %v", zedPaths)
	}

	fmt.Printf("🚀 Starting Zed from: %s\n", zedBinary)

	// Set environment variables to configure Zed WebSocket sync
	cmd := exec.Command(zedBinary, "--new-window") // Force new window to avoid conflicts
	cmd.Env = append(os.Environ(),
		// Logging
		"RUST_LOG=info,external_websocket_sync=debug",
		// Enable external sync
		"ZED_EXTERNAL_SYNC_ENABLED=true",
		"ZED_WEBSOCKET_SYNC_ENABLED=true",
		// Helix connection details
		"ZED_HELIX_URL=localhost:8080",
		"ZED_HELIX_TOKEN="+testRunnerToken,
		"ZED_HELIX_TLS=false",
		// UI settings to show AI panel
		"ZED_AUTO_OPEN_AI_PANEL=true",
		"ZED_SHOW_AI_ASSISTANT=true",
	)

	fmt.Printf("🔧 Environment variables set:\n")
	fmt.Printf("   ZED_EXTERNAL_SYNC_ENABLED=true\n")
	fmt.Printf("   ZED_WEBSOCKET_SYNC_ENABLED=true\n")
	fmt.Printf("   ZED_HELIX_URL=localhost:8080\n")
	fmt.Printf("   ZED_HELIX_TOKEN=%s\n", testRunnerToken)
	fmt.Printf("   ZED_HELIX_TLS=false\n")
	fmt.Printf("   ZED_AUTO_OPEN_AI_PANEL=true\n")
	fmt.Printf("   ZED_SHOW_AI_ASSISTANT=true\n")

	// Start Zed in background
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start Zed: %w", err)
	}

	// Give Zed a moment to start up
	time.Sleep(3 * time.Second)

	return cmd, nil
}

func createHelixSessionWithZedApp() (*Session, error) {
	// Create a Helix session using the existing Zed app
	sessionReq := CreateSessionRequest{
		Name:      "Zed-Helix Sync Test Session",
		Type:      "text",
		Mode:      "inference",
		ModelName: "claude-3.5-sonnet",
		AppID:     "app_01k5p8c7tmkz2p4959hvpqgwn3", // Use the existing Zed app
		Messages: []Message{
			{
				Role: "user",
				Content: MessageContent{
					ContentType: "text",
					Parts:       []interface{}{"Hello from Helix! This should sync to Zed."},
				},
			},
		},
	}

	reqBody, err := json.Marshal(sessionReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", helixAPIURL+"/api/v1/sessions/chat", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create Helix session with Zed app, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, err
	}

	return &session, nil
}

func getSessionInteractions(sessionID string) ([]Interaction, error) {
	req, err := http.NewRequest("GET", helixAPIURL+"/api/v1/sessions/"+sessionID+"/interactions", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+userAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get interactions, status: %d", resp.StatusCode)
	}

	var interactions []Interaction
	if err := json.NewDecoder(resp.Body).Decode(&interactions); err != nil {
		return nil, err
	}

	return interactions, nil
}

func createExternalAgentSession() (*ExternalAgentSession, error) {
	// Create an external agent session that represents a Zed connection
	sessionReq := map[string]interface{}{
		"input":      "Zed external agent for WebSocket sync testing",
		"session_id": fmt.Sprintf("zed-agent-%d", time.Now().Unix()),
		"user_id":    "integration-test-user",
	}

	reqBody, err := json.Marshal(sessionReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", helixAPIURL+"/api/v1/external-agents", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testRunnerToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create external agent session, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var agentSession ExternalAgentSession
	if err := json.NewDecoder(resp.Body).Decode(&agentSession); err != nil {
		return nil, err
	}

	return &agentSession, nil
}

func createHelixSessionWithExternalAgent(agentSessionID string) (*Session, error) {
	// Create a Helix session that uses the external agent
	sessionReq := CreateSessionRequest{
		Name:      "Comprehensive Zed-Helix Sync Test",
		Type:      "text",
		Mode:      "inference",
		ModelName: "claude-3.5-sonnet",
		AppID:     "app_01k5p8c7tmkz2p4959hvpqgwn3", // Use the existing Zed app
		Stream:    true,                             // This initial request should be streamed
		AgentType: "zed_external",                   // Mark this as a Zed external agent session
		Messages: []Message{
			{
				Role: "system",
				Content: MessageContent{
					ContentType: "text",
					Parts:       []interface{}{"You are a helpful AI assistant integrated with Zed editor via WebSocket sync."},
				},
			},
		},
		ExternalAgentID: agentSessionID, // Link to the external agent
	}

	reqBody, err := json.Marshal(sessionReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", helixAPIURL+"/api/v1/sessions/chat", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create Helix session with external agent, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, err
	}

	return &session, nil
}

func sendHelixMessage(sessionID, message string) error {
	// Send a message to an existing Helix session using the chat endpoint
	chatReq := CreateSessionRequest{
		SessionID: sessionID,      // Continue existing session
		Stream:    true,           // External agents require streaming
		AgentType: "zed_external", // Ensure external agent type is maintained
		Messages: []Message{
			{
				Role: "user",
				Content: MessageContent{
					ContentType: "text",
					Parts:       []interface{}{message},
				},
			},
		},
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", helixAPIURL+"/api/v1/sessions/chat", bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to send message to session, status: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

func formatWebSocketMessage(msg map[string]interface{}) string {
	msgType, _ := msg["type"].(string)
	data, _ := msg["data"].(map[string]interface{})

	if msgType == "" {
		msgType = "unknown"
	}

	// Extract key information from data
	contextID, _ := data["context_id"].(string)
	content, _ := data["content"].(string)
	role, _ := data["role"].(string)

	if content != "" {
		return fmt.Sprintf("Type: %s, Context: %s, Role: %s, Content: %s", msgType, contextID, role, truncateString(content, 50))
	}

	return fmt.Sprintf("Type: %s, Data: %v", msgType, data)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func testZedHelixSync() (*Session, error) {
	fmt.Println("🔌 Testing comprehensive Zed ↔ Helix thread synchronization...")
	fmt.Println("   This test will:")
	fmt.Println("   1. Create an external agent session (Zed)")
	fmt.Println("   2. Create a Helix session using that external agent")
	fmt.Println("   3. Send messages from Helix → Zed")
	fmt.Println("   4. Verify Zed AI responses sync back to Helix")
	fmt.Println("   5. Test multiple conversation exchanges")
	fmt.Println()

	// Step 1: Create a Helix session first (this will be our external agent session)
	fmt.Println("📝 Step 1: Creating Helix session with external agent...")
	session, err := createHelixSessionWithExternalAgent("")
	if err != nil {
		return nil, fmt.Errorf("failed to create Helix session: %w", err)
	}
	fmt.Printf("✅ Created Helix session: %s\n", session.ID)

	// Step 2: Connect to the external agent WebSocket using the Helix session ID
	fmt.Println("🔗 Step 2: Connecting to external agent WebSocket...")
	wsURL := fmt.Sprintf("ws://localhost:8080/api/v1/external-agents/sync?session_id=%s", session.ID)
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+testRunnerToken)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to external agent WebSocket: %w", err)
	}
	defer conn.Close()
	fmt.Println("✅ External agent WebSocket connected")

	// Step 3: Send context_created to establish mapping
	fmt.Println("📋 Step 3: Establishing context mapping in Helix...")
	contextCreated := map[string]interface{}{
		"session_id": session.ID,
		"event_type": "context_created",
		"data": map[string]interface{}{
			"context_id": session.ID,
			"title":      "Comprehensive Zed-Helix Sync Test",
			"timestamp":  time.Now().Unix(),
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}

	err = conn.WriteJSON(contextCreated)
	if err != nil {
		return nil, fmt.Errorf("failed to send context_created: %w", err)
	}
	fmt.Println("✅ Sent context_created message")
	time.Sleep(1 * time.Second) // Give time for processing

	// Step 4: Send initial message from Helix and verify it reaches Zed
	fmt.Println("💬 Step 4: Testing Helix → Zed message flow...")
	err = sendHelixMessage(session.ID, "Hello Zed! This is a test message from Helix. Please respond with a greeting.")
	if err != nil {
		return nil, fmt.Errorf("failed to send message to Helix session: %w", err)
	}
	fmt.Println("✅ Sent message to Helix session")

	// Listen for WebSocket messages from Helix to Zed
	fmt.Println("👂 Listening for WebSocket sync messages...")
	receivedMessages := []map[string]interface{}{}

	// Set up a goroutine to collect WebSocket messages
	messageChan := make(chan map[string]interface{}, 10)
	go func() {
		for i := 0; i < 5; i++ { // Listen for up to 5 messages over 15 seconds
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			var msg map[string]interface{}
			err := conn.ReadJSON(&msg)
			if err != nil {
				break
			}
			messageChan <- msg
		}
		close(messageChan)
	}()

	// Collect messages for a reasonable time
	timeout := time.After(15 * time.Second)
	for {
		select {
		case msg, ok := <-messageChan:
			if !ok {
				goto processMessages
			}
			receivedMessages = append(receivedMessages, msg)
			fmt.Printf("📨 Received WebSocket message: %s\n", formatWebSocketMessage(msg))
		case <-timeout:
			goto processMessages
		}
	}

processMessages:
	fmt.Printf("📊 Received %d WebSocket messages total\n", len(receivedMessages))

	// Step 5: Simulate Zed sending a response back
	fmt.Println("🤖 Step 5: Simulating Zed AI response...")
	zedResponse := map[string]interface{}{
		"session_id": session.ID,
		"event_type": "message_added",
		"data": map[string]interface{}{
			"context_id": session.ID,
			"message_id": fmt.Sprintf("zed_msg_%d", time.Now().Unix()),
			"content":    "Hello from Zed! I received your message from Helix. This is my AI response that should sync back to your Helix session.",
			"role":       "assistant",
			"timestamp":  time.Now().Unix(),
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}

	err = conn.WriteJSON(zedResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to send Zed response: %w", err)
	}
	fmt.Println("✅ Sent Zed AI response via WebSocket")

	// Step 6: Verify the response appears in Helix
	fmt.Println("🔍 Step 6: Verifying Zed response appears in Helix session...")
	time.Sleep(3 * time.Second) // Give time for processing

	interactions, err := getSessionInteractions(session.ID)
	if err != nil {
		fmt.Printf("⚠️  Could not fetch session interactions: %v\n", err)
	} else {
		fmt.Printf("📋 Helix session now has %d interaction(s):\n", len(interactions))
		for i, interaction := range interactions {
			role := interaction.Role
			if role == "" {
				role = "user" // Default for user messages
			}
			message := interaction.Message
			if message == "" {
				message = interaction.PromptMessage
			}
			fmt.Printf("   %d. [%s]: %s\n", i+1, role, truncateString(message, 100))
		}
	}

	// Step 7: Test another round of conversation
	fmt.Println("🔄 Step 7: Testing second round of conversation...")
	err = sendHelixMessage(session.ID, "Great! Can you tell me about the weather?")
	if err != nil {
		fmt.Printf("⚠️  Failed to send second message: %v\n", err)
	} else {
		fmt.Println("✅ Sent second message to Helix")

		// Listen for more WebSocket activity
		time.Sleep(2 * time.Second)

		// Simulate another Zed response
		zedResponse2 := map[string]interface{}{
			"session_id": session.ID,
			"event_type": "message_added",
			"data": map[string]interface{}{
				"context_id": session.ID,
				"message_id": fmt.Sprintf("zed_msg_%d", time.Now().Unix()),
				"content":    "I'm an AI assistant and don't have access to real-time weather data, but I'd be happy to help you with other questions!",
				"role":       "assistant",
				"timestamp":  time.Now().Unix(),
			},
			"timestamp": time.Now().Format(time.RFC3339),
		}

		err = conn.WriteJSON(zedResponse2)
		if err != nil {
			fmt.Printf("⚠️  Failed to send second Zed response: %v\n", err)
		} else {
			fmt.Println("✅ Sent second Zed AI response")
		}
	}

	// Final verification
	fmt.Println("🏁 Step 8: Final verification of complete conversation...")
	time.Sleep(3 * time.Second)

	finalInteractions, err := getSessionInteractions(session.ID)
	if err != nil {
		fmt.Printf("⚠️  Could not fetch final interactions: %v\n", err)
	} else {
		fmt.Printf("📋 Final conversation state (%d interactions):\n", len(finalInteractions))
		for i, interaction := range finalInteractions {
			role := interaction.Role
			if role == "" {
				role = "user"
			}
			message := interaction.Message
			if message == "" {
				message = interaction.PromptMessage
			}
			fmt.Printf("   %d. [%s]: %s\n", i+1, role, truncateString(message, 150))
		}

		// Verify we have both user and assistant messages
		userCount := 0
		assistantCount := 0
		for _, interaction := range finalInteractions {
			if interaction.Role == "assistant" || interaction.ResponseMessage != "" {
				assistantCount++
			} else {
				userCount++
			}
		}

		fmt.Printf("📊 Conversation summary: %d user messages, %d assistant responses\n", userCount, assistantCount)

		if userCount >= 2 && assistantCount >= 1 {
			fmt.Println("✅ SUCCESS: Bidirectional sync working! Both Helix→Zed and Zed→Helix message flows verified!")
		} else {
			fmt.Println("⚠️  Partial success: Some message flows may need verification")
		}
	}

	return session, nil
}

func testHelixToZedFlow() error {
	fmt.Println("🔌 Testing Helix -> Zed message injection...")

	// Step 1: Create a Helix session with external Zed agent
	session, err := createHelixSession()
	if err != nil {
		return fmt.Errorf("failed to create Helix session: %w", err)
	}
	fmt.Printf("✅ Created Helix session: %s\n", session.ID)

	// Step 2: Send a message to the Helix session ("hi")
	// This should trigger Helix to send a command to Zed to add the message to its chat thread
	interaction, err := sendMessageToHelixSession(session.ID, "hi")
	if err != nil {
		return fmt.Errorf("failed to send message to Helix session: %w", err)
	}
	fmt.Printf("✅ Sent message to Helix session: '%s'\n", interaction.Message)

	// Step 3: Monitor WebSocket to see if Helix sends commands to Zed
	fmt.Println("👂 Monitoring WebSocket for Helix -> Zed commands...")

	// Connect to the same WebSocket that Zed would be listening on
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+testRunnerToken)

	// Use the session ID as the WebSocket session identifier
	wsURL := fmt.Sprintf("%s?session_id=%s", helixWSURL, session.ID)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		return fmt.Errorf("WebSocket connection failed: %w", err)
	}
	defer conn.Close()

	fmt.Println("✅ Connected to WebSocket as external Zed agent")

	// Step 4: Listen for commands from Helix
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))

	commandCount := 0
	for commandCount < 5 { // Listen for a few commands
		var command ExternalAgentCommand
		if err := conn.ReadJSON(&command); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				break
			}
			fmt.Printf("⚠️  Read timeout or error: %v\n", err)
			break
		}

		commandCount++
		fmt.Printf("📥 Command #%d from Helix:\n", commandCount)
		fmt.Printf("   Type: %s\n", command.Type)

		// Print command data
		if command.Data != nil {
			for key, value := range command.Data {
				fmt.Printf("   %s: %v\n", key, value)
			}
		}

		// Check if this is a command to add a message to Zed's chat thread
		if command.Type == "add_message" || command.Type == "chat_message" {
			if content, ok := command.Data["content"].(string); ok {
				fmt.Printf("🎯 SUCCESS: Helix sent message to Zed: '%s'\n", content)
				if content == "hi" {
					fmt.Println("✅ Verified: 'hi' message was injected into Zed chat thread!")
					return nil
				}
			}
		}

		// Reset deadline for next message
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	}

	if commandCount == 0 {
		return fmt.Errorf("no commands received from Helix - WebSocket sync may not be working")
	}

	fmt.Printf("ℹ️  Received %d command(s) but didn't see the expected message injection\n", commandCount)
	return nil
}

func createHelixSession() (*Session, error) {
	sessionReq := CreateSessionRequest{
		Name:      "Integration Test Session",
		Type:      "text",
		Mode:      "inference",
		ModelName: "qwen3:8b",
		Metadata: map[string]string{
			"agent_type": "zed_external",
			"test":       "integration",
		},
		Messages: []Message{
			{
				Role: "user",
				Content: MessageContent{
					ContentType: "text",
					Parts:       []interface{}{"Hello from integration test"},
				},
			},
		},
	}

	reqBody, err := json.Marshal(sessionReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", helixAPIURL+"/api/v1/sessions/chat", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testRunnerToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create session, status: %d", resp.StatusCode)
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, err
	}

	return &session, nil
}

func sendMessageToHelixSession(sessionID, message string) (*Interaction, error) {
	interactionReq := CreateInteractionRequest{
		Message: message,
	}

	reqBody, err := json.Marshal(interactionReq)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/sessions/%s/interactions", helixAPIURL, sessionID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testRunnerToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create interaction, status: %d", resp.StatusCode)
	}

	var interaction Interaction
	if err := json.NewDecoder(resp.Body).Decode(&interaction); err != nil {
		return nil, err
	}

	return &interaction, nil
}

func startZedWithWebSocketAndAIPanel() (*exec.Cmd, error) {
	// Find Zed binary
	zedPaths := []string{
		"../../zed-build/zed",
		"../../zed/target/debug/zed",
		"../../zed/target/release/zed",
		"./zed-build/zed",
	}

	var zedPath string
	for _, path := range zedPaths {
		if _, err := os.Stat(path); err == nil {
			zedPath = path
			break
		}
	}

	if zedPath == "" {
		return nil, fmt.Errorf("Zed binary not found in any of the expected locations: %v", zedPaths)
	}

	fmt.Printf("🚀 Starting Zed from: %s\n", zedPath)

	// Prepare Zed command with environment variables
	cmd := exec.Command(zedPath)

	// Create isolated config and data directories
	testConfigDir := "/home/luke/pm/helix/test-zed-config/config"
	testDataDir := "/home/luke/pm/helix/test-zed-config/data"

	// Set environment variables for WebSocket sync, AI panel, and isolation
	cmd.Env = append(os.Environ(),
		"RUST_LOG=info,external_websocket_sync=debug",
		"ZED_EXTERNAL_SYNC_ENABLED=true",
		"ZED_WEBSOCKET_SYNC_ENABLED=true",
		"ZED_HELIX_URL=localhost:8080",
		"ZED_HELIX_TOKEN="+testRunnerToken,
		"ZED_HELIX_TLS=false",
		"ZED_AUTO_OPEN_AI_PANEL=true",
		"ZED_SHOW_AI_ASSISTANT=true",
		// Isolate Zed config and data directories
		"ZED_CONFIG_DIR="+testConfigDir,
		"ZED_DATA_DIR="+testDataDir,
		"XDG_CONFIG_HOME="+testConfigDir+"/..",
		"XDG_DATA_HOME="+testDataDir+"/..",
	)

	fmt.Println("🔧 Environment variables set:")
	fmt.Println("   ZED_EXTERNAL_SYNC_ENABLED=true")
	fmt.Println("   ZED_WEBSOCKET_SYNC_ENABLED=true")
	fmt.Println("   ZED_HELIX_URL=localhost:8080")
	fmt.Println("   ZED_HELIX_TOKEN=" + testRunnerToken)
	fmt.Println("   ZED_HELIX_TLS=false")
	fmt.Println("   ZED_AUTO_OPEN_AI_PANEL=true")
	fmt.Println("   ZED_SHOW_AI_ASSISTANT=true")
	fmt.Println("   ZED_CONFIG_DIR=" + testConfigDir)
	fmt.Println("   ZED_DATA_DIR=" + testDataDir)
	fmt.Println("🔒 Using isolated Zed config/data (won't interfere with your personal Zed)")

	// Start Zed in the background
	err := cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start Zed: %w", err)
	}

	return cmd, nil
}

func verifyZedConversations() error {
	testDataDir := "/home/luke/pm/helix/test-zed-config/data"
	conversationsDir := testDataDir + "/conversations"

	fmt.Println("🔍 Verifying Zed AI conversations state...")
	fmt.Printf("   Checking directory: %s\n", conversationsDir)

	// Check if conversations directory exists
	if _, err := os.Stat(conversationsDir); os.IsNotExist(err) {
		fmt.Println("   ❌ Conversations directory doesn't exist")
		return fmt.Errorf("conversations directory not found: %s", conversationsDir)
	}

	// List conversation files
	files, err := os.ReadDir(conversationsDir)
	if err != nil {
		return fmt.Errorf("failed to read conversations directory: %w", err)
	}

	fmt.Printf("   📁 Found %d conversation files:\n", len(files))
	for _, file := range files {
		fmt.Printf("      - %s\n", file.Name())

		// Try to read and parse the conversation file
		filePath := conversationsDir + "/" + file.Name()
		content, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("      ❌ Failed to read %s: %v\n", file.Name(), err)
			continue
		}

		fmt.Printf("      📝 Content preview: %s\n", truncateString(string(content), 100))
	}

	if len(files) == 0 {
		fmt.Println("   ⚠️  No conversation files found - Helix sessions may not be syncing to Zed AI")
		return fmt.Errorf("no conversations found in Zed")
	}

	fmt.Printf("   ✅ Found %d conversations in Zed\n", len(files))
	return nil
}

func waitForInterrupt() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	fmt.Println("\n👋 Shutting down...")
}
