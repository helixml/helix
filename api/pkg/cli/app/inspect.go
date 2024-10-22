package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

func init() {
	rootCmd.AddCommand(inspectCmd)
	inspectCmd.Flags().StringP("output", "o", "yaml", "Output format. One of: json|yaml")
}

var inspectCmd = &cobra.Command{
	Use:   "inspect [app ID]",
	Short: "Inspect an app entry",
	Long:  `Retrieve and display detailed information about a specific app in JSON or YAML format.`,
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

		// only show app.Config.Helix since that is the thing that roundtrips with helix apply -f
		appConfig := app.Config.Helix

		outputFormat, _ := cmd.Flags().GetString("output")
		outputFormat = strings.ToLower(outputFormat)

		var output []byte
		switch outputFormat {
		case "json":
			output, err = json.MarshalIndent(appConfig, "", "  ")
		case "yaml":
			output, err = yaml.Marshal(appConfig)
		default:
			return fmt.Errorf("unsupported output format: %s", outputFormat)
		}

		if err != nil {
			return fmt.Errorf("failed to marshal app to %s: %w", outputFormat, err)
		}

		fmt.Println(string(output))

		return nil
	},
}
