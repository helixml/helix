package spectask

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "spectask",
		Short:   "Spec task testing and management",
		Aliases: []string{"st"},
	}

	cmd.AddCommand(newStartCommand())
	cmd.AddCommand(newScreenshotCommand())
	cmd.AddCommand(newListCommand())
	cmd.AddCommand(newResumeCommand())
	cmd.AddCommand(newListAgentsCommand())
	cmd.AddCommand(newStreamCommand())
	cmd.AddCommand(newStopCommand())
	cmd.AddCommand(newInteractCommand())
	cmd.AddCommand(newLiveCommand())
	cmd.AddCommand(newTestCommand())
	cmd.AddCommand(newSendCommand())
	cmd.AddCommand(newMCPCommand())
	cmd.AddCommand(newE2ECommand())
	cmd.AddCommand(newHealthCommand())
	cmd.AddCommand(newKeyboardTestCommand())
	cmd.AddCommand(newScrollTestCommand())

	return cmd
}

func newStartCommand() *cobra.Command {
	var taskName string
	var projectID string
	var agentID string
	var prompt string

	cmd := &cobra.Command{
		Use:   "start [task-id]",
		Short: "Start a spec task planning session (creates sandbox)",
		Long: `Start a spec task planning session which creates a sandbox desktop.

If no task-id is provided, a new spec task will be created.
Use --project to specify which project to create the task in.
Use --agent to specify which Helix agent/app to use (e.g., app_01xxx).

Example workflow:
  1. Fork a sample project:  helix project fork modern-todo-app --name "My Project"
  2. Start a spec task:      helix spectask start --project prj_xxx --agent app_xxx -n "Add dark mode"
  3. Connect via browser:    Visit /wolf-ui and enter the lobby PIN shown
  4. Or use Moonlight:       helix moonlight list-pending && helix moonlight pair <pin>`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiURL := getAPIURL()
			token := getToken()

			var taskID string
			if len(args) > 0 {
				taskID = args[0]
			} else {
				// Create a new spec task
				if projectID == "" {
					return fmt.Errorf("--project is required when creating a new task")
				}
				fmt.Println("Creating new spec task...")
				taskPrompt := prompt
				if taskPrompt == "" {
					taskPrompt = "Testing RevDial connectivity"
				}
				task, err := createSpecTask(apiURL, token, taskName, taskPrompt, projectID, agentID)
				if err != nil {
					return fmt.Errorf("failed to create spec task: %w", err)
				}
				taskID = task.ID
				fmt.Printf("✅ Created spec task: %s (ID: %s)\n", task.Name, task.ID)
				if agentID != "" {
					fmt.Printf("   Agent: %s\n", agentID)
				}
			}

			// Start planning - this triggers async session creation
			fmt.Printf("Starting planning for task %s...\n", taskID)
			task, err := triggerStartPlanning(apiURL, token, taskID)
			if err != nil {
				return fmt.Errorf("failed to start planning: %w", err)
			}
			fmt.Printf("✅ Task status: %s\n", task.Status)

			// Poll for session to be created (sandbox takes ~10-15s to start)
			fmt.Printf("⏳ Waiting for sandbox to start (this takes ~15 seconds)...\n")
			session, err := waitForTaskSession(apiURL, token, taskID, 60*time.Second)
			if err != nil {
				return fmt.Errorf("failed waiting for session: %w", err)
			}

			fmt.Printf("\n✅ Sandbox is running!\n")
			fmt.Printf("   Session ID: %s\n", session.ID)

			// Show connection instructions
			fmt.Printf("\n📺 Connect to Desktop:\n")
			if session.Metadata.WolfLobbyPIN != "" {
				fmt.Printf("   Wolf-UI (browser): Visit %s/wolf-ui\n", apiURL)
				fmt.Printf("   Lobby PIN: %s\n", session.Metadata.WolfLobbyPIN)
			} else {
				fmt.Printf("   Wolf-UI (browser): Visit %s/wolf-ui\n", apiURL)
				fmt.Printf("   (Lobby PIN will appear after sandbox fully initializes)\n")
			}

			fmt.Printf("\n   Native Moonlight client:\n")
			fmt.Printf("   1. First pair your client:  helix moonlight list-pending\n")
			fmt.Printf("   2. Enter PIN from client:   helix moonlight pair <pin>\n")
			fmt.Printf("   3. Then connect to the Wolf server\n")

			fmt.Printf("\n📷 Test screenshot:\n")
			fmt.Printf("   helix spectask screenshot %s\n", session.ID)

			return nil
		},
	}

	cmd.Flags().StringVarP(&taskName, "name", "n", "CLI Test Task", "Task name")
	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Project ID (required when creating new task)")
	cmd.Flags().StringVarP(&agentID, "agent", "a", "", "Agent/App ID to use (e.g., app_01xxx)")
	cmd.Flags().StringVar(&prompt, "prompt", "", "Task prompt/description")

	return cmd
}

func newScreenshotCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "screenshot <session-id>",
		Short: "Test screenshot endpoint (tests RevDial connectivity)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			fmt.Printf("Testing screenshot for session %s...\n", sessionID)

			screenshotURL := fmt.Sprintf("%s/api/v1/external-agents/%s/screenshot", apiURL, sessionID)
			req, err := http.NewRequest("GET", screenshotURL, nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
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

			// Read screenshot data
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read screenshot: %w", err)
			}

			// Save to file
			filename := fmt.Sprintf("screenshot-%s.jpg", time.Now().Format("20060102-150405"))
			if err := os.WriteFile(filename, data, 0644); err != nil {
				return fmt.Errorf("failed to save screenshot: %w", err)
			}

			fmt.Printf("✅ Screenshot saved: %s (%d bytes)\n", filename, len(data))
			return nil
		},
	}
}

type SessionsResponse struct {
	Sessions []Session `json:"sessions"`
}

func newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List active spec task sessions with external agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			apiURL := getAPIURL()
			token := getToken()

			req, err := http.NewRequest("GET", apiURL+"/api/v1/sessions", nil)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			var response SessionsResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				return fmt.Errorf("failed to parse sessions: %w", err)
			}

			fmt.Println("Active Sessions with External Agents:")
			fmt.Println()
			count := 0
			for _, s := range response.Sessions {
				// Only show sessions with Wolf lobby (external agent sessions)
				if s.Metadata.WolfLobbyID != "" {
					count++
					fmt.Printf("Session: %s\n", s.ID)
					fmt.Printf("  Type: %s\n", s.Type)
					if s.Metadata.ContainerName != "" {
						fmt.Printf("  Container: %s\n", s.Metadata.ContainerName)
					}
					if s.Metadata.WolfLobbyPIN != "" {
						fmt.Printf("  Lobby PIN: %s (for Wolf-UI browser access)\n", s.Metadata.WolfLobbyPIN)
					}
					fmt.Printf("  Screenshot: helix spectask screenshot %s\n", s.ID)
					fmt.Println()
				}
			}

			if count == 0 {
				fmt.Println("No sessions with active external agents found.")
				fmt.Println("\nTo start a session:")
				fmt.Println("  1. Fork a sample project:  helix project fork modern-todo-app --name \"My Project\"")
				fmt.Println("  2. Create a spec task:     helix spectask start <project-id> -n \"Task Name\"")
			} else {
				fmt.Printf("Found %d session(s) with external agents.\n", count)
			}

			return nil
		},
	}
}

