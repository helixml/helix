package spectask

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

// ChatMessage represents a streaming chat message chunk
type ChatMessage struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Done    bool   `json:"done"`
}

func newInteractCommand() *cobra.Command {
	var showHistory bool
	var historyCount int
	var sendPrompt string
	var watchMode bool
	var watchInterval int

	cmd := &cobra.Command{
		Use:   "interact <session-id>",
		Short: "Interact with a Helix session - chat, view history, send prompts",
		Long: `Interactive CLI for chatting with a Helix session.

This command allows you to:
  - View session information and status
  - See conversation history
  - Send prompts and receive streaming responses
  - Watch mode for real-time session updates

Examples:
  # View session info and recent history
  helix spectask interact ses_01xxx

  # Show last 10 turns of history
  helix spectask interact ses_01xxx --history --count 10

  # Send a prompt and see streaming response
  helix spectask interact ses_01xxx --send "What files have you modified?"

  # Interactive chat mode (type prompts, see responses)
  helix spectask interact ses_01xxx

  # Watch mode - auto-refresh session status
  helix spectask interact ses_01xxx --watch --interval 5
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			if watchMode {
				return runWatchMode(apiURL, token, sessionID, watchInterval)
			}

			if sendPrompt != "" {
				return sendAndStreamResponse(apiURL, token, sessionID, sendPrompt)
			}

			// Show session info
			session, err := getSessionDetails(apiURL, token, sessionID)
			if err != nil {
				return fmt.Errorf("failed to get session: %w", err)
			}

			printSessionInfo(session)

			if showHistory {
				if err := printSessionHistory(apiURL, token, sessionID, historyCount); err != nil {
					return err
				}
			}

			// If no specific action, run interactive mode
			if !showHistory && sendPrompt == "" {
				return runInteractiveChat(apiURL, token, sessionID, session)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&showHistory, "history", "H", false, "Show conversation history")
	cmd.Flags().IntVarP(&historyCount, "count", "c", 5, "Number of history entries to show")
	cmd.Flags().StringVarP(&sendPrompt, "send", "s", "", "Send a prompt and show streaming response")
	cmd.Flags().BoolVarP(&watchMode, "watch", "w", false, "Watch mode - continuously show session status")
	cmd.Flags().IntVar(&watchInterval, "interval", 3, "Watch interval in seconds")

	return cmd
}

func getSessionDetails(apiURL, token, sessionID string) (*types.Session, error) {
	url := fmt.Sprintf("%s/api/v1/sessions/%s", apiURL, sessionID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var session types.Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, err
	}

	return &session, nil
}

func printSessionInfo(session *types.Session) {
	fmt.Printf("\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("📋 Session: %s\n", session.Name)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("   ID:         %s\n", session.ID)
	fmt.Printf("   Type:       %s\n", session.Type)
	fmt.Printf("   Mode:       %s\n", session.Mode)

	if session.Metadata.ExternalAgentID != "" {
		fmt.Printf("   Agent:      %s\n", session.Metadata.ExternalAgentID)
	}
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
}

func getInteractions(apiURL, token, sessionID string, limit int) ([]*types.Interaction, error) {
	url := fmt.Sprintf("%s/api/v1/sessions/%s/interactions?limit=%d", apiURL, sessionID, limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var interactions []*types.Interaction
	if err := json.NewDecoder(resp.Body).Decode(&interactions); err != nil {
		return nil, err
	}

	return interactions, nil
}

func printSessionHistory(apiURL, token, sessionID string, count int) error {
	interactions, err := getInteractions(apiURL, token, sessionID, count)
	if err != nil {
		return fmt.Errorf("failed to get history: %w", err)
	}

	if len(interactions) == 0 {
		fmt.Printf("📭 No conversation history yet\n\n")
		return nil
	}

	fmt.Printf("📜 Conversation History (last %d turns)\n", len(interactions))
	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n\n")

	for i, entry := range interactions {
		turnNum := len(interactions) - i
		summary := entry.Summary
		if summary == "" {
			summary = truncateString(entry.PromptMessage, 60)
		}

		fmt.Printf("  Turn %d: %s\n", turnNum, summary)

		// Show prompt preview
		promptPreview := truncateString(entry.PromptMessage, 70)
		fmt.Printf("  └─ 🧑 %s\n", promptPreview)

		// Show response preview
		responsePreview := truncateString(entry.ResponseMessage, 70)
		fmt.Printf("     🤖 %s\n\n", responsePreview)
	}

	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")
	fmt.Printf("💡 Use --count N to see more history\n\n")

	return nil
}

func truncateString(s string, maxLen int) string {
	// Replace newlines with spaces for preview
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)

	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func sendAndStreamResponse(apiURL, token, sessionID, prompt string) error {
	fmt.Printf("\n📤 Sending prompt to session %s...\n", sessionID)
	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")
	fmt.Printf("🧑 %s\n", prompt)
	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")
	fmt.Printf("🤖 ")

	// Create chat request
	chatURL := fmt.Sprintf("%s/api/v1/sessions/%s/chat", apiURL, sessionID)

	payload := map[string]interface{}{
		"message": prompt,
		"stream":  true,
	}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", chatURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chat API returned %d: %s", resp.StatusCode, string(body))
	}

	// Stream the response (SSE format)
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading stream: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse SSE data lines
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var msg ChatMessage
			if err := json.Unmarshal([]byte(data), &msg); err != nil {
				// Try to print as plain text
				fmt.Print(data)
				continue
			}

			fmt.Print(msg.Content)
			if msg.Done {
				break
			}
		}
	}

	fmt.Printf("\n───────────────────────────────────────────────────────────────────────────────\n\n")
	return nil
}

func runInteractiveChat(apiURL, token, sessionID string, session *types.Session) error {
	fmt.Printf("🎮 Interactive Chat Mode\n")
	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")
	fmt.Printf("Commands:\n")
	fmt.Printf("  /history    - Show conversation history\n")
	fmt.Printf("  /screenshot - Take a screenshot\n")
	fmt.Printf("  /status     - Show session status\n")
	fmt.Printf("  /stream     - Open video stream (in new terminal)\n")
	fmt.Printf("  /quit       - Exit interactive mode\n")
	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")
	fmt.Printf("Type your message and press Enter to send:\n\n")

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("🧑 > ")

		// Check for Ctrl+C in a non-blocking way
		inputChan := make(chan string, 1)
		errChan := make(chan error, 1)

		go func() {
			input, err := reader.ReadString('\n')
			if err != nil {
				errChan <- err
				return
			}
			inputChan <- input
		}()

		select {
		case <-sigChan:
			fmt.Printf("\n\n👋 Goodbye!\n")
			return nil

		case err := <-errChan:
			if err == io.EOF {
				fmt.Printf("\n\n👋 Goodbye!\n")
				return nil
			}
			return err

		case input := <-inputChan:
			input = strings.TrimSpace(input)
			if input == "" {
				continue
			}

			// Handle commands
			switch strings.ToLower(input) {
			case "/quit", "/exit", "/q":
				fmt.Printf("\n👋 Goodbye!\n")
				return nil

			case "/history", "/h":
				if err := printSessionHistory(apiURL, token, sessionID, 5); err != nil {
					fmt.Printf("❌ %v\n", err)
				}
				continue

			case "/screenshot", "/ss":
				if err := takeAndDisplayScreenshot(apiURL, token, sessionID); err != nil {
					fmt.Printf("❌ %v\n", err)
				}
				continue

			case "/status", "/s":
				session, err := getSessionDetails(apiURL, token, sessionID)
				if err != nil {
					fmt.Printf("❌ %v\n", err)
					continue
				}
				printSessionInfo(session)
				continue

			case "/stream":
				fmt.Printf("📺 To view video stream, run in another terminal:\n")
				fmt.Printf("   helix spectask stream %s\n\n", sessionID)
				continue

			case "/help", "/?":
				fmt.Printf("Commands:\n")
				fmt.Printf("  /history    - Show conversation history\n")
				fmt.Printf("  /screenshot - Take a screenshot\n")
				fmt.Printf("  /status     - Show session status\n")
				fmt.Printf("  /stream     - Open video stream (in new terminal)\n")
				fmt.Printf("  /quit       - Exit interactive mode\n\n")
				continue
			}

			// Regular message - send to chat
			fmt.Printf("\n")
			if err := sendAndStreamResponse(apiURL, token, sessionID, input); err != nil {
				fmt.Printf("❌ Error: %v\n\n", err)
			}
		}
	}
}

func takeAndDisplayScreenshot(apiURL, token, sessionID string) error {
	fmt.Printf("📸 Taking screenshot...\n")

	screenshotURL := fmt.Sprintf("%s/api/v1/external-agents/%s/screenshot", apiURL, sessionID)
	req, err := http.NewRequest("GET", screenshotURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get screenshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("screenshot request failed: %d - %s", resp.StatusCode, string(body))
	}

	// Read and save screenshot
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read screenshot: %w", err)
	}

	filename := fmt.Sprintf("screenshot-%s.jpg", time.Now().Format("20060102-150405"))
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to save screenshot: %w", err)
	}

	fmt.Printf("✅ Screenshot saved: %s (%d bytes)\n\n", filename, len(data))
	return nil
}

func runWatchMode(apiURL, token, sessionID string, interval int) error {
	fmt.Printf("\n🔄 Watch Mode - Session %s (refresh every %ds)\n", sessionID, interval)
	fmt.Printf("Press Ctrl+C to exit\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	// Initial display
	displayWatchStatus(apiURL, token, sessionID)

	for {
		select {
		case <-sigChan:
			fmt.Printf("\n\n👋 Watch mode ended\n")
			return nil

		case <-ticker.C:
			// Clear screen (ANSI escape)
			fmt.Print("\033[H\033[2J")
			fmt.Printf("🔄 Watch Mode - Session %s (refresh every %ds)\n", sessionID, interval)
			fmt.Printf("Press Ctrl+C to exit\n")
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
			displayWatchStatus(apiURL, token, sessionID)
		}
	}
}

func displayWatchStatus(apiURL, token, sessionID string) {
	session, err := getSessionDetails(apiURL, token, sessionID)
	if err != nil {
		fmt.Printf("❌ Error getting session: %v\n", err)
		return
	}

	printSessionInfo(session)

	// Show last interaction
	interactions, err := getInteractions(apiURL, token, sessionID, 1)
	if err == nil && len(interactions) > 0 {
		fmt.Printf("📝 Latest Turn:\n")
		fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")

		entry := interactions[0]
		fmt.Printf("🧑 %s\n\n", truncateString(entry.PromptMessage, 100))
		fmt.Printf("🤖 %s\n", truncateString(entry.ResponseMessage, 200))
		fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")
	}

	fmt.Printf("\n⏰ Last refresh: %s\n", time.Now().Format("15:04:05"))
}

// LiveStream represents a combined live view with video + chat
type LiveStream struct {
	apiURL    string
	token     string
	sessionID string
	wsConn    *websocket.Conn
}

func newLiveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "live <session-id>",
		Short: "Combined live view with video stream stats and session chat",
		Long: `Live view combining video stream monitoring with session interaction.

This creates a split-screen experience showing:
  - Video stream statistics (FPS, bitrate, codec)
  - Recent session activity
  - Ability to send commands to the agent

Example:
  helix spectask live ses_01xxx
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			return runLiveMode(apiURL, token, sessionID)
		},
	}

	return cmd
}

