package knowledge

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(removeCmd)
}

var removeCmd = &cobra.Command{
	Use:     "remove",
	Aliases: []string{"rm"},
	Short:   "Delete helix knowledge",
	Long:    ``,
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("knowledge name or ID is required")
		}

		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		knowledges, err := apiClient.ListKnowledge(cmd.Context(), &client.KnowledgeFilter{})
		if err != nil {
			return fmt.Errorf("failed to list knowledge: %w", err)
		}

		// Find the knowledge by name or ID
		var knowledge *types.Knowledge
		for _, k := range knowledges {
			if k.Name == args[0] || k.ID == args[0] {
				knowledge = k
				break
			}
		}

		if knowledge == nil {
			return fmt.Errorf("knowledge %s not found", args[0])
		}

		// Delete the knowledge
		if err := apiClient.DeleteKnowledge(cmd.Context(), knowledge.ID); err != nil {
			return fmt.Errorf("failed to delete knowledge: %w", err)
		}

		fmt.Printf("Knowledge %s deleted\n", knowledge.ID)

		return nil
	},
}
