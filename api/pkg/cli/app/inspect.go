package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

func init() {
	rootCmd.AddCommand(inspectCmd)

	inspectCmd.Flags().String("output", "yaml", "Output format. One of: json|yaml")
}

var inspectCmd = &cobra.Command{
	Use:   "inspect [agent ID]",
	Short: "Inspect an agent entry",
	Long:  `Retrieve and display detailed information about a specific agent in JSON or YAML format.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		organization, err := cmd.Flags().GetString("organization")
		if err != nil {
			return err
		}

		app, err := lookupApp(cmd.Context(), apiClient, organization, args[0])
		if err != nil {
			return fmt.Errorf("failed to lookup agent: %w", err)
		}

		// only show app.Config.Helix since that is the thing that roundtrips with helix apply -f
		appConfig := app.Config.Helix

		// Remove tools section from all assistants
		for i := range appConfig.Assistants {
			appConfig.Assistants[i].Tools = nil
		}

		// Convert to CRD format for both JSON and YAML output
		crd := types.AppHelixConfigCRD{
			APIVersion: "app.aispec.org/v1alpha1",
			Kind:       "AIApp",
			Metadata: types.AppHelixConfigMetadata{
				Name: appConfig.Name,
			},
			Spec: appConfig,
		}

		// Clear the name from the spec since it's now in metadata
		crd.Spec.Name = ""

		outputFormat, _ := cmd.Flags().GetString("output")
		outputFormat = strings.ToLower(outputFormat)

		var output []byte
		switch outputFormat {
		case "json":
			output, err = json.MarshalIndent(crd, "", "  ")
		case "yaml":
			output, err = yaml.Marshal(crd)
		default:
			return fmt.Errorf("unsupported output format: %s", outputFormat)
		}

		if err != nil {
			return fmt.Errorf("failed to marshal agent to %s: %w", outputFormat, err)
		}

		fmt.Println(string(output))

		return nil
	},
}
