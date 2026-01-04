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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

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
	cmd.AddCommand(newStreamStatsCommand())
	cmd.AddCommand(newStopCommand())

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
			filename := fmt.Sprintf("screenshot-%s.png", time.Now().Format("20060102-150405"))
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

func newStreamStatsCommand() *cobra.Command {
	var duration int
	var outputFile string
	var verbose bool

	cmd := &cobra.Command{
		Use:   "stream-stats <session-id>",
		Short: "Connect to video stream and print detailed statistics",
		Long: `Connects to the Moonlight WebSocket and prints real-time statistics about the video stream.

This command is useful for development and testing to verify:
1. Video encoding is working
2. Frame rates and bitrates
3. Message types being exchanged
4. End-to-end latency

Examples:
  helix spectask stream-stats ses_01xxx                    # Run until Ctrl+C
  helix spectask stream-stats ses_01xxx --duration 30      # Run for 30 seconds
  helix spectask stream-stats ses_01xxx --output video.dat # Save video data to file
  helix spectask stream-stats ses_01xxx -v                 # Verbose output
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			// Build WebSocket URL
			wsURL := strings.Replace(apiURL, "http://", "ws://", 1)
			wsURL = strings.Replace(wsURL, "https://", "wss://", 1)

			moonlightSessionID := "agent-" + sessionID
			streamURL := fmt.Sprintf("%s/moonlight/host/stream?session_id=%s", wsURL, url.QueryEscape(moonlightSessionID))

			fmt.Printf("📊 Stream Statistics for session %s\n", sessionID)
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			fmt.Printf("WebSocket URL: %s\n", streamURL)
			if duration > 0 {
				fmt.Printf("Duration: %d seconds\n", duration)
			}
			if outputFile != "" {
				fmt.Printf("Output file: %s\n", outputFile)
			}
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

			// Connect to WebSocket
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

			// Fetch Wolf app ID for this session (needed for AuthenticateAndInit)
			// For lobby-based sessions (external agents), we need the placeholder app ID,
			// not the wolf_app_id from session state (which is a lobby config, not a Wolf app)
			wolfAppID := 0

			// First check if this is a lobby-based session by checking session config for wolf_lobby_id
			isLobbyBased := false
			sessionURL := fmt.Sprintf("%s/api/v1/sessions/%s", apiURL, sessionID)
			sessionReq, _ := http.NewRequest("GET", sessionURL, nil)
			sessionReq.Header.Set("Authorization", "Bearer "+token)
			sessionResp, err := (&http.Client{Timeout: 5 * time.Second}).Do(sessionReq)
			if err == nil && sessionResp.StatusCode == http.StatusOK {
				var sessionData struct {
					Config struct {
						WolfLobbyID  string `json:"wolf_lobby_id"`
						WolfLobbyPIN string `json:"wolf_lobby_pin"`
					} `json:"config"`
				}
				if json.NewDecoder(sessionResp.Body).Decode(&sessionData) == nil {
					if sessionData.Config.WolfLobbyID != "" {
						isLobbyBased = true
						fmt.Printf("🎪 Lobby-based session (lobby: %s, PIN: %s)\n", sessionData.Config.WolfLobbyID, sessionData.Config.WolfLobbyPIN)
					}
				}
				sessionResp.Body.Close()
			}

			// For lobby-based sessions, fetch the placeholder app ID (Select Agent / Blank)
			if isLobbyBased {
				uiAppURL := fmt.Sprintf("%s/api/v1/wolf/ui-app-id?session_id=%s", apiURL, sessionID)
				uiAppReq, _ := http.NewRequest("GET", uiAppURL, nil)
				uiAppReq.Header.Set("Authorization", "Bearer "+token)
				uiAppResp, err := (&http.Client{Timeout: 5 * time.Second}).Do(uiAppReq)
				if err == nil && uiAppResp.StatusCode == http.StatusOK {
					var uiAppState struct {
						PlaceholderAppID string `json:"placeholder_app_id"`
					}
					if json.NewDecoder(uiAppResp.Body).Decode(&uiAppState) == nil && uiAppState.PlaceholderAppID != "" {
						if parsed, err := strconv.Atoi(uiAppState.PlaceholderAppID); err == nil {
							wolfAppID = parsed
							fmt.Printf("📱 Using placeholder app ID: %d (Select Agent)\n", wolfAppID)
						}
					}
					uiAppResp.Body.Close()
				} else if err != nil {
					fmt.Printf("⚠️  Failed to fetch placeholder app ID: %v\n", err)
				} else {
					body, _ := io.ReadAll(uiAppResp.Body)
					uiAppResp.Body.Close()
					fmt.Printf("⚠️  Failed to fetch placeholder app ID: %d - %s\n", uiAppResp.StatusCode, string(body))
				}
			}

			// Send AuthenticateAndInit message to start the stream
			// This follows the moonlight-web signaling protocol
			clientUniqueID := fmt.Sprintf("cli-%d", time.Now().UnixNano())
			authMessage := map[string]interface{}{
				"AuthenticateAndInit": map[string]interface{}{
					"credentials":              token, // Will be replaced by proxy with moonlight creds
					"session_id":               moonlightSessionID,
					"mode":                     "create",
					"client_unique_id":         clientUniqueID,
					"host_id":                  0, // Wolf local mode
					"app_id":                   wolfAppID, // Wolf app ID from session state
					"bitrate":                  10000, // 10 Mbps
					"packet_size":              1024,
					"fps":                      60,
					"width":                    1920,
					"height":                   1080,
					"video_sample_queue_size":  16,
					"play_audio_local":         false,
					"audio_sample_queue_size":  16,
					"video_supported_formats":  1, // H264 = 1 (from moonlight-web api_bindings.ts)
					"video_colorspace":         "Rec709",
					"video_color_range_full":   true,
				},
			}

			authJSON, err := json.Marshal(authMessage)
			if err != nil {
				return fmt.Errorf("failed to marshal auth message: %w", err)
			}

			fmt.Printf("📤 Sending AuthenticateAndInit (mode=create, client=%s)...\n", clientUniqueID[:20])
			if err := conn.WriteMessage(websocket.TextMessage, authJSON); err != nil {
				return fmt.Errorf("failed to send auth message: %w", err)
			}
			fmt.Printf("✅ Auth message sent, waiting for responses...\n\n")

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
			stats := &streamStats{
				startTime:    time.Now(),
				messageSizes: make(map[int]int), // size bucket -> count
				messageTypes: make(map[int]int), // type -> count
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
					stats.messageTypes[msgType]++

					// Bucket sizes for histogram
					sizeBucket := (len(data) / 1024) * 1024 // Round to nearest KB
					stats.messageSizes[sizeBucket]++

					// Track min/max message sizes
					if stats.minMsgSize == 0 || len(data) < stats.minMsgSize {
						stats.minMsgSize = len(data)
					}
					if len(data) > stats.maxMsgSize {
						stats.maxMsgSize = len(data)
					}

					stats.lastMsgTime = time.Now()
					stats.mu.Unlock()

					// Write to file if specified
					if outFile != nil {
						outFile.Write(data)
					}

					// Verbose output or signaling message parsing
					typeStr := websocketMsgTypeStr(msgType)
					if msgType == websocket.TextMessage {
						// Parse signaling messages and display them
						var msg map[string]interface{}
						if err := json.Unmarshal(data, &msg); err == nil {
							// Check for various message types
							if _, ok := msg["StageStarting"]; ok {
								stage := msg["StageStarting"].(map[string]interface{})["stage"]
								fmt.Printf("📡 Stage starting: %v\n", stage)
							} else if _, ok := msg["StageComplete"]; ok {
								stage := msg["StageComplete"].(map[string]interface{})["stage"]
								fmt.Printf("✅ Stage complete: %v\n", stage)
							} else if _, ok := msg["StageFailed"]; ok {
								stageFailed := msg["StageFailed"].(map[string]interface{})
								fmt.Printf("❌ Stage failed: %v (error: %v)\n", stageFailed["stage"], stageFailed["error_code"])
							} else if _, ok := msg["WebRtcConfig"]; ok {
								fmt.Printf("🔧 WebRTC config received (ICE servers configured)\n")
							} else if _, ok := msg["ConnectionComplete"]; ok {
								fmt.Printf("🎉 Connection complete! Stream is active.\n")
							} else if _, ok := msg["ConnectionTerminated"]; ok {
								terminated := msg["ConnectionTerminated"].(map[string]interface{})
								fmt.Printf("🛑 Connection terminated (error: %v)\n", terminated["error_code"])
							} else if _, ok := msg["Signaling"]; ok {
								signaling := msg["Signaling"].(map[string]interface{})
								if _, ok := signaling["Description"]; ok {
									desc := signaling["Description"].(map[string]interface{})
									fmt.Printf("📝 SDP %v received\n", desc["ty"])
								} else if _, ok := signaling["AddIceCandidate"]; ok {
									fmt.Printf("🧊 ICE candidate received\n")
								}
							} else if verbose {
								// Unknown message type - show in verbose mode
								fmt.Printf("[%s] %s: %s\n",
									time.Now().Format("15:04:05.000"),
									typeStr,
									string(data)[:min(100, len(data))])
							}
						} else if verbose {
							// Print the actual content for debugging
							fmt.Printf("[%s] %s: %d bytes - content: %q\n",
								time.Now().Format("15:04:05.000"),
								typeStr,
								len(data),
								string(data))
						}
					} else if verbose {
						fmt.Printf("[%s] %s: %d bytes\n",
							time.Now().Format("15:04:05.000"),
							typeStr,
							len(data))
					}
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
				fmt.Printf("Messages received:  %d\n", stats.totalMessages)
				fmt.Printf("Total data:         %s\n", formatBytes(stats.totalBytes))

				if elapsed.Seconds() > 0 {
					msgRate := float64(stats.totalMessages) / elapsed.Seconds()
					bitrate := float64(stats.totalBytes*8) / elapsed.Seconds()
					fmt.Printf("Message rate:       %.2f msg/s\n", msgRate)
					fmt.Printf("Bitrate:            %s/s\n", formatBits(int64(bitrate)))
				}

				if stats.totalMessages > 0 {
					avgSize := stats.totalBytes / int64(stats.totalMessages)
					fmt.Printf("Avg message size:   %s\n", formatBytes(avgSize))
					fmt.Printf("Min message size:   %s\n", formatBytes(int64(stats.minMsgSize)))
					fmt.Printf("Max message size:   %s\n", formatBytes(int64(stats.maxMsgSize)))
				}

				fmt.Printf("\nMessage types:\n")
				for msgType, count := range stats.messageTypes {
					fmt.Printf("  %s: %d\n", websocketMsgTypeStr(msgType), count)
				}

				if final && len(stats.messageSizes) > 0 {
					fmt.Printf("\nMessage size distribution:\n")
					for size, count := range stats.messageSizes {
						pct := float64(count) / float64(stats.totalMessages) * 100
						fmt.Printf("  %s-%s: %d (%.1f%%)\n",
							formatBytes(int64(size)),
							formatBytes(int64(size+1024)),
							count, pct)
					}
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
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Write video data to file")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Print each message received")

	return cmd
}

// streamStats tracks statistics for stream-stats command
type streamStats struct {
	mu            sync.Mutex
	startTime     time.Time
	totalMessages int
	totalBytes    int64
	minMsgSize    int
	maxMsgSize    int
	lastMsgTime   time.Time
	messageSizes  map[int]int // size bucket -> count
	messageTypes  map[int]int // websocket message type -> count
}

func websocketMsgTypeStr(t int) string {
	switch t {
	case websocket.TextMessage:
		return "Text"
	case websocket.BinaryMessage:
		return "Binary"
	case websocket.CloseMessage:
		return "Close"
	case websocket.PingMessage:
		return "Ping"
	case websocket.PongMessage:
		return "Pong"
	default:
		return fmt.Sprintf("Unknown(%d)", t)
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

func newStreamCommand() *cobra.Command {
	var keepAlive bool
	var timeout int
	var vlcServer string
	var keyboard bool
	var sendText string

	cmd := &cobra.Command{
		Use:   "stream <session-id>",
		Short: "Start video streaming for a session and return browser URL",
		Long: `Connects to the Moonlight WebSocket to trigger video streaming.

This command:
1. Connects to the Moonlight signaling WebSocket (triggers Wolf to start encoding)
2. Keeps the connection open to maintain the stream
3. Returns the URL to view in browser

The video stream uses WebRTC and requires a browser to view. This command
triggers the stream so you can verify the video pipeline is working.

VLC Server Mode:
When --vlc-server is specified, starts a local HTTP server that VLC can connect to.
The raw video data from the Moonlight WebSocket is streamed to any HTTP clients.

Keyboard Mode:
When --keyboard is specified, captures terminal input and sends to the sandbox.
This allows you to type commands directly into the remote desktop.

Example:
  helix spectask stream ses_01xxx
  helix spectask stream ses_01xxx --keep-alive   # Keep stream running
  helix spectask stream ses_01xxx --vlc-server :8889  # Start VLC server
  helix spectask stream ses_01xxx --keyboard     # Enable keyboard input
  helix spectask stream ses_01xxx --text "ls -la"  # Send text and exit
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			// Build WebSocket URL
			// Convert http:// to ws:// or https:// to wss://
			wsURL := strings.Replace(apiURL, "http://", "ws://", 1)
			wsURL = strings.Replace(wsURL, "https://", "wss://", 1)

			// Moonlight Web WebSocket endpoint
			// Format: /moonlight/host/stream?session_id=agent-{session_id}
			moonlightSessionID := "agent-" + sessionID
			streamURL := fmt.Sprintf("%s/moonlight/host/stream?session_id=%s", wsURL, url.QueryEscape(moonlightSessionID))

			fmt.Printf("🎬 Starting video stream for session %s...\n", sessionID)
			fmt.Printf("   WebSocket URL: %s\n", streamURL)

			// Build browser URL for viewing
			browserURL := fmt.Sprintf("%s/dashboard/sessions/%s?stream=true", apiURL, sessionID)
			fmt.Printf("\n📺 View in browser:\n   %s\n", browserURL)

			// Connect to WebSocket with auth header
			header := http.Header{}
			header.Set("Authorization", "Bearer "+token)

			dialer := websocket.Dialer{
				HandshakeTimeout: 10 * time.Second,
			}

			conn, resp, err := dialer.Dial(streamURL, header)
			if err != nil {
				if resp != nil {
					body, _ := io.ReadAll(resp.Body)
					return fmt.Errorf("WebSocket connection failed: %w - %s", err, string(body))
				}
				return fmt.Errorf("WebSocket connection failed: %w", err)
			}
			defer conn.Close()

			fmt.Printf("\n✅ WebSocket connected! Video stream is active.\n")

			// Send text mode (non-interactive) - just send and exit
			if sendText != "" {
				fmt.Printf("\n⌨️  Sending text to sandbox: %q\n", sendText)
				if err := sendTextToSession(apiURL, token, sessionID, sendText); err != nil {
					return fmt.Errorf("failed to send text: %w", err)
				}
				return nil
			}

			// Interactive mode: VLC server + keyboard + mouse
			if vlcServer != "" || keyboard {
				return runInteractiveStream(apiURL, token, sessionID, conn, vlcServer, keyboard, timeout)
			}

			if !keepAlive {
				// Just verify connection works, then close
				fmt.Println("   Stream triggered successfully. Connection will close.")
				fmt.Println("\n💡 Tip: Use --keep-alive to keep the stream running")

				// Keep connection for a few seconds to ensure stream starts
				time.Sleep(3 * time.Second)
				return nil
			}

			// Keep alive mode - maintain connection until interrupted
			fmt.Printf("   Keeping stream alive (timeout: %ds, Ctrl+C to stop)...\n", timeout)

			// Handle graceful shutdown
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

			// Ping/pong to keep connection alive
			done := make(chan struct{})
			go func() {
				defer close(done)
				for {
					_, _, err := conn.ReadMessage()
					if err != nil {
						if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
							fmt.Printf("WebSocket read error: %v\n", err)
						}
						return
					}
				}
			}()

			// Send periodic pings
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			timeoutChan := time.After(time.Duration(timeout) * time.Second)

			for {
				select {
				case <-sigChan:
					fmt.Println("\n🛑 Stopping stream...")
					conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					return nil
				case <-done:
					fmt.Println("Connection closed by server")
					return nil
				case <-timeoutChan:
					fmt.Println("\n⏰ Timeout reached, closing stream...")
					conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					return nil
				case <-ticker.C:
					if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
						fmt.Printf("Ping failed: %v\n", err)
						return nil
					}
				}
			}
		},
	}

	cmd.Flags().BoolVar(&keepAlive, "keep-alive", false, "Keep the stream running until interrupted")
	cmd.Flags().IntVar(&timeout, "timeout", 300, "Timeout in seconds for keep-alive mode")
	cmd.Flags().StringVar(&vlcServer, "vlc-server", "", "Start HTTP server for VLC (e.g., :8889)")
	cmd.Flags().BoolVar(&keyboard, "keyboard", false, "Enable keyboard input mode (type to send to sandbox)")
	cmd.Flags().StringVar(&sendText, "text", "", "Send text to sandbox and exit (non-interactive)")

	return cmd
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
