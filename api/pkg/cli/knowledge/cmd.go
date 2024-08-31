package knowledge

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "knowledge",
	Short:   "Helix knowledge management",
	Aliases: []string{"k"},
	Long:    `TODO`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
	},
}

func New() *cobra.Command {
	return rootCmd
}

func lookupKnowledge(apiClient *client.HelixClient, ref string) (*types.Knowledge, error) {
	knowledge, err := apiClient.ListKnowledge(&client.KnowledgeFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", err)
	}

	for _, knowledge := range knowledge {
		if knowledge.Name == ref || knowledge.ID == ref {
			return knowledge, nil
		}
	}

	return nil, fmt.Errorf("knowledge not found: %s", ref)
}
