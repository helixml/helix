package org

import (
	"fmt"
	"os"
	"strings"

	"github.com/helixml/helix/api/pkg/client"
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

func newWebhooksListCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List webhook endpoints",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, orgID, err := orgClient(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			endpoints, err := c.ListWebhookEndpoints(cmd.Context(), orgID)
			if err != nil {
				return err
			}
			return printJSON(endpoints)
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
			c, orgID, err := orgClient(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			req := &client.WebhookEndpointRequest{URL: args[0], ProjectID: project, Description: description}
			if events != "" {
				req.Events = strings.Split(events, ",")
			}
			out, err := c.CreateWebhookEndpoint(cmd.Context(), orgID, req)
			if err != nil {
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
			c, orgID, err := orgClient(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			if err := c.DeleteWebhookEndpoint(cmd.Context(), orgID, args[0]); err != nil {
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
			c, orgID, err := orgClient(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			deliveries, err := c.ListWebhookDeliveries(cmd.Context(), orgID, args[0])
			if err != nil {
				return err
			}
			return printJSON(deliveries)
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
			c, orgID, err := orgClient(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			if _, err := c.ReplayWebhookDelivery(cmd.Context(), orgID, args[0], args[1]); err != nil {
				return err
			}
			fmt.Println("replayed", args[1])
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	return cmd
}
