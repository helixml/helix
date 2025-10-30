package moonlight

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

var listPendingCmd = &cobra.Command{
	Use:     "list-pending",
	Aliases: []string{"ls", "pending"},
	Short:   "List pending Moonlight pairing requests",
	Long:    `List all pending Moonlight client pairing requests waiting for PIN entry`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		// Check for JSON output flag
		jsonOutput, _ := cmd.Flags().GetBool("json")

		resp, err := apiClient.GetWolfPendingPairRequests(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to get pending pair requests: %w", err)
		}

		if jsonOutput {
			fmt.Println(string(resp))
			return nil
		}

		// Parse response
		var requests []map[string]interface{}
		if err := json.Unmarshal(resp, &requests); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		if len(requests) == 0 {
			fmt.Println("No pending pairing requests found.")
			fmt.Println("\nTo pair a Moonlight client:")
			fmt.Println("1. Open Moonlight on your client device")
			fmt.Println("2. Add a new PC with the server IP")
			fmt.Println("3. Wait for the pairing request to appear here")
			fmt.Println("4. Use 'helix moonlight pair <pair-secret> <pin>' to complete pairing")
			return nil
		}

		// Create table writer
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "PAIR_SECRET\tCLIENT_IP")

		for _, req := range requests {
			pairSecret := getStringField(req, "pair_secret")
			clientIP := getStringField(req, "client_ip")

			fmt.Fprintf(w, "%s\t%s\n", pairSecret, clientIP)
		}

		if err := w.Flush(); err != nil {
			return err
		}

		fmt.Printf("\nFound %d pending pairing request(s).\n", len(requests))
		fmt.Println("Use 'helix moonlight pair <pair-secret> <pin>' to complete pairing.")

		return nil
	},
}

func init() {
	listPendingCmd.Flags().BoolP("json", "j", false, "Output in JSON format")
}

