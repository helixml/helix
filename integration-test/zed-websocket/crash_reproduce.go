package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	helixWSURL = "ws://localhost:8080/api/v1/external-agents/sync"
)

var testRunnerToken string
var anthropicAPIKey string

func main() {
	fmt.Println("🔥 ZED CRASH REPRODUCTION TEST")
	fmt.Println("===============================")
	fmt.Println("This test creates the exact conditions that cause the agent panel crash:")
	fmt.Println("1. Starts Zed with WebSocket sync enabled")
	fmt.Println("2. Sends continuous WebSocket messages to create active processing")
	fmt.Println("3. You click the agent panel during active processing to trigger crash")
	fmt.Println("")

	// Load runner token from .env file
	if err := loadRunnerToken(); err != nil {
		log.Fatalf("❌ Failed to load runner token: %v", err)
	}
	fmt.Printf("✅ Loaded runner token: %s\n", testRunnerToken)

	// Load Anthropic API key from .env file
	if err := loadAnthropicAPIKey(); err != nil {
		log.Fatalf("❌ Failed to load Anthropic API key: %v", err)
	}
	fmt.Printf("✅ Loaded Anthropic API key: %s\n", anthropicAPIKey[:20]+"...")

	// Start Zed with WebSocket sync
	zedCmd, err := startZedWithCrashMonitoring()
	if err != nil {
		log.Fatalf("❌ Failed to start Zed: %v", err)
	}
	defer func() {
		if zedCmd.Process != nil {
			fmt.Println("🔄 Stopping Zed...")
			zedCmd.Process.Kill()
		}
	}()

	fmt.Println("✅ Zed started with crash monitoring")
	fmt.Println("⏳ Waiting for Zed to initialize...")
	time.Sleep(5 * time.Second)

	// Start continuous WebSocket activity
	go func() {
		if err := maintainWebSocketActivity(); err != nil {
			fmt.Printf("⚠️ WebSocket activity error: %v\n", err)
		}
	}()

	fmt.Println("")
	fmt.Println("🔥 CRASH TEST ACTIVE - READY FOR TESTING")
	fmt.Println("=========================================")
	fmt.Println("✅ Zed is running with WebSocket sync enabled")
	fmt.Println("✅ Continuous WebSocket messages are being sent")
	fmt.Println("✅ Agent panel should be open and visible")
	fmt.Println("✅ Crash monitoring is active")
	fmt.Println("")
	fmt.Println("🎯 TO REPRODUCE CRASH:")
	fmt.Println("   1. Look for the Zed window with AI panel open")
	fmt.Println("   2. Click on the agent panel while messages are being processed")
	fmt.Println("   3. Watch for window disappearing (crash)")
	fmt.Println("   4. Check /tmp/zed-crash-test.log for crash details")
	fmt.Println("")
	fmt.Println("⏰ Test will run for 120 seconds with continuous WebSocket activity...")
	fmt.Println("")

	// Set up signal handling for clean shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Run test for 120 seconds or until interrupted
	timeout := time.After(120 * time.Second)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c:
			fmt.Println("\n🛑 Test interrupted by user")
			return
		case <-timeout:
			fmt.Println("\n⏰ 120 seconds elapsed - test completed")
			return
		case <-ticker.C:
			fmt.Println("🔄 WebSocket activity continuing... Click agent panel to test crash!")
		}
	}
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

func loadAnthropicAPIKey() error {
	// Read .env file from project root
	envFile := "../../.env"
	content, err := os.ReadFile(envFile)
	if err != nil {
		return fmt.Errorf("failed to read .env file: %w", err)
	}

	// Parse the .env file for ANTHROPIC_API_KEY
	lines := string(content)
	for _, line := range strings.Split(lines, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ANTHROPIC_API_KEY=") {
			anthropicAPIKey = strings.TrimPrefix(line, "ANTHROPIC_API_KEY=")
			return nil
		}
	}

	return fmt.Errorf("ANTHROPIC_API_KEY not found in .env file")
}

