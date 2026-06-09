package project

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
)

func newListCommand() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects in an organization",
		Long: `List projects in an organization.

Projects are organization-scoped. If --org is omitted and you belong to a
single org, that org is used; if you belong to multiple, you will be
prompted. The HELIX_ORG environment variable is also honoured.`,
		Aliases: []string{"ls"},
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
			endpoint := apiURL + "/api/v1/projects?" + q.Encode()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)

			httpClient := &http.Client{}
			resp, err := httpClient.Do(req)
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

	cmd.Flags().StringVarP(&orgFlag, "org", "o", "", "Organization name or ID (defaults to $HELIX_ORG, then your only org, or prompts)")
	return cmd
}

type Project struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Technologies  []string `json:"technologies"`
	Status        string   `json:"status"`
	StartupScript string   `json:"startup_script"`
}
