package project

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

func newInspectCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "tasks <project-id>",
		Short:   "List all tasks for a project",
		Aliases: []string{"inspect"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			// Get project details
			req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/projects/%s", apiURL, projectID), nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("failed to get project: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("project not found (status %d)", resp.StatusCode)
			}

			var project Project
			if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
				return fmt.Errorf("failed to decode project: %w", err)
			}

			// Get tasks for project
			req, err = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/spec-tasks?project_id=%s", apiURL, projectID), nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err = client.Do(req)
			if err != nil {
				return fmt.Errorf("failed to get tasks: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("failed to get tasks (status %d)", resp.StatusCode)
			}

			var tasks []types.SpecTask
			if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
				return fmt.Errorf("failed to decode tasks: %w", err)
			}

			// Display project info
			fmt.Printf("\n📁 Project: %s\n", project.Name)
			fmt.Printf("ID: %s\n", project.ID)
			if project.Description != "" {
				fmt.Printf("Description: %s\n", project.Description)
			}
			if len(project.Technologies) > 0 {
				fmt.Printf("Technologies: %v\n", project.Technologies)
			}
			fmt.Printf("Status: %s\n\n", project.Status)

			// Group tasks by status
			backlog := []types.SpecTask{}
			inProgress := []types.SpecTask{}
			done := []types.SpecTask{}

			for _, task := range tasks {
				switch task.Status {
				case "backlog":
					backlog = append(backlog, task)
				case "in_progress", "planning", "implementing", "review":
					inProgress = append(inProgress, task)
				case "done", "completed":
					done = append(done, task)
				default:
					backlog = append(backlog, task)
				}
			}

			// Display tasks
			fmt.Printf("📋 Tasks (%d total)\n\n", len(tasks))

			if len(backlog) > 0 {
				fmt.Printf("🔵 Backlog (%d)\n", len(backlog))
				for _, task := range backlog {
					displayTask(task)
				}
				fmt.Println()
			}

			if len(inProgress) > 0 {
				fmt.Printf("🟡 In Progress (%d)\n", len(inProgress))
				for _, task := range inProgress {
					displayTask(task)
				}
				fmt.Println()
			}

			if len(done) > 0 {
				fmt.Printf("✅ Done (%d)\n", len(done))
				for _, task := range done {
					displayTask(task)
				}
				fmt.Println()
			}

			if len(tasks) > 0 {
				fmt.Printf("💡 To start planning a task:\n")
				fmt.Printf("   helix spectask start <task-id>\n\n")
			}

			return nil
		},
	}
}

func displayTask(task types.SpecTask) {
	fmt.Printf("  • %s\n", task.Name)
	fmt.Printf("    ID: %s | Priority: %s | Type: %s\n", task.ID, task.Priority, task.Type)
	if task.Description != "" && task.Description != task.Name {
		fmt.Printf("    Description: %s\n", task.Description)
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
