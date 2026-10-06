package org

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

// triggerListResponse mirrors the /orgs/{org}/triggers response. It is
// declared here rather than imported from the server package so the CLI
// depends on the wire contract, not on the handler's internals.
type triggerListResponse struct {
	Triggers []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		Kind        string `json:"kind"`
	} `json:"triggers"`
}

func newTriggersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "triggers",
		Short:   "List helix-org triggers",
		Aliases: []string{"trigger"},
	}
	cmd.AddCommand(newTriggersListCmd())
	return cmd
}

func newTriggersListCmd() *cobra.Command {
	var orgFlag string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List triggers in an organization",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			orgID, err := c.resolveOrg(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			var resp triggerListResponse
			if err := c.doJSON(cmd.Context(), http.MethodGet, "/orgs/"+orgID+"/triggers", nil, &resp, 30*time.Second); err != nil {
				return err
			}
			if jsonOut {
				return printJSON(resp)
			}
			fmt.Printf("%-40s %-16s %s\n", "ID", "KIND", "NAME")
			for _, t := range resp.Triggers {
				fmt.Printf("%-40s %-16s %s\n", t.ID, t.Kind, t.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}
