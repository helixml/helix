package app

import (
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(inspectCmd)
}

var inspectCmd = &cobra.Command{
	Use:   "inspect [app ID]",
	Short: "Inspect an app entry",
	Long:  `Retrieve and display detailed information about a specific app in JSON format.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		app, err := lookupApp(apiClient, args[0])
		if err != nil {
			return fmt.Errorf("failed to lookup app: %w", err)
		}

		jsonBytes, err := json.MarshalIndent(app, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal app to JSON: %w", err)
		}

		fmt.Println(string(jsonBytes))

		return nil
	},
}