func startZedWithCrashMonitoring() (*exec.Cmd, error) {
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

	// Start Zed normally (not under GDB) so the window appears
	cmd := exec.Command(zedBinary, "--allow-multiple-instances")
	cmd.Env = append(os.Environ(),
		// Logging
		"ZED_LOG=debug,external_websocket_sync=trace",
		"RUST_LOG=debug,external_websocket_sync=trace",
		// Enable external sync
		"ZED_EXTERNAL_SYNC_ENABLED=true",
		"ZED_WEBSOCKET_SYNC_ENABLED=true",
		// Helix connection details
		"ZED_HELIX_URL=localhost:8080",
		"ZED_HELIX_TOKEN="+testRunnerToken,
		"ZED_HELIX_TLS=false",
		// UI settings
		"ZED_AUTO_OPEN_AI_PANEL=true",
		"ZED_SHOW_AI_ASSISTANT=true",
		// Use isolated config
		"ZED_CONFIG_DIR=/home/luke/pm/helix/test-zed-config/config",
		"ZED_DATA_DIR=/home/luke/pm/helix/test-zed-config/data",
		"ANTHROPIC_API_KEY="+anthropicAPIKey,
	)

	fmt.Printf("🔧 Environment variables set for crash reproduction:\n")
	fmt.Printf("   ZED_EXTERNAL_SYNC_ENABLED=true\n")
	fmt.Printf("   ZED_WEBSOCKET_SYNC_ENABLED=true\n")
	fmt.Printf("   ZED_AUTO_OPEN_AI_PANEL=true\n")
	fmt.Printf("   Running normally (not under GDB) to ensure window appears\n")

	// Capture stdout/stderr for debugging
	logFile, err := os.OpenFile("/tmp/zed-crash-test.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Printf("⚠️ Warning: Could not open log file: %v\n", err)
	} else {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		fmt.Println("📝 Zed logs will be written to /tmp/zed-crash-test.log")
	}

	// Start Zed in background
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start Zed: %w", err)
	}

	return cmd, nil
}

func maintainWebSocketActivity() error {
	fmt.Println("🔌 Starting continuous WebSocket activity to trigger crash conditions...")

	// Connect to WebSocket with agent ID
	agentID := fmt.Sprintf("crash-test-agent-%d", time.Now().Unix())
	wsURL := fmt.Sprintf("%s?agent_id=%s", helixWSURL, agentID)

	dialer := websocket.DefaultDialer
	header := make(map[string][]string)
	header["Authorization"] = []string{"Bearer " + testRunnerToken}

	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}
	defer conn.Close()

	fmt.Printf("✅ Connected to WebSocket as agent: %s\n", agentID)

	// Send continuous messages to maintain active processing
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	messageCount := 0
	for {
		select {
		case <-ticker.C:
			messageCount++

			// Send a context_created message to trigger thread creation
			contextID := fmt.Sprintf("crash-test-context-%d", messageCount)
			message := map[string]interface{}{
				"session_id": contextID,
				"event_type": "context_created",
				"data": map[string]interface{}{
					"context_id": contextID,
					"title":      fmt.Sprintf("Crash Test Context %d", messageCount),
					"timestamp":  time.Now().Unix(),
				},
				"timestamp": time.Now().Format(time.RFC3339),
			}

			if err := conn.WriteJSON(message); err != nil {
				fmt.Printf("⚠️ Failed to send WebSocket message: %v\n", err)
				return err
			}

			fmt.Printf("📤 Sent crash test message #%d - WebSocket processing active\n", messageCount)
			fmt.Println("🔥 CLICK THE AGENT PANEL NOW to trigger crash!")

			// Also send a message_added to simulate active conversation
			messageData := map[string]interface{}{
				"session_id": contextID,
				"event_type": "message_added",
				"data": map[string]interface{}{
					"context_id": contextID,
					"message_id": fmt.Sprintf("msg-%d", messageCount),
					"content":    fmt.Sprintf("This is crash test message %d to maintain active WebSocket processing", messageCount),
					"role":       "user",
					"timestamp":  time.Now().Unix(),
				},
				"timestamp": time.Now().Format(time.RFC3339),
			}

			if err := conn.WriteJSON(messageData); err != nil {
				fmt.Printf("⚠️ Failed to send message data: %v\n", err)
				return err
			}

		default:
			// Keep connection alive
			time.Sleep(100 * time.Millisecond)
		}
	}
}
