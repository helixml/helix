package project

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func newListSamplesCommand() *cobra.Command {
	var category string
	var difficulty string

	cmd := &cobra.Command{
		Use:   "samples",
		Short: "List available sample projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			apiURL := getAPIURL()
			token := getToken()

			url := apiURL + "/api/v1/sample-projects/simple"
			if category != "" || difficulty != "" {
				url += "?"
				if category != "" {
					url += "category=" + category
				}
				if difficulty != "" {
					if category != "" {
						url += "&"
					}
					url += "difficulty=" + difficulty
				}
			}

			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("failed to list sample projects: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("API returned status %d", resp.StatusCode)
			}

			var samples []SampleProject
			if err := json.NewDecoder(resp.Body).Decode(&samples); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}

			// Display samples
			fmt.Printf("\n📚 Available Sample Projects (%d)\n\n", len(samples))
			for _, sample := range samples {
				fmt.Printf("ID: %s\n", sample.ID)
				fmt.Printf("Name: %s\n", sample.Name)
				fmt.Printf("Description: %s\n", sample.Description)
				fmt.Printf("Category: %s | Difficulty: %s\n", sample.Category, sample.Difficulty)
				fmt.Printf("Technologies: %v\n", sample.Technologies)
				fmt.Printf("Tasks: %d sample tasks included\n", len(sample.TaskPrompts))
				if sample.GitHubRepo != "" {
					fmt.Printf("GitHub: %s\n", sample.GitHubRepo)
				}
				fmt.Printf("\n")
			}

			fmt.Printf("💡 To fork a sample project:\n")
			fmt.Printf("   helix project fork <sample-id> --name \"My Project\"\n\n")

			return nil
		},
	}

	cmd.Flags().StringVarP(&category, "category", "c", "", "Filter by category (web, mobile, api, etc.)")
	cmd.Flags().StringVarP(&difficulty, "difficulty", "d", "", "Filter by difficulty (beginner, intermediate, advanced)")

	return cmd
}

type SampleProject struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	GitHubRepo    string             `json:"github_repo"`
	DefaultBranch string             `json:"default_branch"`
	Technologies  []string           `json:"technologies"`
	TaskPrompts   []SampleTaskPrompt `json:"task_prompts"`
	ReadmeURL     string             `json:"readme_url"`
	DemoURL       string             `json:"demo_url,omitempty"`
	Difficulty    string             `json:"difficulty"`
	Category      string             `json:"category"`
}

type SampleTaskPrompt struct {
	Prompt      string   `json:"prompt"`
	Priority    string   `json:"priority"`
	Labels      []string `json:"labels"`
	Context     string   `json:"context"`
	Constraints string   `json:"constraints"`
}
