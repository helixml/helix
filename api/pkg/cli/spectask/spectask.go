package spectask

import (
	"bytes"
	"encoding/binary"
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

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
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
	cmd.AddCommand(newBenchmarkCommand())
	cmd.AddCommand(newLatencyCommand())
	cmd.AddCommand(newExecCommand())
	cmd.AddCommand(newCopyCommand())

	return cmd
}

func newStartCommand() *cobra.Command {
	var taskName string
	var projectID string
	var agentID string
	var prompt string
	var promptFile string
	var quiet bool

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
  3. Connect via browser:    Visit /sessions/<session-id> to access the desktop`,
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
				if !quiet {
					fmt.Println("Creating new spec task...")
				}
				taskPrompt := prompt
				// --prompt-file lets you dispatch an entire brief (e.g. a design
				// doc) as the task prompt without committing it to the repo.
				// Appended after --prompt when both are given.
				if promptFile != "" {
					data, readErr := os.ReadFile(promptFile)
					if readErr != nil {
						return fmt.Errorf("failed to read --prompt-file %q: %w", promptFile, readErr)
					}
					if taskPrompt != "" {
						taskPrompt = taskPrompt + "\n\n" + string(data)
					} else {
						taskPrompt = string(data)
					}
				}
				if taskPrompt == "" {
					taskPrompt = "Testing RevDial connectivity"
				}
				task, err := createSpecTask(apiURL, token, taskName, taskPrompt, projectID, agentID)
				if err != nil {
					return fmt.Errorf("failed to create spec task: %w", err)
				}
				taskID = task.ID
				if !quiet {
					fmt.Printf("✅ Created spec task: %s (ID: %s)\n", task.Name, task.ID)
					if agentID != "" {
						fmt.Printf("   Agent: %s\n", agentID)
					}
				}
			}

			// Start planning - this triggers async session creation
			if !quiet {
				fmt.Printf("Starting planning for task %s...\n", taskID)
			}
			task, err := triggerStartPlanning(apiURL, token, taskID)
			if err != nil {
				return fmt.Errorf("failed to start planning: %w", err)
			}
			if !quiet {
				fmt.Printf("✅ Task status: %s\n", task.Status)
			}

			// Poll for session to be created (sandbox takes ~10-15s to start)
			if !quiet {
				fmt.Printf("⏳ Waiting for sandbox to start (this takes ~15 seconds)...\n")
			}
			session, err := waitForTaskSession(apiURL, token, taskID, 3*time.Minute)
			if err != nil {
				return fmt.Errorf("failed waiting for session: %w", err)
			}

			if quiet {
				fmt.Println(session.ID)
			} else {
				fmt.Printf("\n✅ Sandbox is running!\n")
				fmt.Printf("   Session ID: %s\n", session.ID)

				// Show connection instructions
				fmt.Printf("\n📺 Connect to Desktop:\n")
				fmt.Printf("   Open in browser: %s/sessions/%s\n", apiURL, session.ID)

				fmt.Printf("\n📷 Test screenshot:\n")
				fmt.Printf("   helix spectask screenshot %s\n", session.ID)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&taskName, "name", "n", "CLI Test Task", "Task name")
	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Project ID (required when creating new task)")
	cmd.Flags().StringVarP(&agentID, "agent", "a", "", "Agent/App ID to use (e.g., app_01xxx)")
	cmd.Flags().StringVar(&prompt, "prompt", "", "Task prompt/description")
	cmd.Flags().StringVar(&promptFile, "prompt-file", "", "Read the task prompt from a file (e.g. a design doc) — dispatch a full brief without committing it to the repo. Appended after --prompt if both are set.")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Only output session ID")

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
				// Only show sessions with dev container (external agent sessions)
				if s.Metadata.DevContainerID != "" {
					count++
					fmt.Printf("Session: %s\n", s.ID)
					fmt.Printf("  Type: %s\n", s.Type)
					if s.Metadata.ContainerName != "" {
						fmt.Printf("  Container: %s\n", s.Metadata.ContainerName)
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

	url := fmt.Sprintf("%s/api/v1/sessions/%s/stop-external-agent", apiURL, sessionID)
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
		if s.Metadata.DevContainerID != "" {
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
	ID        string          `json:"session_id"`
	Mode      string          `json:"mode"`
	Type      string          `json:"type"`
	ModelName string          `json:"model_name"`
	Metadata  SessionMetadata `json:"metadata"`
}

type SessionMetadata struct {
	ContainerName   string `json:"container_name"`
	ContainerID     string `json:"container_id"`
	ExecutorMode    string `json:"executor_mode"`
	DevContainerID  string `json:"dev_container_id"`
	ExternalAgentID string `json:"external_agent_id"`
	SpecTaskID      string `json:"spec_task_id"`
	StatusMessage   string `json:"status_message"`
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

// waitForTaskSession polls for a session with dev_container_id to be created for the task
func waitForTaskSession(apiURL, token, taskID string, timeout time.Duration) (*Session, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second
	lastStatusMsg := ""

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

		// Find session for this task that has a running sandbox
		for _, s := range response.Sessions {
			if s.Metadata.SpecTaskID == taskID {
				// Show status message updates (e.g., "Unpacking build cache (2.1/7.0 GB)")
				if s.Metadata.StatusMessage != "" && s.Metadata.StatusMessage != lastStatusMsg {
					fmt.Printf("   %s\n", s.Metadata.StatusMessage)
					lastStatusMsg = s.Metadata.StatusMessage
				}
				// Check for active container
				if s.Metadata.DevContainerID != "" || s.Metadata.ContainerID != "" {
					return &s, nil
				}
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
			if containerID, ok := result["dev_container_id"].(string); ok {
				fmt.Printf("   Container ID: %s\n", containerID)
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
	var (
		orgFlag        string
		zedExternalOnly bool
	)
	cmd := &cobra.Command{
		Use:   "list-agents",
		Short: "List agents (apps) in an organization",
		Long: `List agents in an organization.

Agents are organization-scoped. If --org is omitted and you belong to a
single org, that org is used; if you belong to multiple, you will be
prompted. The HELIX_ORG environment variable is also honoured.

By default every agent the user has access to in the org is listed, with
the assistant type shown so it is obvious which can be launched with
"helix spectask start" (zed_external) vs other agent kinds. Pass
--zed-external-only to restore the old behaviour and hide non-spec-task
agents.`,
		Aliases: []string{"agents"},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			apiClient, err := client.NewClientFromEnv()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			orgID, err := cli.ResolveOrganizationInteractive(ctx, apiClient, orgFlag)
			if err != nil {
				return err
			}

			apiURL := getAPIURL()
			token := getToken()

			q := url.Values{}
			q.Set("organization_id", orgID)
			endpoint := apiURL + "/api/v1/apps?" + q.Encode()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)

			httpClient := &http.Client{}
			resp, err := httpClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			var apps []App
			if err := json.NewDecoder(resp.Body).Decode(&apps); err != nil {
				return fmt.Errorf("failed to parse apps: %w", err)
			}

			fmt.Println("Available Agents:")
			fmt.Println()

			shown := 0
			for _, app := range apps {
				// Surface the most relevant assistant per app: prefer zed_external
				// (the only kind launchable via `spectask start` today), fall back
				// to the first assistant otherwise so non-spec-task agents are
				// still visible to the user.
				var primary *Assistant
				for i, assistant := range app.Config.Helix.Assistants {
					if assistant.AgentType == "zed_external" {
						primary = &app.Config.Helix.Assistants[i]
						break
					}
				}
				if primary == nil && len(app.Config.Helix.Assistants) > 0 {
					if zedExternalOnly {
						continue
					}
					primary = &app.Config.Helix.Assistants[0]
				}
				if primary == nil {
					// App with no assistants at all - skip.
					continue
				}

				shown++
				usable := primary.AgentType == "zed_external"
				marker := " (not launchable via spectask start)"
				if usable {
					marker = ""
				}

				fmt.Printf("App: %s%s\n", app.Name, marker)
				fmt.Printf("  ID: %s\n", app.ID)
				fmt.Printf("  Assistant: %s\n", primary.Name)
				if primary.AgentType != "" {
					fmt.Printf("  Agent type: %s\n", primary.AgentType)
				}
				if primary.CodeAgentRuntime != "" {
					fmt.Printf("  Runtime: %s\n", primary.CodeAgentRuntime)
				}
				if primary.Model != "" {
					fmt.Printf("  Model: %s\n", primary.Model)
				}
				if usable {
					fmt.Printf("  Usage: helix spectask start --project <prj_id> --agent %s -n \"Task name\"\n", app.ID)
				}
				fmt.Println()
			}

			switch {
			case shown == 0 && zedExternalOnly:
				fmt.Println("No agents with zed_external assistants found in this org.")
				fmt.Println("Create one in the Helix UI, or drop --zed-external-only to see all agents.")
			case shown == 0:
				fmt.Println("No agents found in this org. Create one in the Helix UI first.")
			default:
				fmt.Printf("Found %d agent(s) in this org.\n", shown)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&orgFlag, "org", "o", "", "Organization name or ID (defaults to $HELIX_ORG, then your only org, or prompts)")
	cmd.Flags().BoolVar(&zedExternalOnly, "zed-external-only", false, "Hide agents whose assistant is not zed_external (the only kind launchable via spectask start today)")
	return cmd
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
	WsMsgCursorImage   = 0x50
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

// SPSInfo contains parsed H.264 SPS constraint information
type SPSInfo struct {
	ProfileIDC         byte
	ConstraintSet0Flag bool
	ConstraintSet1Flag bool
	ConstraintSet2Flag bool
	ConstraintSet3Flag bool
	ConstraintSet4Flag bool
	ConstraintSet5Flag bool
	LevelIDC           byte
	RawConstraintByte  byte
}

// parseSPSFromH264 finds and parses the SPS NAL unit from H.264 Annex B data.
// Returns nil if no SPS found.
func parseSPSFromH264(data []byte) *SPSInfo {
	// H.264 Annex B uses start codes: 0x00000001 or 0x000001
	// NAL unit type is in bits 0-4 of the first byte after start code
	// SPS NAL unit type = 7

	// Find all NAL units
	for i := 0; i < len(data)-4; i++ {
		// Check for 4-byte start code (0x00000001)
		if data[i] == 0x00 && data[i+1] == 0x00 && data[i+2] == 0x00 && data[i+3] == 0x01 {
			if i+4 < len(data) {
				nalType := data[i+4] & 0x1F
				if nalType == 7 && i+7 < len(data) { // SPS
					// SPS structure:
					// byte 0: NAL header
					// byte 1: profile_idc
					// byte 2: constraint_set flags
					// byte 3: level_idc
					constraintByte := data[i+6]
					return &SPSInfo{
						ProfileIDC:         data[i+5],
						ConstraintSet0Flag: (constraintByte & 0x80) != 0,
						ConstraintSet1Flag: (constraintByte & 0x40) != 0,
						ConstraintSet2Flag: (constraintByte & 0x20) != 0,
						ConstraintSet3Flag: (constraintByte & 0x10) != 0,
						ConstraintSet4Flag: (constraintByte & 0x08) != 0,
						ConstraintSet5Flag: (constraintByte & 0x04) != 0,
						LevelIDC:           data[i+7],
						RawConstraintByte:  constraintByte,
					}
				}
			}
		}
		// Check for 3-byte start code (0x000001)
		if data[i] == 0x00 && data[i+1] == 0x00 && data[i+2] == 0x01 {
			if i+3 < len(data) {
				nalType := data[i+3] & 0x1F
				if nalType == 7 && i+6 < len(data) { // SPS
					constraintByte := data[i+5]
					return &SPSInfo{
						ProfileIDC:         data[i+4],
						ConstraintSet0Flag: (constraintByte & 0x80) != 0,
						ConstraintSet1Flag: (constraintByte & 0x40) != 0,
						ConstraintSet2Flag: (constraintByte & 0x20) != 0,
						ConstraintSet3Flag: (constraintByte & 0x10) != 0,
						ConstraintSet4Flag: (constraintByte & 0x08) != 0,
						ConstraintSet5Flag: (constraintByte & 0x04) != 0,
						LevelIDC:           data[i+6],
						RawConstraintByte:  constraintByte,
					}
				}
			}
		}
	}
	return nil
}

// profileName returns a human-readable name for H.264 profile_idc
func profileName(profileIDC byte) string {
	switch profileIDC {
	case 66:
		return "Baseline"
	case 77:
		return "Main"
	case 88:
		return "Extended"
	case 100:
		return "High"
	case 110:
		return "High 10"
	case 122:
		return "High 4:2:2"
	case 244:
		return "High 4:4:4 Predictive"
	default:
		return fmt.Sprintf("Unknown(%d)", profileIDC)
	}
}

// renderCursorASCII renders cursor bitmap data as ASCII art
// Supports RGBA8888 and ARGB8888 formats, down-samples to maxSize x maxSize
// Also returns sample pixel values for debugging
func renderCursorASCII(data []byte, width, height, stride int, format uint32, maxSize int) (string, string) {
	if width == 0 || height == 0 || len(data) < 4 {
		return "", ""
	}

	// Characters from transparent to opaque (using brightness for color)
	// Using block characters for better visibility
	chars := []rune{' ', '░', '▒', '▓', '█'}

	// Calculate step for down-sampling
	stepX := (width + maxSize - 1) / maxSize
	stepY := (height + maxSize - 1) / maxSize
	if stepX < 1 {
		stepX = 1
	}
	if stepY < 1 {
		stepY = 1
	}

	outWidth := (width + stepX - 1) / stepX
	outHeight := (height + stepY - 1) / stepY

	var result strings.Builder
	var pixelSamples strings.Builder

	// Sample first few pixels for debugging
	pixelSamples.WriteString("Sample pixels (first 8): ")
	for i := 0; i < 8 && i*4+3 < len(data); i++ {
		r, g, b, a := data[i*4], data[i*4+1], data[i*4+2], data[i*4+3]
		// Interpret based on format
		switch format {
		case 0x34325241: // ARGB8888: A,R,G,B bytes
			a, r, g, b = data[i*4], data[i*4+1], data[i*4+2], data[i*4+3]
		case 0x34324152: // RGBA8888: R,G,B,A bytes
			r, g, b, a = data[i*4], data[i*4+1], data[i*4+2], data[i*4+3]
		case 0x34324142: // BGRA8888: B,G,R,A bytes
			b, g, r, a = data[i*4], data[i*4+1], data[i*4+2], data[i*4+3]
		case 0x34324241: // ABGR8888: A,B,G,R bytes
			a, b, g, r = data[i*4], data[i*4+1], data[i*4+2], data[i*4+3]
		}
		if i > 0 {
			pixelSamples.WriteString(" ")
		}
		pixelSamples.WriteString(fmt.Sprintf("(%d,%d,%d,%d)", r, g, b, a))
	}

	for y := 0; y < outHeight; y++ {
		srcY := y * stepY
		if srcY >= height {
			break
		}
		for x := 0; x < outWidth; x++ {
			srcX := x * stepX
			if srcX >= width {
				break
			}

			// Get pixel from source
			srcIdx := srcY*stride + srcX*4
			if srcIdx+3 >= len(data) {
				result.WriteRune(' ')
				continue
			}

			var r, g, b, a byte
			switch format {
			case 0x34325241: // ARGB8888: A,R,G,B bytes
				a = data[srcIdx]
				r = data[srcIdx+1]
				g = data[srcIdx+2]
				b = data[srcIdx+3]
			case 0x34324152: // RGBA8888: R,G,B,A bytes
				r = data[srcIdx]
				g = data[srcIdx+1]
				b = data[srcIdx+2]
				a = data[srcIdx+3]
			case 0x34324142: // BGRA8888: B,G,R,A bytes
				b = data[srcIdx]
				g = data[srcIdx+1]
				r = data[srcIdx+2]
				a = data[srcIdx+3]
			case 0x34324241: // ABGR8888: A,B,G,R bytes
				a = data[srcIdx]
				b = data[srcIdx+1]
				g = data[srcIdx+2]
				r = data[srcIdx+3]
			default: // Assume RGBA
				r = data[srcIdx]
				g = data[srcIdx+1]
				b = data[srcIdx+2]
				a = data[srcIdx+3]
			}

			// If alpha is 0, pixel is transparent
			if a == 0 {
				result.WriteRune(' ')
				continue
			}

			// Calculate brightness (0-255)
			brightness := (int(r) + int(g) + int(b)) / 3

			// Combine alpha and brightness for character selection
			// Higher alpha + higher brightness = more solid character
			combined := (int(a) + brightness) / 2

			// Map to character index
			charIdx := (combined * (len(chars) - 1)) / 255
			if charIdx >= len(chars) {
				charIdx = len(chars) - 1
			}
			if charIdx < 0 {
				charIdx = 0
			}
			result.WriteRune(chars[charIdx])
		}
		result.WriteRune('\n')
	}

	return result.String(), pixelSamples.String()
}

func newStreamCommand() *cobra.Command {
	var duration int
	var outputFile string
	var verbose bool
	var width, height, fps, bitrate int
	var videoMode string
	var mouseSweep bool

	cmd := &cobra.Command{
		Use:   "stream <session-id>",
		Short: "Connect to video stream and display real-time statistics",
		Long: `Connects to the WebSocket video streaming protocol and displays real-time statistics.

This command streams raw H.264/HEVC/AV1 video frames directly over WebSocket.
This is ideal for testing and debugging the video pipeline.

Statistics displayed:
  - Frame rate (FPS)
  - Bitrate (Mbps)
  - Frame sizes and types (keyframe vs delta)
  - Resolution and codec
  - Keyframe count

Video modes:
  - shm:      Shared memory path (default, most compatible, 1-2 CPU copies)
  - native:   Native GStreamer DMA-BUF (requires GStreamer 1.24+, fewer copies)
  - zerocopy: pipewirezerocopysrc plugin (true zero-copy, requires plugin)

Examples:
  helix spectask stream ses_01xxx                             # Run until Ctrl+C (1080p default)
  helix spectask stream ses_01xxx --width 3840 --height 2160  # Stream at 4K resolution
  helix spectask stream ses_01xxx --duration 30               # Run for 30 seconds
  helix spectask stream ses_01xxx --output video.h264         # Save raw video to file
  helix spectask stream ses_01xxx --video-mode zerocopy       # Test zero-copy streaming
  helix spectask stream ses_01xxx --mouse-sweep               # Sweep mouse to trigger cursor updates
  helix spectask stream ses_01xxx -v                          # Verbose mode (show each frame)
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			fmt.Printf("📊 Video Stream for session %s\n", sessionID)
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

			// Build WebSocket URL for direct streaming
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
			// Add video_mode if specified (overrides container's HELIX_VIDEO_MODE)
			if videoMode != "" {
				initMessage["video_mode"] = videoMode
				fmt.Printf("📤 Sending init message (%dx%d@%dfps, %dkbps, mode=%s)...\n",
					width, height, fps, bitrate, videoMode)
			} else {
				fmt.Printf("📤 Sending init message (%dx%d@%dfps, %dkbps)...\n",
					width, height, fps, bitrate)
			}
			initJSON, _ := json.Marshal(initMessage)
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

			// Mouse sweep goroutine - sends mouse movements via WebSocket to trigger cursor shape changes
			if mouseSweep {
				fmt.Printf("🖱️  Mouse sweep enabled - moving mouse horizontally across middle of screen\n")
				go func() {
					// Wait a moment for stream to initialize
					time.Sleep(2 * time.Second)

					// Sweep mouse from left to right across the middle of the screen
					// Using the same binary protocol as ws_input.go handleWSMouseAbsolute
					// Format: [type:1][subType:1][x:2 BE int16][y:2 BE int16][refWidth:2 BE int16][refHeight:2 BE int16]
					sweepY := int16(height / 2) // Middle of screen
					refW := int16(width)
					refH := int16(height)

					sweepNum := 0
					for {
						select {
						case <-done:
							return
						default:
						}

						sweepNum++
						fmt.Printf("🖱️  Starting mouse sweep #%d\n", sweepNum)

						// Sweep from left to right
						for x := int16(100); x < int16(width-100); x += 50 {
							select {
							case <-done:
								return
							default:
							}

							// Build mouse absolute message
							// Format: [type:1][subType:1][x:2 BE][y:2 BE][refWidth:2 BE][refHeight:2 BE] = 10 bytes
							msg := make([]byte, 10)
							msg[0] = WsMsgMouseAbsolute // 0x12
							msg[1] = 1                  // subType: 1 = absolute position
							binary.BigEndian.PutUint16(msg[2:4], uint16(x))
							binary.BigEndian.PutUint16(msg[4:6], uint16(sweepY))
							binary.BigEndian.PutUint16(msg[6:8], uint16(refW))
							binary.BigEndian.PutUint16(msg[8:10], uint16(refH))

							if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
								fmt.Printf("❌ Mouse sweep error: %v\n", err)
								return
							}

							// Slow sweep - 50ms between movements to allow cursor shape to update
							time.Sleep(50 * time.Millisecond)
						}

						// Wait before next sweep
						time.Sleep(3 * time.Second)
					}
				}()
			}

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

							// Track frames per second for instantaneous FPS
							elapsed := time.Since(stats.startTime)
							currentSec := int(elapsed.Seconds())
							if currentSec != stats.currentSecond {
								// New second - save last second's FPS and reset
								stats.lastSecondFPS = stats.currentSecondFPS
								stats.currentSecond = currentSec
								stats.currentSecondFPS = 1
							} else {
								stats.currentSecondFPS++
							}

							if len(data) >= 15 {
								codec := data[1]
								flags := data[2]
								isKeyframe := (flags & 0x01) != 0
								if stats.codec == 0 {
									stats.codec = codec
								}
								if isKeyframe {
									stats.keyframes++
									// Parse and log SPS from keyframes
									frameData := data[15:]
									if sps := parseSPSFromH264(frameData); sps != nil {
										// Always log first keyframe SPS, or all in verbose mode
										if stats.keyframes == 1 || verbose {
											b2i := func(b bool) int {
												if b {
													return 1
												}
												return 0
											}
											fmt.Printf("🔑 SPS: profile_idc=%d (%s) constraint_set=[%d%d%d%d%d%d] level=%d.%d (raw=0x%02x)\n",
												sps.ProfileIDC, profileName(sps.ProfileIDC),
												b2i(sps.ConstraintSet0Flag), b2i(sps.ConstraintSet1Flag),
												b2i(sps.ConstraintSet2Flag), b2i(sps.ConstraintSet3Flag),
												b2i(sps.ConstraintSet4Flag), b2i(sps.ConstraintSet5Flag),
												sps.LevelIDC/10, sps.LevelIDC%10,
												sps.RawConstraintByte)
											// Highlight constraint_set3_flag specifically for zero-latency decode
											if sps.ConstraintSet3Flag {
												fmt.Println("   ✅ constraint_set3_flag=1 (zero-latency decode enabled)")
											} else {
												fmt.Println("   ⚠️  constraint_set3_flag=0 (decoder may buffer frames)")
											}
										}
									} else if stats.keyframes == 1 {
										fmt.Println("⚠️  First keyframe: no SPS NAL found")
									}
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
						case WsMsgCursorImage:
							// Cursor image message
							// Format: type(1) + lastMoverID(4) + posX(4) + posY(4) + hotspotX(4) + hotspotY(4) + bitmapSize(4) + [format(4) + width(4) + height(4) + stride(4) + pixels...]
							stats.cursorMessages++
							if len(data) >= 25 {
								hotspotX := int(data[13]) | int(data[14])<<8 | int(data[15])<<16 | int(data[16])<<24
								hotspotY := int(data[17]) | int(data[18])<<8 | int(data[19])<<16 | int(data[20])<<24
								bitmapSize := int(data[21]) | int(data[22])<<8 | int(data[23])<<16 | int(data[24])<<24

								// If bitmap is present, extract width/height/stride from header
								// Header: format(4) + width(4) + height(4) + stride(4) = 16 bytes, then pixels
								if bitmapSize >= 16 && len(data) >= 41 {
									cursorFormat := uint32(data[25]) | uint32(data[26])<<8 | uint32(data[27])<<16 | uint32(data[28])<<24
									cursorWidth := int(data[29]) | int(data[30])<<8 | int(data[31])<<16 | int(data[32])<<24
									cursorHeight := int(data[33]) | int(data[34])<<8 | int(data[35])<<16 | int(data[36])<<24
									cursorStride := int(data[37]) | int(data[38])<<8 | int(data[39])<<16 | int(data[40])<<24
									stats.lastCursorWidth = cursorWidth
									stats.lastCursorHeight = cursorHeight
									stats.lastCursorHotspotX = hotspotX
									stats.lastCursorHotspotY = hotspotY

									// Extract pixel data (starts at offset 41)
									pixelDataStart := 41
									pixelDataEnd := 25 + bitmapSize // bitmapSize includes header
									if pixelDataEnd > len(data) {
										pixelDataEnd = len(data)
									}
									pixelData := data[pixelDataStart:pixelDataEnd]

									// Always show cursor info with ASCII art in verbose mode
									if verbose || stats.cursorMessages == 1 {
										fmt.Printf("\n🖱️  [%s] Cursor #%d: %dx%d hotspot=(%d,%d) stride=%d format=0x%08x (%d bytes)\n",
											time.Now().Format("15:04:05.000"), stats.cursorMessages,
											cursorWidth, cursorHeight, hotspotX, hotspotY, cursorStride, cursorFormat, len(pixelData))

										// Render ASCII art cursor (max 24 chars wide for terminal)
										asciiArt, pixelSamples := renderCursorASCII(pixelData, cursorWidth, cursorHeight, cursorStride, cursorFormat, 24)
										if asciiArt != "" {
											fmt.Printf("    %s\n", pixelSamples)
											// Indent each line of ASCII art
											for _, line := range strings.Split(asciiArt, "\n") {
												if line != "" {
													fmt.Printf("    |%s|\n", line)
												}
											}
										}
									}
								} else if verbose {
									fmt.Printf("🖱️  [%s] Cursor: no bitmap (hotspot=(%d,%d))\n",
										time.Now().Format("15:04:05.000"), hotspotX, hotspotY)
								}
							}
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
					// Show instantaneous FPS (last complete second) prominently
					instantFPS := stats.lastSecondFPS
					if instantFPS == 0 && stats.currentSecondFPS > 0 {
						// If we haven't completed a second yet, show current
						instantFPS = stats.currentSecondFPS
					}
					fmt.Printf("Instant FPS:        %d fps", instantFPS)
					if instantFPS > 0 && instantFPS < 50 {
						fmt.Printf(" ⚠️")
					} else if instantFPS >= 55 {
						fmt.Printf(" ✅")
					}
					fmt.Println()
					fmt.Printf("Average FPS:        %.1f fps\n", videoFps)
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

				// Cursor statistics
				if stats.cursorMessages > 0 {
					fmt.Printf("Cursor updates:     %d", stats.cursorMessages)
					if stats.lastCursorWidth > 0 {
						fmt.Printf(" (last: %dx%d hotspot=(%d,%d))",
							stats.lastCursorWidth, stats.lastCursorHeight,
							stats.lastCursorHotspotX, stats.lastCursorHotspotY)
					}
					fmt.Println()
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
	cmd.Flags().StringVar(&videoMode, "video-mode", "", "Video capture mode: shm, native, or zerocopy (default: container env)")
	cmd.Flags().BoolVar(&mouseSweep, "mouse-sweep", false, "Sweep mouse horizontally across middle of screen to trigger cursor shape changes")

	return cmd
}

// videoStreamStats tracks statistics for the stream command
type videoStreamStats struct {
	mu            sync.Mutex
	startTime     time.Time
	totalMessages int
	totalBytes    int64
	videoFrames   int
	videoBytes    int64
	audioFrames   int
	audioBytes    int64
	keyframes     int
	batchCount    int
	minFrameSize  int
	maxFrameSize  int
	codec         byte
	width         int
	height        int
	fps           int
	// Per-second frame tracking for instantaneous FPS
	currentSecond    int // Which second bucket we're in
	currentSecondFPS int // Frames received in current second
	lastSecondFPS    int // Frames received in previous second (displayed)
	// Cursor message tracking
	cursorMessages     int // Number of cursor messages received
	lastCursorWidth    int // Last cursor bitmap width
	lastCursorHeight   int // Last cursor bitmap height
	lastCursorHotspotX int // Last cursor hotspot X
	lastCursorHotspotY int // Last cursor hotspot Y
}


// getContainerAppID fetches the placeholder app ID for a session
// This is required for the AuthenticateAndInit message
// Deprecated: This endpoint is no longer used with direct WebSocket streaming
func getContainerAppID(apiURL, token, sessionID string) (int, error) {
	// Legacy endpoint - no longer used with direct streaming
	url := fmt.Sprintf("%s/api/v1/sessions/%s/app-id", apiURL, sessionID)

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

// configurePendingSession pre-configures the session with our client_unique_id
// This allows the streaming server to immediately attach us to the session when we connect
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
  Browser -> Helix API -> RevDial -> desktop container -> D-Bus -> GNOME

Uses evdev keycodes for input injection.

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

	// Get container app ID (legacy - may not be needed for direct streaming)
	appID, err := getContainerAppID(apiURL, token, sessionID)
	if err != nil {
		fmt.Printf("⚠️  Could not get app ID (continuing anyway): %v\n", err)
		appID = 0
	} else {
		fmt.Printf("✅ Got app ID: %d\n", appID)
	}

	// Configure pending session
	clientUniqueID := fmt.Sprintf("helix-kbd-test-%d", time.Now().UnixNano())
	if err := configurePendingSession(apiURL, token, sessionID, clientUniqueID); err != nil {
		return fmt.Errorf("failed to configure session: %w", err)
	}
	fmt.Printf("✅ Configured pending session: %s\n", clientUniqueID)

	// Build WebSocket URL for direct streaming
	wsURL := strings.Replace(apiURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	streamURL := fmt.Sprintf("%s/api/v1/external-agents/%s/ws/stream", wsURL, url.QueryEscape(sessionID))

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
			WsMsgKeyboardInput,                       // msg type
			0x00,                                     // subType = key input
			0x01,                                     // isDown = true
			0x00,                                     // modifiers = none
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
			WsMsgKeyboardInput,                       // msg type
			0x00,                                     // subType = key input
			0x00,                                     // isDown = false
			0x00,                                     // modifiers = none
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

This command tests the DirectInput WebSocket path:
  Browser -> Helix API -> RevDial -> desktop container -> D-Bus -> GNOME

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