func newStopCommand() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "stop <session-id>",
		Short: "Stop a running sandbox session",
		Long: `Stops a running sandbox session and its container.

Use --all to stop all sessions with external agents.

Examples:
  helix spectask stop ses_01xxx      # Stop specific session
  helix spectask stop --all          # Stop all sessions
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiURL := getAPIURL()
			token := getToken()

			if all {
				return stopAllSessions(apiURL, token)
			}

			if len(args) == 0 {
				return fmt.Errorf("session ID required (or use --all)")
			}

			sessionID := args[0]
			return stopSession(apiURL, token, sessionID)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Stop all sessions with external agents")

	return cmd
}

func stopSession(apiURL, token, sessionID string) error {
	fmt.Printf("Stopping session %s...\n", sessionID)

	url := fmt.Sprintf("%s/api/v1/external-agents/%s", apiURL, sessionID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to stop session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stop failed: %d - %s", resp.StatusCode, string(body))
	}

	fmt.Printf("Session %s stopped.\n", sessionID)
	return nil
}

func stopAllSessions(apiURL, token string) error {
	// Get all sessions
	req, err := http.NewRequest("GET", apiURL+"/api/v1/sessions", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var response SessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to parse sessions: %w", err)
	}

	stopped := 0
	for _, s := range response.Sessions {
		if s.Metadata.WolfLobbyID != "" {
			if err := stopSession(apiURL, token, s.ID); err != nil {
				fmt.Printf("Failed to stop %s: %v\n", s.ID, err)
			} else {
				stopped++
			}
		}
	}

	fmt.Printf("\nStopped %d session(s).\n", stopped)
	return nil
}

// Helper functions

func getAPIURL() string {
	url := os.Getenv("HELIX_URL")
	if url == "" {
		url = "http://localhost:8080"
	}
	return url
}

func getToken() string {
	token := os.Getenv("HELIX_API_KEY")
	if token == "" {
		token = "oh-hallo-insecure-token" // Dev default
	}
	return token
}

type SpecTask struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type Session struct {
	ID         string          `json:"session_id"`
	Mode       string          `json:"mode"`
	Type       string          `json:"type"`
	ModelName  string          `json:"model_name"`
	Metadata   SessionMetadata `json:"metadata"`
}

type SessionMetadata struct {
	ContainerName   string `json:"container_name"`
	WolfLobbyID     string `json:"wolf_lobby_id"`
	WolfLobbyPIN    string `json:"wolf_lobby_pin"`
	ExternalAgentID string `json:"external_agent_id"`
	SpecTaskID      string `json:"spec_task_id"`
}

func createSpecTask(apiURL, token, name, prompt, projectID, agentID string) (*SpecTask, error) {
	payload := map[string]string{
		"name":   name,
		"prompt": prompt, // API expects "prompt" not "description"
	}
	if projectID != "" {
		payload["project_id"] = projectID
	}
	if agentID != "" {
		payload["app_id"] = agentID // API expects "app_id" not "helix_app_id"
	}
	jsonData, _ := json.Marshal(payload)

	// Use /from-prompt endpoint which handles spec task creation from prompts
	req, err := http.NewRequest("POST", apiURL+"/api/v1/spec-tasks/from-prompt", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var task SpecTask
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return nil, err
	}

	return &task, nil
}

// triggerStartPlanning starts planning for a task (returns task, not session)
func triggerStartPlanning(apiURL, token, taskID string) (*SpecTask, error) {
	url := fmt.Sprintf("%s/api/v1/spec-tasks/%s/start-planning", apiURL, taskID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var task SpecTask
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return nil, err
	}

	return &task, nil
}

// waitForTaskSession polls for a session with wolf_lobby_id to be created for the task
func waitForTaskSession(apiURL, token, taskID string, timeout time.Duration) (*Session, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		// Get all sessions and find one for this task
		req, err := http.NewRequest("GET", apiURL+"/api/v1/sessions", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		var response SessionsResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			resp.Body.Close()
			time.Sleep(pollInterval)
			continue
		}
		resp.Body.Close()

		// Find session for this task with wolf_lobby_id (means sandbox is running)
		for _, s := range response.Sessions {
			if s.Metadata.SpecTaskID == taskID && s.Metadata.WolfLobbyID != "" {
				return &s, nil
			}
		}

		time.Sleep(pollInterval)
	}

	return nil, fmt.Errorf("timeout waiting for sandbox to start (task: %s)", taskID)
}

func newResumeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <session-id>",
		Short: "Resume an existing session (tests session restore functionality)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			fmt.Printf("Resuming session %s...\n", sessionID)

			url := fmt.Sprintf("%s/api/v1/sessions/%s/resume", apiURL, sessionID)
			req, err := http.NewRequest("POST", url, nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)

			client := &http.Client{Timeout: 60 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("failed to resume session: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("resume failed: %d - %s", resp.StatusCode, string(body))
			}

			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			fmt.Printf("✅ Session resumed successfully!\n")
			if lobbyPIN, ok := result["wolf_lobby_pin"].(string); ok && lobbyPIN != "" {
				fmt.Printf("   Lobby PIN: %s\n", lobbyPIN)
				fmt.Printf("   Connect via Wolf-UI: %s/wolf-ui\n", apiURL)
			}
			if lobbyID, ok := result["wolf_lobby_id"].(string); ok {
				fmt.Printf("   Lobby ID: %s\n", lobbyID)
			}

			return nil
		},
	}
}

type App struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Config AppConfig `json:"config"`
}

type AppConfig struct {
	Helix HelixConfig `json:"helix"`
}

type HelixConfig struct {
	Assistants []Assistant `json:"assistants"`
}

type Assistant struct {
	Name             string `json:"name"`
	AgentType        string `json:"agent_type"`
	CodeAgentRuntime string `json:"code_agent_runtime"`
	Model            string `json:"model"`
}

type AppsResponse struct {
	Apps []App `json:"apps"`
}

func newListAgentsCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "list-agents",
		Short:   "List available Helix agents/apps",
		Aliases: []string{"agents"},
		RunE: func(cmd *cobra.Command, args []string) error {
			apiURL := getAPIURL()
			token := getToken()

			req, err := http.NewRequest("GET", apiURL+"/api/v1/apps", nil)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			var apps []App
			if err := json.NewDecoder(resp.Body).Decode(&apps); err != nil {
				return fmt.Errorf("failed to parse apps: %w", err)
			}

			fmt.Println("Available Agents (Apps with zed_external assistants):")
			fmt.Println()

			count := 0
			for _, app := range apps {
				// Find zed_external assistants
				for _, assistant := range app.Config.Helix.Assistants {
					if assistant.AgentType == "zed_external" {
						count++
						fmt.Printf("App: %s\n", app.Name)
						fmt.Printf("  ID: %s\n", app.ID)
						fmt.Printf("  Assistant: %s\n", assistant.Name)
						if assistant.CodeAgentRuntime != "" {
							fmt.Printf("  Runtime: %s\n", assistant.CodeAgentRuntime)
						}
						if assistant.Model != "" {
							fmt.Printf("  Model: %s\n", assistant.Model)
						}
						fmt.Printf("  Usage: helix spectask start --project <prj_id> --agent %s -n \"Task name\"\n", app.ID)
						fmt.Println()
						break
					}
				}
			}

			if count == 0 {
				fmt.Println("No agents with zed_external assistants found.")
				fmt.Println("Create an agent with a zed_external assistant in the Helix UI first.")
			} else {
				fmt.Printf("Found %d agent(s) with external assistant support.\n", count)
			}

			return nil
		},
	}
}


func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatBits(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d bps", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cbps", float64(b)/float64(div), "KMG"[exp])
}

// WebSocket message types for custom video protocol (matching websocket-stream.ts)
const (
	WsMsgVideoFrame    = 0x01
	WsMsgAudioFrame    = 0x02
	WsMsgVideoBatch    = 0x03
	WsMsgKeyboardInput = 0x10
	WsMsgMouseClick    = 0x11
	WsMsgMouseAbsolute = 0x12
	WsMsgMouseRelative = 0x13
	WsMsgControlMsg    = 0x20
	WsMsgStreamInit    = 0x30
	WsMsgPing          = 0x40
	WsMsgPong          = 0x41
)

// Video codec constants
const (
	VideoCodecH264        = 0x01
	VideoCodecH264High444 = 0x02
	VideoCodecH265        = 0x10
	VideoCodecH265Main10  = 0x11
	VideoCodecAV1Main8    = 0x20
	VideoCodecAV1Main10   = 0x21
)

func codecName(codec byte) string {
	switch codec {
	case VideoCodecH264:
		return "H.264"
	case VideoCodecH264High444:
		return "H.264 High 4:4:4"
	case VideoCodecH265:
		return "HEVC"
	case VideoCodecH265Main10:
		return "HEVC Main10"
	case VideoCodecAV1Main8:
		return "AV1"
	case VideoCodecAV1Main10:
		return "AV1 10bit"
	default:
		return fmt.Sprintf("Unknown(0x%02x)", codec)
	}
}

func newStreamCommand() *cobra.Command {
	var duration int
	var outputFile string
	var verbose bool
	var width, height, fps, bitrate int

	cmd := &cobra.Command{
		Use:   "stream <session-id>",
		Short: "Connect to video stream and display real-time statistics",
		Long: `Connects to the WebSocket video streaming protocol and displays real-time statistics.

This command uses the WebSocket-only protocol (not WebRTC) which streams raw H.264/HEVC/AV1
video frames directly over WebSocket. This is ideal for testing and debugging the video pipeline.

Statistics displayed:
  - Frame rate (FPS)
  - Bitrate (Mbps)
  - Frame sizes and types (keyframe vs delta)
  - Resolution and codec
  - Keyframe count

