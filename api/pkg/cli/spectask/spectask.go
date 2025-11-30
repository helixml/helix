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

	return cmd
}

func newStartCommand() *cobra.Command {
	var taskName string

	cmd := &cobra.Command{
		Use:   "start [task-id]",
		Short: "Start a spec task planning session (creates sandbox)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiURL := getAPIURL()
			token := getToken()

			var taskID string
			if len(args) > 0 {
				taskID = args[0]
			} else {
				// Create a new spec task
				fmt.Println("Creating new spec task...")
				task, err := createSpecTask(apiURL, token, taskName, "Testing RevDial connectivity")
				if err != nil {
					return fmt.Errorf("failed to create spec task: %w", err)
				}
				taskID = task.ID
				fmt.Printf("✅ Created spec task: %s (ID: %s)\n", task.Name, task.ID)
			}

			// Start planning session
			fmt.Printf("Starting planning session for task %s...\n", taskID)
			session, err := startPlanningSession(apiURL, token, taskID)
			if err != nil {
				return fmt.Errorf("failed to start planning session: %w", err)
			}

			fmt.Printf("✅ Planning session started: %s\n", session.ID)
			fmt.Printf("   Container: %s\n", session.Metadata.ContainerName)
			fmt.Printf("   Waiting for sandbox to be ready...\n")

			// Wait for sandbox to be ready
			time.Sleep(10 * time.Second)

			fmt.Printf("✅ Sandbox should be running now\n")
			fmt.Printf("   Session ID: %s\n", session.ID)

			// Show connection instructions
			fmt.Printf("\n📺 Connect to Desktop:\n")
			if session.Metadata.WolfLobbyPIN != "" {
				fmt.Printf("   Wolf-UI (browser): Visit /wolf-ui and enter lobby PIN: %s\n", session.Metadata.WolfLobbyPIN)
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
}

func createSpecTask(apiURL, token, name, description string) (*SpecTask, error) {
	payload := map[string]string{
		"name":        name,
		"description": description,
	}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", apiURL+"/api/v1/spec-tasks", bytes.NewBuffer(jsonData))
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

func startPlanningSession(apiURL, token, taskID string) (*Session, error) {
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

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, err
	}

	return &session, nil
}
