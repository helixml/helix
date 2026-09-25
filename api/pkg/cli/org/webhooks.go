package org

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// `helix org webhooks`: the org's outbound webhook endpoints (Standard Webhooks),
// e.g. bot_instance.turn_completed for a gateway that forwards bot replies.
// Managing endpoints needs the org owner.
func newWebhooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "webhooks",
		Aliases: []string{"webhook", "hooks"},
		Short:   "Manage the org's outbound webhook endpoints (bot_instance.turn_completed, …)",
		Long: `Outbound webhooks for an organization. Payloads are signed with Standard Webhooks
headers (webhook-id, webhook-timestamp, webhook-signature); the signing secret is printed
once, on create.

Examples:
  helix org webhooks create https://portal.example.com/helix --events bot_instance.turn_completed
  helix org webhooks list
  helix org webhooks deliveries whe_01xxx
  helix org webhooks delete whe_01xxx`,
	}
	cmd.AddCommand(newWebhooksListCmd(), newWebhooksCreateCmd(), newWebhooksDeleteCmd(), newWebhooksDeliveriesCmd(), newWebhooksReplayCmd())
	return cmd
}

func webhooksBase(cmd *cobra.Command, orgFlag string) (*httpClient, string, error) {
	c, err := newHTTPClient()
	if err != nil {
		return nil, "", err
	}
	orgID, err := c.resolveOrg(cmd.Context(), orgFlag)
	if err != nil {
		return nil, "", err
	}
	return c, "/organizations/" + orgID + "/webhook-endpoints", nil
}

func newWebhooksListCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List webhook endpoints",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, base, err := webhooksBase(cmd, orgFlag)
			if err != nil {
				return err
			}
			var out any
			if err := c.doJSON(cmd.Context(), http.MethodGet, base, nil, &out, 30*time.Second); err != nil {
				return err
			}
			return printJSON(out)
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	return cmd
}

func newWebhooksCreateCmd() *cobra.Command {
	var orgFlag, events, project, description string
	cmd := &cobra.Command{
		Use:   "create <https-url>",
		Short: "Create an endpoint; prints its signing secret once",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, base, err := webhooksBase(cmd, orgFlag)
			if err != nil {
				return err
			}
			body := map[string]any{"url": args[0]}
			if events != "" {
				body["events"] = strings.Split(events, ",")
			}
			if project != "" {
				body["project_id"] = project
			}
			if description != "" {
				body["description"] = description
			}
			var out map[string]any
			if err := c.doJSON(cmd.Context(), http.MethodPost, base, body, &out, 30*time.Second); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "store the secret now — it is not shown again")
			return printJSON(out)
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().StringVar(&events, "events", "", "Comma list of events (default: all), e.g. bot_instance.turn_completed")
	cmd.Flags().StringVar(&project, "project", "", "Only events of this project (e.g. a bot's project)")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	return cmd
}

func newWebhooksDeleteCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "delete <endpoint-id>",
		Short: "Disable a webhook endpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, base, err := webhooksBase(cmd, orgFlag)
			if err != nil {
				return err
			}
			if err := c.doJSON(cmd.Context(), http.MethodDelete, base+"/"+args[0], nil, nil, 30*time.Second); err != nil {
				return err
			}
			fmt.Println("deleted", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	return cmd
}

func newWebhooksDeliveriesCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "deliveries <endpoint-id>",
		Short: "Show the last 50 deliveries (status, attempts, response)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, base, err := webhooksBase(cmd, orgFlag)
			if err != nil {
				return err
			}
			var out any
			if err := c.doJSON(cmd.Context(), http.MethodGet, base+"/"+args[0]+"/deliveries", nil, &out, 30*time.Second); err != nil {
				return err
			}
			return printJSON(out)
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	return cmd
}

func newWebhooksReplayCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "replay <endpoint-id> <delivery-id>",
		Short: "Re-send one delivery",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, base, err := webhooksBase(cmd, orgFlag)
			if err != nil {
				return err
			}
			if err := c.doJSON(cmd.Context(), http.MethodPost, base+"/"+args[0]+"/deliveries/"+args[1]+"/replay", nil, nil, 30*time.Second); err != nil {
				return err
			}
			fmt.Println("replayed", args[1])
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	return cmd
}
