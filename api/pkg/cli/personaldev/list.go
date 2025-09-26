package personaldev

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List personal development environments",
	Long:    `List all personal development environments for the current user`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		// Check for JSON output flag
		jsonOutput, _ := cmd.Flags().GetBool("json")

		resp, err := apiClient.ListPersonalDevEnvironments(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to list personal dev environments: %w", err)
		}


		if jsonOutput {
			fmt.Println(string(resp))
			return nil
		}

		// Parse response
		var environments []map[string]interface{}
		if err := json.Unmarshal(resp, &environments); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		// Create table writer
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSTATUS\tCREATED\tLAST_ACTIVITY")

		for _, env := range environments {
			id := getStringField(env, "InstanceID")
			name := getStringField(env, "environment_name")
			status := getStringField(env, "Status")
			created := getStringField(env, "CreatedAt")
			lastActivity := getStringField(env, "LastActivity")

			// Parse and format dates
			createdTime := parseTimeString(created)
			lastActivityTime := parseTimeString(lastActivity)

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				id, name, status,
				formatTime(createdTime),
				formatTime(lastActivityTime))
		}

		return w.Flush()
	},
}

func init() {
	listCmd.Flags().BoolP("json", "j", false, "Output in JSON format")
}