Examples:
  helix spectask stream ses_01xxx                           # Run until Ctrl+C (1080p default)
  helix spectask stream ses_01xxx --width 3840 --height 2160  # Stream at 4K resolution
  helix spectask stream ses_01xxx --duration 30             # Run for 30 seconds
  helix spectask stream ses_01xxx --output video.h264       # Save raw video to file
  helix spectask stream ses_01xxx -v                        # Verbose mode (show each frame)
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			fmt.Printf("📊 Video Stream for session %s\n", sessionID)
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

			// Build WebSocket URL - direct mode (bypass Wolf/Moonlight)
			wsURL := strings.Replace(apiURL, "http://", "ws://", 1)
			wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
			streamURL := fmt.Sprintf("%s/api/v1/external-agents/%s/ws/stream", wsURL, url.QueryEscape(sessionID))

			fmt.Printf("WebSocket URL: %s\n", streamURL)
			if duration > 0 {
				fmt.Printf("Duration: %d seconds\n", duration)
			}
			if outputFile != "" {
				fmt.Printf("Output file: %s\n", outputFile)
			}
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

			// Step 4: Connect to WebSocket
			header := http.Header{}
			header.Set("Authorization", "Bearer "+token)

			dialer := websocket.Dialer{
				HandshakeTimeout: 10 * time.Second,
			}

			startTime := time.Now()
			conn, resp, err := dialer.Dial(streamURL, header)
			if err != nil {
				if resp != nil {
					body, _ := io.ReadAll(resp.Body)
					return fmt.Errorf("WebSocket connection failed: %w - %s", err, string(body))
				}
				return fmt.Errorf("WebSocket connection failed: %w", err)
			}
			defer conn.Close()

			connectTime := time.Since(startTime)
			fmt.Printf("✅ Connected in %v\n\n", connectTime.Round(time.Millisecond))

			// Send init message (direct WebSocket protocol)
			initMessage := map[string]interface{}{
				"type":                    "init",
				"session_id":              sessionID,
				"width":                   width,
				"height":                  height,
				"fps":                     fps,
				"bitrate":                 bitrate,
				"packet_size":             1024,
				"play_audio_local":        false,
				"video_supported_formats": 1, // H264 = 0x01
			}
			initJSON, _ := json.Marshal(initMessage)
			fmt.Printf("📤 Sending init message (%dx%d@%dfps, %dkbps)...\n",
				width, height, fps, bitrate)
			if err := conn.WriteMessage(websocket.TextMessage, initJSON); err != nil {
				return fmt.Errorf("failed to send init: %w", err)
			}
			fmt.Printf("✅ Init sent, waiting for video frames...\n\n")

			// Optional output file
			var outFile *os.File
			if outputFile != "" {
				f, err := os.Create(outputFile)
				if err != nil {
					return fmt.Errorf("failed to create output file: %w", err)
				}
				defer f.Close()
				outFile = f
				fmt.Printf("📁 Writing video data to %s\n\n", outputFile)
			}

			// Statistics tracking
			stats := &videoStreamStats{
				startTime: time.Now(),
			}

			// Handle graceful shutdown
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

			done := make(chan struct{})
			var lastError error

			// Message reading goroutine
			go func() {
				defer close(done)
				for {
					msgType, data, err := conn.ReadMessage()
					if err != nil {
						if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
							lastError = err
						}
						return
					}

					stats.mu.Lock()
					stats.totalMessages++
					stats.totalBytes += int64(len(data))

					if msgType == websocket.BinaryMessage && len(data) > 0 {
						wsMsgType := data[0]
						switch wsMsgType {
						case WsMsgVideoFrame:
							stats.videoFrames++
							stats.videoBytes += int64(len(data))
							if len(data) >= 15 {
								codec := data[1]
								flags := data[2]
								isKeyframe := (flags & 0x01) != 0
								if stats.codec == 0 {
									stats.codec = codec
								}
								if isKeyframe {
									stats.keyframes++
								}
								frameSize := len(data) - 15
								if frameSize < stats.minFrameSize || stats.minFrameSize == 0 {
									stats.minFrameSize = frameSize
								}
								if frameSize > stats.maxFrameSize {
									stats.maxFrameSize = frameSize
								}
								if verbose {
									frameType := "delta"
									if isKeyframe {
										frameType = "KEY"
									}
									fmt.Printf("[%s] Video: %s %d bytes (%s)\n",
										time.Now().Format("15:04:05.000"),
										codecName(codec), frameSize, frameType)
								}
								// Write raw frame data to file if specified
								if outFile != nil {
									outFile.Write(data[15:])
								}
							}
						case WsMsgAudioFrame:
							stats.audioFrames++
							stats.audioBytes += int64(len(data))
						case WsMsgVideoBatch:
							if len(data) >= 3 {
								frameCount := int(data[1])<<8 | int(data[2])
								stats.videoFrames += frameCount
								stats.batchCount++
								if verbose {
									fmt.Printf("[%s] Batch: %d frames\n",
										time.Now().Format("15:04:05.000"), frameCount)
								}
							}
						case WsMsgStreamInit:
							if len(data) >= 13 {
								codec := data[1]
								width := int(data[2])<<8 | int(data[3])
								height := int(data[4])<<8 | int(data[5])
								fps := int(data[6])
								stats.codec = codec
								stats.width = width
								stats.height = height
								stats.fps = fps
								fmt.Printf("📺 StreamInit: %dx%d@%dfps (%s)\n",
									width, height, fps, codecName(codec))
							}
						case WsMsgPong:
							// RTT measurement would go here
						}
					} else if msgType == websocket.TextMessage {
						// JSON control message
						if verbose {
							fmt.Printf("[%s] Control: %s\n",
								time.Now().Format("15:04:05.000"),
								string(data[:min(100, len(data))]))
						}
					}
					stats.mu.Unlock()
				}
			}()

			// Statistics printer goroutine
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			var timeoutChan <-chan time.Time
			if duration > 0 {
				timeoutChan = time.After(time.Duration(duration) * time.Second)
			}

			printStats := func(final bool) {
				stats.mu.Lock()
				defer stats.mu.Unlock()

				elapsed := time.Since(stats.startTime)
				header := "📈 Live Statistics"
				if final {
					header = "📊 Final Statistics"
				}

				fmt.Printf("\n%s (elapsed: %v)\n", header, elapsed.Round(time.Second))
				fmt.Printf("───────────────────────────────────────────────\n")

				if stats.width > 0 {
					fmt.Printf("Resolution:         %dx%d\n", stats.width, stats.height)
				}
				if stats.codec > 0 {
					fmt.Printf("Codec:              %s\n", codecName(stats.codec))
				}

				fmt.Printf("Video frames:       %d", stats.videoFrames)
				if stats.keyframes > 0 {
					fmt.Printf(" (%d keyframes)\n", stats.keyframes)
				} else {
					fmt.Println()
				}
				fmt.Printf("Audio frames:       %d\n", stats.audioFrames)
				fmt.Printf("Total data:         %s\n", formatBytes(stats.totalBytes))

				if elapsed.Seconds() > 0 {
					videoFps := float64(stats.videoFrames) / elapsed.Seconds()
					videoBitrate := float64(stats.videoBytes*8) / elapsed.Seconds()
					fmt.Printf("Frame rate:         %.2f fps\n", videoFps)
					fmt.Printf("Video bitrate:      %s/s\n", formatBits(int64(videoBitrate)))
				}

				if stats.videoFrames > 0 {
					avgSize := stats.videoBytes / int64(stats.videoFrames)
					fmt.Printf("Avg frame size:     %s\n", formatBytes(avgSize))
					fmt.Printf("Frame size range:   %s - %s\n",
						formatBytes(int64(stats.minFrameSize)),
						formatBytes(int64(stats.maxFrameSize)))
				}

				if stats.batchCount > 0 {
					fmt.Printf("Batch messages:     %d\n", stats.batchCount)
				}

				fmt.Printf("───────────────────────────────────────────────\n")
			}

			for {
				select {
				case <-sigChan:
					fmt.Println("\n🛑 Interrupted...")
					conn.WriteMessage(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					printStats(true)
					return nil

				case <-done:
					if lastError != nil {
						fmt.Printf("\n❌ Connection error: %v\n", lastError)
					} else {
						fmt.Println("\n📭 Connection closed by server")
					}
					printStats(true)
					return lastError

				case <-timeoutChan:
					fmt.Printf("\n⏰ Duration reached (%ds)\n", duration)
					conn.WriteMessage(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					printStats(true)
					return nil

				case <-ticker.C:
					if !verbose {
						printStats(false)
					}
				}
			}
		},
	}

	cmd.Flags().IntVarP(&duration, "duration", "d", 0, "Run for specified seconds (0 = until interrupted)")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Write raw video frames to file")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Print each frame received")
	cmd.Flags().IntVar(&width, "width", 1920, "Video stream width in pixels")
	cmd.Flags().IntVar(&height, "height", 1080, "Video stream height in pixels")
	cmd.Flags().IntVar(&fps, "fps", 60, "Video stream frames per second")
	cmd.Flags().IntVar(&bitrate, "bitrate", 10000, "Video stream bitrate in kbps")

	return cmd
}

