package org

import (
	"fmt"
	"net/http"
	"time"

	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/spf13/cobra"
)

func newTopicsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "topics",
		Short:   "List helix-org topics",
		Aliases: []string{"topic"},
	}
	cmd.AddCommand(newTopicsListCmd())
	return cmd
}

func newTopicsListCmd() *cobra.Command {
	var orgFlag string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List topics in an organization",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			orgID, err := c.resolveOrg(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			var resp orgapi.TopicsResponse
			if err := c.doJSON(cmd.Context(), http.MethodGet, "/orgs/"+orgID+"/topics", nil, &resp, 30*time.Second); err != nil {
				return err
			}
			if jsonOut {
				return printJSON(resp)
			}
			fmt.Printf("%-40s %-16s %s\n", "ID", "KIND", "NAME")
			for _, t := range resp.Topics {
				fmt.Printf("%-40s %-16s %s\n", t.ID, t.Kind, t.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}
