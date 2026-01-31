package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

func newForkCommand() *cobra.Command {
	var projectName string
	var description string

	cmd := &cobra.Command{
		Use:   "fork <sample-project-id>",
		Short: "Create a new project from a sample project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sampleID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			payload := map[string]string{
				"sample_project_id": sampleID,
			}
			if projectName != "" {
				payload["project_name"] = projectName
			}
			if description != "" {
				payload["description"] = description
			}

			jsonData, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("failed to marshal request: %w", err)
			}

			url := apiURL + "/api/v1/sample-projects/simple/fork"
			req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			fmt.Printf("🔄 Forking sample project '%s'...\n", sampleID)

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("failed to fork project: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
			}

			var result ForkResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}

			fmt.Printf("\n✅ Project created successfully!\n\n")
			fmt.Printf("Project ID: %s\n", result.ProjectID)
			fmt.Printf("Tasks Created: %d\n", result.TasksCreated)
			fmt.Printf("Message: %s\n\n", result.Message)

			if result.GitHubRepoURL != "" {
				fmt.Printf("GitHub Repository: %s\n\n", result.GitHubRepoURL)
			}

			fmt.Printf("📋 Next steps:\n")
			fmt.Printf("   1. List tasks: helix project tasks %s\n", result.ProjectID)
			fmt.Printf("   2. Start planning a task: helix spectask start <task-id>\n")
			fmt.Printf("   3. Get screenshot: helix spectask screenshot <session-id>\n\n")

			return nil
		},
	}

	cmd.Flags().StringVarP(&projectName, "name", "n", "", "Project name (defaults to sample name)")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Project description")

	return cmd
}

type ForkResponse struct {
	ProjectID     string `json:"project_id"`
	GitHubRepoURL string `json:"github_repo_url"`
	TasksCreated  int    `json:"tasks_created"`
	Message       string `json:"message"`
}