// videoStreamStats tracks statistics for the stream command
type videoStreamStats struct {
	mu           sync.Mutex
	startTime    time.Time
	totalMessages int
	totalBytes   int64
	videoFrames  int
	videoBytes   int64
	audioFrames  int
	audioBytes   int64
	keyframes    int
	batchCount   int
	minFrameSize int
	maxFrameSize int
	codec        byte
	width        int
	height       int
	fps          int
}

// runInteractiveStream runs a combined interactive session with VLC server, keyboard, and mouse support
func runInteractiveStream(apiURL, token, sessionID string, wsConn *websocket.Conn, vlcAddr string, keyboardEnabled bool, timeout int) error {
	fmt.Printf("\n🎮 Interactive stream mode\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Start VLC server if requested
	var vlcClients sync.Map

	if vlcAddr != "" {
		fmt.Printf("📡 VLC server: http://localhost%s/stream\n", vlcAddr)

		// Start VLC HTTP server
		mux := http.NewServeMux()

		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "Helix Video Stream Server\n\nEndpoints:\n  /stream - Video stream (connect VLC here)\n  /stats - Connection stats\n")
		})

		mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
			clientID := fmt.Sprintf("%s-%d", r.RemoteAddr, time.Now().UnixNano())
			clientChan := make(chan []byte, 50)
			vlcClients.Store(clientID, clientChan)
			defer vlcClients.Delete(clientID)

			fmt.Printf("📺 VLC client connected: %s\n", r.RemoteAddr)

			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("Transfer-Encoding", "chunked")

			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "Streaming not supported", http.StatusInternalServerError)
				return
			}

			for data := range clientChan {
				_, err := w.Write(data)
				if err != nil {
					fmt.Printf("📴 VLC client disconnected: %s\n", r.RemoteAddr)
					return
				}
				flusher.Flush()
			}
		})

		mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
			count := 0
			vlcClients.Range(func(_, _ interface{}) bool {
				count++
				return true
			})
			fmt.Fprintf(w, "Connected VLC clients: %d\n", count)
		})

		server := &http.Server{Addr: vlcAddr, Handler: mux}
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				fmt.Printf("VLC server error: %v\n", err)
			}
		}()
	}

	if keyboardEnabled {
		fmt.Printf("⌨️  Keyboard: Type to send keystrokes\n")
	}
	fmt.Printf("Press Ctrl+C to exit.\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Handle Ctrl+C gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan struct{})

	// Read WebSocket messages and broadcast to VLC clients
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				msgType, data, err := wsConn.ReadMessage()
				if err != nil {
					return
				}
				// Broadcast binary messages to VLC clients
				if msgType == websocket.BinaryMessage {
					vlcClients.Range(func(key, value interface{}) bool {
						ch := value.(chan []byte)
						select {
						case ch <- data:
						default:
							// Client too slow, drop frame
						}
						return true
					})
				}
			}
		}
	}()

	// Keep WebSocket alive with pings
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := wsConn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Keyboard input handling
	var inputChan chan byte
	if keyboardEnabled {
		inputChan = make(chan byte, 100)
		go func() {
			defer close(inputChan)
			buf := make([]byte, 1)
			for {
				select {
				case <-done:
					return
				default:
					n, err := os.Stdin.Read(buf)
					if err != nil || n == 0 {
						continue
					}
					inputChan <- buf[0]
				}
			}
		}()
	}

	// Timeout handling
	var timeoutChan <-chan time.Time
	if timeout > 0 {
		timeoutChan = time.After(time.Duration(timeout) * time.Second)
	}

	for {
		select {
		case <-sigChan:
			close(done)
			// Close all VLC client channels
			vlcClients.Range(func(key, value interface{}) bool {
				close(value.(chan []byte))
				return true
			})
			wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			fmt.Println("\n🛑 Interactive stream ended.")
			return nil

		case <-timeoutChan:
			close(done)
			vlcClients.Range(func(key, value interface{}) bool {
				close(value.(chan []byte))
				return true
			})
			wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			fmt.Printf("\n⏰ Timeout reached (%ds)\n", timeout)
			return nil

		case b, ok := <-inputChan:
			if !ok || !keyboardEnabled {
				continue
			}

			// Convert byte to keycode and send
			keycode := byteToKeycode(b)
			if keycode > 0 {
				// Send key press and release
				if err := sendKeyEvent(apiURL, token, sessionID, keycode, true); err != nil {
					fmt.Printf("\nKey error: %v\n", err)
					continue
				}
				if err := sendKeyEvent(apiURL, token, sessionID, keycode, false); err != nil {
					// Ignore release errors
				}
				// Echo the character locally for feedback
				if b >= 32 && b < 127 {
					fmt.Printf("%c", b)
				} else if b == 13 || b == 10 {
					fmt.Println()
				}
			}
		}
	}
}