func runLiveMode(apiURL, token, sessionID string) error {
	fmt.Printf("\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("🎮 HELIX LIVE - Session Monitor\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Session: %s\n", sessionID)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Get session info
	session, err := getSessionDetails(apiURL, token, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	printSessionInfo(session)

	// Connect to WebSocket for video stats
	fmt.Printf("📡 Connecting to video stream...\n")

	appID, err := getContainerAppID(apiURL, token, sessionID)
	if err != nil {
		fmt.Printf("⚠️  Video stream not available: %v\n", err)
		fmt.Printf("   (Session may still be starting or doesn't have external agent)\n\n")
		// Fall back to chat-only mode
		return runInteractiveChat(apiURL, token, sessionID, session)
	}

	clientUniqueID := fmt.Sprintf("helix-live-%d", time.Now().UnixNano())
	if err := configurePendingSession(apiURL, token, sessionID, clientUniqueID); err != nil {
		return fmt.Errorf("failed to configure session: %w", err)
	}

	// Build WebSocket URL for direct streaming
	wsURL := strings.Replace(apiURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	streamURL := fmt.Sprintf("%s/api/v1/external-agents/%s/ws/stream", wsURL, url.QueryEscape(sessionID))

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.Dial(streamURL, header)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("⚠️  Video connection failed: %s\n", string(body))
		} else {
			fmt.Printf("⚠️  Video connection failed: %v\n", err)
		}
		return runInteractiveChat(apiURL, token, sessionID, session)
	}
	defer conn.Close()

	// Send init message
	initMessage := map[string]interface{}{
		"type":                    "init",
		"host_id":                 0,
		"app_id":                  appID,
		"session_id":              sessionID,
		"client_unique_id":        clientUniqueID,
		"width":                   1920,
		"height":                  1080,
		"fps":                     60,
		"bitrate":                 10000,
		"packet_size":             1024,
		"play_audio_local":        false,
		"video_supported_formats": 1,
	}
	initJSON, _ := json.Marshal(initMessage)
	if err := conn.WriteMessage(websocket.TextMessage, initJSON); err != nil {
		return fmt.Errorf("failed to send init: %w", err)
	}

	fmt.Printf("✅ Video stream connected\n\n")

	// Start stats collection in background
	videoStats := &liveVideoStats{}
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-done:
				return
			default:
				msgType, data, err := conn.ReadMessage()
				if err != nil {
					return
				}
				if msgType == websocket.BinaryMessage && len(data) > 0 {
					videoStats.update(data)
				}
			}
		}
	}()

	// Display loop with stats
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	fmt.Printf("📊 Live Stats (Ctrl+C to exit, type message + Enter to chat)\n")
	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n\n")

	// Start input reader
	inputChan := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			select {
			case <-done:
				return
			default:
				fmt.Printf("💬 > ")
				input, err := reader.ReadString('\n')
				if err != nil {
					continue
				}
				inputChan <- strings.TrimSpace(input)
			}
		}
	}()

	for {
		select {
		case <-sigChan:
			close(done)
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			fmt.Printf("\n\n👋 Live mode ended\n")
			printFinalStats(videoStats)
			return nil

		case <-ticker.C:
			// Print stats inline
			stats := videoStats.getStats()
			fmt.Printf("\r📺 %dx%d | %.1f fps | %s/s | %d frames     ",
				stats.width, stats.height, stats.fps, formatBits(int64(stats.bitrate)), stats.frames)

		case input := <-inputChan:
			if input == "" {
				continue
			}
			if strings.HasPrefix(input, "/") {
				switch strings.ToLower(input) {
				case "/quit", "/q":
					close(done)
					return nil
				case "/screenshot", "/ss":
					takeAndDisplayScreenshot(apiURL, token, sessionID)
				case "/history", "/h":
					printSessionHistory(apiURL, token, sessionID, 3)
				default:
					fmt.Printf("Unknown command: %s\n", input)
				}
			} else {
				// Send chat message
				fmt.Printf("\n")
				sendAndStreamResponse(apiURL, token, sessionID, input)
			}
		}
	}
}

