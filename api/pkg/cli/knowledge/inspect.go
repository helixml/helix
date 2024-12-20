package knowledge

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
	Use:   "inspect [knowledge ID]",
	Short: "Inspect a knowledge entry",
	Long:  ``,
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		knowledge, err := apiClient.GetKnowledge(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("failed to get knowledge: %w", err)
		}

		jsonBytes, err := json.MarshalIndent(knowledge, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal knowledge to JSON: %w", err)
		}

		fmt.Println(string(jsonBytes))

		return nil
	},
}