// sendTextToSession sends a string as keyboard input
func sendTextToSession(apiURL, token, sessionID, text string) error {
	fmt.Printf("Sending text to session %s: %q\n", sessionID, text)

	for _, c := range text {
		keycode := charToKeycode(byte(c))
		if keycode > 0 {
			// Send key press
			if err := sendKeyEvent(apiURL, token, sessionID, keycode, true); err != nil {
				return fmt.Errorf("failed to send key press: %w", err)
			}
			// Send key release
			if err := sendKeyEvent(apiURL, token, sessionID, keycode, false); err != nil {
				return fmt.Errorf("failed to send key release: %w", err)
			}
			// Small delay between keys
			time.Sleep(10 * time.Millisecond)
		}
	}

	fmt.Println("Text sent successfully!")
	return nil
}

// sendKeyEvent sends a single key event to the sandbox
func sendKeyEvent(apiURL, token, sessionID string, keycode uint32, pressed bool) error {
	inputURL := fmt.Sprintf("%s/api/v1/external-agents/%s/input", apiURL, sessionID)

	event := map[string]interface{}{
		"type":    "key",
		"keycode": keycode,
		"state":   pressed,
	}

	jsonData, err := json.Marshal(event)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", inputURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("input API returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// byteToKeycode converts a terminal byte to Linux keycode
func byteToKeycode(b byte) uint32 {
	return charToKeycode(b)
}

// charToKeycode maps ASCII characters to Linux input keycodes
// Based on linux/input-event-codes.h
func charToKeycode(c byte) uint32 {
	// Linux keycodes for common keys
	keycodes := map[byte]uint32{
		'1': 2, '2': 3, '3': 4, '4': 5, '5': 6, '6': 7, '7': 8, '8': 9, '9': 10, '0': 11,
		'-': 12, '=': 13,
		'q': 16, 'w': 17, 'e': 18, 'r': 19, 't': 20, 'y': 21, 'u': 22, 'i': 23, 'o': 24, 'p': 25,
		'[': 26, ']': 27,
		'a': 30, 's': 31, 'd': 32, 'f': 33, 'g': 34, 'h': 35, 'j': 36, 'k': 37, 'l': 38,
		';': 39, '\'': 40, '`': 41, '\\': 43,
		'z': 44, 'x': 45, 'c': 46, 'v': 47, 'b': 48, 'n': 49, 'm': 50,
		',': 51, '.': 52, '/': 53,
		' ': 57, // Space

		// Upper case uses same keycodes (shift would need separate handling)
		'Q': 16, 'W': 17, 'E': 18, 'R': 19, 'T': 20, 'Y': 21, 'U': 22, 'I': 23, 'O': 24, 'P': 25,
		'A': 30, 'S': 31, 'D': 32, 'F': 33, 'G': 34, 'H': 35, 'J': 36, 'K': 37, 'L': 38,
		'Z': 44, 'X': 45, 'C': 46, 'V': 47, 'B': 48, 'N': 49, 'M': 50,

		// Control characters
		13: 28, // Enter
		10: 28, // Newline (also Enter)
		9:  15, // Tab
		8:  14, // Backspace
		27: 1,  // Escape

		// Shifted symbols (without shift for now - basic implementation)
		'!': 2, '@': 3, '#': 4, '$': 5, '%': 6, '^': 7, '&': 8, '*': 9, '(': 10, ')': 11,
		'_': 12, '+': 13, '{': 26, '}': 27, '|': 43, ':': 39, '"': 40, '~': 41,
		'<': 51, '>': 52, '?': 53,
	}

	if kc, ok := keycodes[c]; ok {
		return kc
	}
	return 0
}

// getWolfAppID fetches the Wolf placeholder app ID for a session
// This is required for the AuthenticateAndInit message
func getWolfAppID(apiURL, token, sessionID string) (int, error) {
	// Use the wolf/ui-app-id endpoint which returns the placeholder app ID
	url := fmt.Sprintf("%s/api/v1/wolf/ui-app-id?session_id=%s", apiURL, sessionID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		PlaceholderAppID string `json:"placeholder_app_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	// Parse the app ID as integer
	var appID int
	if _, err := fmt.Sscanf(result.PlaceholderAppID, "%d", &appID); err != nil {
		return 0, fmt.Errorf("invalid app_id '%s': %w", result.PlaceholderAppID, err)
	}

	return appID, nil
}

// configurePendingSession pre-configures Wolf with our client_unique_id
// This allows Wolf to immediately attach us to the lobby when we connect
func configurePendingSession(apiURL, token, sessionID, clientUniqueID string) error {
	url := fmt.Sprintf("%s/api/v1/external-agents/%s/configure-pending-session", apiURL, sessionID)

	payload := map[string]string{
		"client_unique_id": clientUniqueID,
	}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// newKeyboardTestCommand creates a command to test keyboard input via WebSocket
func newKeyboardTestCommand() *cobra.Command {
	var keyCode int
	var count int
	var delay int

	cmd := &cobra.Command{
		Use:   "keyboard-test <session-id>",
		Short: "Test keyboard input over WebSocket stream",
		Long: `Connects to WebSocket stream and sends keyboard events to debug input path.

This command tests the WebSocket keyboard input path:
  Browser -> Helix API (moonlight_proxy.go) -> moonlight-web (stream.rs) -> streamer (main.rs) -> Wolf

Uses Windows Virtual Key (VK) codes, same as Moonlight protocol.

Examples:
  # Send 'a' key (VK code 0x41 = 65)
  helix spectask keyboard-test ses_01xxx

  # Send specific key (e.g., Enter = 0x0D = 13)
  helix spectask keyboard-test ses_01xxx --key 13

  # Send multiple keystrokes
  helix spectask keyboard-test ses_01xxx --count 5 --delay 200
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			return runKeyboardTest(apiURL, token, sessionID, keyCode, count, delay)
		},
	}

	cmd.Flags().IntVar(&keyCode, "key", 0x41, "Virtual Key code (default 0x41 = 'A')")
	cmd.Flags().IntVarP(&count, "count", "n", 1, "Number of keystrokes to send")
	cmd.Flags().IntVarP(&delay, "delay", "d", 100, "Delay between keystrokes in ms")

	return cmd
}

