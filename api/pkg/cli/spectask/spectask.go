package spectask

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

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
	ID         string                 `json:"id"`
	Mode       string                 `json:"mode"`
	Type       string                 `json:"type"`
	ModelName  string                 `json:"model_name"`
	Metadata   SessionMetadata        `json:"metadata"`
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
