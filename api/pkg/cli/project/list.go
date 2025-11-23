package project

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List all projects",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			apiURL := getAPIURL()
			token := getToken()

			req, err := http.NewRequest("GET", apiURL+"/api/v1/projects", nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("failed to list projects: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("API returned status %d", resp.StatusCode)
			}

			var projects []Project
			if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}

			fmt.Printf("\n📁 Projects (%d)\n\n", len(projects))
			for _, proj := range projects {
				fmt.Printf("ID: %s\n", proj.ID)
				fmt.Printf("Name: %s\n", proj.Name)
				if proj.Description != "" {
					fmt.Printf("Description: %s\n", proj.Description)
				}
				if len(proj.Technologies) > 0 {
					fmt.Printf("Technologies: %v\n", proj.Technologies)
				}
				fmt.Printf("Status: %s\n", proj.Status)
				fmt.Printf("\n")
			}

			if len(projects) > 0 {
				fmt.Printf("💡 To view project tasks:\n")
				fmt.Printf("   helix project tasks <project-id>\n\n")
			}

			return nil
		},
	}
}

type Project struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Technologies  []string `json:"technologies"`
	Status        string   `json:"status"`
	StartupScript string   `json:"startup_script"`
}