// runKeyboardTest connects to WebSocket and sends keyboard events
func runKeyboardTest(apiURL, token, sessionID string, keyCode, count, delayMs int) error {
	fmt.Printf("⌨️  Keyboard Test - Session %s\n", sessionID)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Key code: 0x%02X (%d), Count: %d, Delay: %dms\n\n", keyCode, keyCode, count, delayMs)

	// Get Wolf app ID
	appID, err := getWolfAppID(apiURL, token, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get Wolf app ID: %w", err)
	}
	fmt.Printf("✅ Got Wolf app ID: %d\n", appID)

	// Configure pending session
	clientUniqueID := fmt.Sprintf("helix-kbd-test-%d", time.Now().UnixNano())
	if err := configurePendingSession(apiURL, token, sessionID, clientUniqueID); err != nil {
		return fmt.Errorf("failed to configure session: %w", err)
	}
	fmt.Printf("✅ Configured pending session: %s\n", clientUniqueID)

	// Build WebSocket URL
	wsURL := strings.Replace(apiURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	streamURL := fmt.Sprintf("%s/moonlight/api/ws/stream?session_id=%s", wsURL, url.QueryEscape(sessionID))

	fmt.Printf("📡 Connecting to: %s\n", streamURL)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.Dial(streamURL, header)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("WebSocket dial failed: %s", string(body))
		}
		return fmt.Errorf("WebSocket dial failed: %w", err)
	}
	defer conn.Close()

	fmt.Printf("✅ WebSocket connected\n")

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
	fmt.Printf("✅ Sent init message\n")

	// Start a goroutine to read responses (needed to keep connection alive)
	done := make(chan struct{})
	frameCount := 0
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
					if data[0] == WsMsgVideoFrame {
						frameCount++
						if frameCount == 1 {
							fmt.Printf("✅ Receiving video frames\n")
						}
					} else if data[0] == WsMsgStreamInit {
						fmt.Printf("✅ Received stream init\n")
					}
				} else if msgType == websocket.TextMessage {
					fmt.Printf("📩 Text message: %s\n", string(data))
				}
			}
		}
	}()

	// Wait for stream to initialize (receive at least one frame)
	time.Sleep(2 * time.Second)
	if frameCount == 0 {
		fmt.Printf("⚠️  No video frames received yet, but continuing with keyboard test\n")
	}

	fmt.Printf("\n⌨️  Sending %d keyboard event(s)...\n", count)
	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")

	for i := 0; i < count; i++ {
		// Keyboard message format (matches ws_protocol.rs):
		// [WsMsgKeyboardInput(1)] [subType(1)] [isDown(1)] [modifiers(1)] [keyCode(2 bytes big-endian)]
		// subType: 0 = key input, 1 = text input
		// modifiers: bitmask (shift=1, ctrl=2, alt=4, win=8)

		// Send key down
		keyDownMsg := []byte{
			WsMsgKeyboardInput, // msg type
			0x00,               // subType = key input
			0x01,               // isDown = true
			0x00,               // modifiers = none
			byte(keyCode >> 8), byte(keyCode & 0xFF), // keyCode big-endian
		}

		fmt.Printf("  [%d/%d] Sending key DOWN: 0x%02X (%d) - bytes: %v\n", i+1, count, keyCode, keyCode, keyDownMsg)
		if err := conn.WriteMessage(websocket.BinaryMessage, keyDownMsg); err != nil {
			return fmt.Errorf("failed to send key down: %w", err)
		}

		// Short delay between down and up
		time.Sleep(50 * time.Millisecond)

		// Send key up
		keyUpMsg := []byte{
			WsMsgKeyboardInput, // msg type
			0x00,               // subType = key input
			0x00,               // isDown = false
			0x00,               // modifiers = none
			byte(keyCode >> 8), byte(keyCode & 0xFF), // keyCode big-endian
		}

		fmt.Printf("  [%d/%d] Sending key UP:   0x%02X (%d) - bytes: %v\n", i+1, count, keyCode, keyCode, keyUpMsg)
		if err := conn.WriteMessage(websocket.BinaryMessage, keyUpMsg); err != nil {
			return fmt.Errorf("failed to send key up: %w", err)
		}

		if i < count-1 {
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}
	}

	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")
	fmt.Printf("✅ Sent %d keyboard event(s)\n", count)
	fmt.Printf("📊 Received %d video frames during test\n", frameCount)
	fmt.Printf("\n💡 Check sandbox logs for keyboard handling:\n")
	fmt.Printf("   docker compose logs --tail 100 sandbox-nvidia 2>&1 | grep -E 'keyboard|Keyboard|WsStream'\n")

	// Clean close
	close(done)
	conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	time.Sleep(100 * time.Millisecond)

	return nil
}

