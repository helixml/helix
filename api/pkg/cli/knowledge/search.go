package knowledge

import (
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

func init() {
	searchCmd.Flags().String("app", "", "App ID to search within")
	searchCmd.Flags().String("knowledge", "", "Knowledge ID to search within")
	searchCmd.Flags().String("prompt", "", "Search prompt")

	searchCmd.MarkFlagRequired("app")
	searchCmd.MarkFlagRequired("prompt")

	rootCmd.AddCommand(searchCmd)
}

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search knowledge",
	Long:  `Search through knowledge using a prompt`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		appID, _ := cmd.Flags().GetString("app")
		knowledgeID, _ := cmd.Flags().GetString("knowledge")
		prompt, _ := cmd.Flags().GetString("prompt")

		results, err := apiClient.SearchKnowledge(cmd.Context(), &client.KnowledgeSearchQuery{
			AppID:       appID,
			KnowledgeID: knowledgeID,
			Prompt:      prompt,
		})
		if err != nil {
			return fmt.Errorf("failed to search knowledge: %w", err)
		}

		jsonBytes, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal results to JSON: %w", err)
		}

		fmt.Println(string(jsonBytes))

		return nil
	},
}