type liveVideoStats struct {
	frames    int
	bytes     int64
	keyframes int
	width     int
	height    int
	codec     byte
	startTime time.Time
}

func (s *liveVideoStats) update(data []byte) {
	if len(data) == 0 {
		return
	}

	if s.startTime.IsZero() {
		s.startTime = time.Now()
	}

	msgType := data[0]
	switch msgType {
	case WsMsgVideoFrame:
		s.frames++
		s.bytes += int64(len(data))
		if len(data) >= 15 {
			s.codec = data[1]
			if data[2]&0x01 != 0 {
				s.keyframes++
			}
		}
	case WsMsgStreamInit:
		if len(data) >= 6 {
			s.codec = data[1]
			s.width = int(data[2])<<8 | int(data[3])
			s.height = int(data[4])<<8 | int(data[5])
		}
	}
}

type statsSnapshot struct {
	frames  int
	bytes   int64
	fps     float64
	bitrate float64
	width   int
	height  int
}

func (s *liveVideoStats) getStats() statsSnapshot {
	elapsed := time.Since(s.startTime).Seconds()
	if elapsed == 0 {
		elapsed = 1
	}

	return statsSnapshot{
		frames:  s.frames,
		bytes:   s.bytes,
		fps:     float64(s.frames) / elapsed,
		bitrate: float64(s.bytes*8) / elapsed,
		width:   s.width,
		height:  s.height,
	}
}

func printFinalStats(stats *liveVideoStats) {
	s := stats.getStats()
	fmt.Printf("\n📊 Final Statistics:\n")
	fmt.Printf("   Frames: %d (%d keyframes)\n", s.frames, stats.keyframes)
	fmt.Printf("   Data: %s\n", formatBytes(s.bytes))
	fmt.Printf("   Avg FPS: %.1f\n", s.fps)
	fmt.Printf("   Avg Bitrate: %s/s\n", formatBits(int64(s.bitrate)))
}