// newScrollTestCommand creates a command to test scroll input via DirectInput WebSocket
func newScrollTestCommand() *cobra.Command {
	var deltaX, deltaY float32
	var deltaMode int
	var count int
	var delay int
	var trackpad bool

	cmd := &cobra.Command{
		Use:   "scroll-test <session-id>",
		Short: "Test scroll input over DirectInput WebSocket",
		Long: `Connects to the DirectInput WebSocket and sends scroll events to debug the scroll path.

This command tests the DirectInput WebSocket path that bypasses Moonlight/Wolf:
  Browser -> Helix API -> RevDial -> screenshot-server -> D-Bus -> GNOME

Uses the same binary protocol as the browser's DirectInputWebSocket class.

Examples:
  # Send a scroll down event (positive deltaY = scroll down in browser terms)
  helix spectask scroll-test ses_01xxx

  # Send scroll up (negative deltaY)
  helix spectask scroll-test ses_01xxx --deltaY -120

  # Send horizontal scroll
  helix spectask scroll-test ses_01xxx --deltaX 100

  # Send multiple scroll events
  helix spectask scroll-test ses_01xxx --count 10 --delay 50

  # Simulate trackpad scroll (smooth, small deltas)
  helix spectask scroll-test ses_01xxx --trackpad --deltaY -30 --count 20 --delay 20
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			return runScrollTest(apiURL, token, sessionID, deltaX, deltaY, deltaMode, trackpad, count, delay)
		},
	}

	cmd.Flags().Float32Var(&deltaX, "deltaX", 0, "Horizontal scroll delta (positive = right)")
	cmd.Flags().Float32Var(&deltaY, "deltaY", 120, "Vertical scroll delta (positive = down, negative = up)")
	cmd.Flags().IntVar(&deltaMode, "deltaMode", 0, "Delta mode: 0=pixel, 1=line, 2=page")
	cmd.Flags().BoolVar(&trackpad, "trackpad", false, "Simulate trackpad scroll (sets is_trackpad flag)")
	cmd.Flags().IntVarP(&count, "count", "n", 1, "Number of scroll events to send")
	cmd.Flags().IntVarP(&delay, "delay", "d", 100, "Delay between scroll events in ms")

	return cmd
}

// Message type constants (must match ws_input.go and direct-input.ts)
const (
	MsgTypeScroll = 0x05
)

// runScrollTest connects to DirectInput WebSocket and sends scroll events
func runScrollTest(apiURL, token, sessionID string, deltaX, deltaY float32, deltaMode int, trackpad bool, count, delayMs int) error {
	fmt.Printf("🖱️  Scroll Test - Session %s\n", sessionID)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("deltaX=%.1f, deltaY=%.1f, deltaMode=%d, trackpad=%v, count=%d, delay=%dms\n\n",
		deltaX, deltaY, deltaMode, trackpad, count, delayMs)

	// Build WebSocket URL for DirectInput endpoint
	wsURL := strings.Replace(apiURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	inputURL := fmt.Sprintf("%s/api/v1/external-agents/%s/ws/input?access_token=%s",
		wsURL, url.QueryEscape(sessionID), url.QueryEscape(token))

	fmt.Printf("📡 Connecting to: %s\n", strings.Replace(inputURL, token, "***", 1))

	header := http.Header{}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.Dial(inputURL, header)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("WebSocket dial failed: %s", string(body))
		}
		return fmt.Errorf("WebSocket dial failed: %w", err)
	}
	defer conn.Close()

	fmt.Printf("✅ WebSocket connected\n\n")

	fmt.Printf("🖱️  Sending %d scroll event(s)...\n", count)
	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")

	for i := 0; i < count; i++ {
		// Build binary message: [type:1][deltaMode:1][flags:1][deltaX:4][deltaY:4] = 11 bytes
		// This matches the format expected by ws_input.go and direct-input.ts
		buf := make([]byte, 11)
		buf[0] = MsgTypeScroll
		buf[1] = byte(deltaMode)
		if trackpad {
			buf[2] = 0x01 // is_trackpad flag
		} else {
			buf[2] = 0x00
		}

		// deltaX as little-endian float32 at offset 3
		putFloat32LE(buf[3:7], deltaX)
		// deltaY as little-endian float32 at offset 7
		putFloat32LE(buf[7:11], deltaY)

		fmt.Printf("  [%d/%d] Sending scroll: deltaX=%.1f deltaY=%.1f mode=%d trackpad=%v\n",
			i+1, count, deltaX, deltaY, deltaMode, trackpad)
		fmt.Printf("         bytes: %v\n", buf)

		if err := conn.WriteMessage(websocket.BinaryMessage, buf); err != nil {
			return fmt.Errorf("failed to send scroll: %w", err)
		}

		if i < count-1 {
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}
	}

	fmt.Printf("───────────────────────────────────────────────────────────────────────────────\n")
	fmt.Printf("✅ Sent %d scroll event(s)\n", count)
	fmt.Printf("\n💡 Check sandbox logs for scroll handling:\n")
	fmt.Printf("   docker compose exec -T sandbox docker logs <container> 2>&1 | grep -i scroll\n")

	// Clean close
	conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	time.Sleep(100 * time.Millisecond)

	return nil
}

// putFloat32LE writes a float32 to a byte slice in little-endian format
func putFloat32LE(b []byte, f float32) {
	bits := *(*uint32)(unsafe.Pointer(&f))
	b[0] = byte(bits)
	b[1] = byte(bits >> 8)
	b[2] = byte(bits >> 16)
	b[3] = byte(bits >> 24)
}
