package model

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/store"
)

var (
	deleteForce bool
)

func init() {
	rootCmd.AddCommand(deleteCmd)

	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "Force delete without confirmation")
}

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:     "delete MODEL_ID [MODEL_ID...]",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete one or more helix models",
	Long: `Delete one or more Helix model configurations.

Examples:
  # Delete a single model (with confirmation)
  helix model delete my-model

  # Delete multiple models
  helix model delete model1 model2 model3

  # Force delete without confirmation
  helix model delete my-model --force

Warning: This action is irreversible. The model configuration will be permanently deleted.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		// First, verify all models exist
		query := &store.ListModelsQuery{}
		allModels, err := apiClient.ListHelixModels(cmd.Context(), query)
		if err != nil {
			return fmt.Errorf("failed to list models: %w", err)
		}

		var modelsToDelete []string
		for _, modelID := range args {
			found := false
			for _, model := range allModels {
				if model.ID == modelID {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("model not found: %s", modelID)
			}
			modelsToDelete = append(modelsToDelete, modelID)
		}

		// Confirmation prompt unless --force is used
		if !deleteForce {
			fmt.Fprintf(cmd.OutOrStdout(), "Are you sure you want to delete the following model(s)?\n")
			for _, modelID := range modelsToDelete {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", modelID)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nThis action is irreversible. Continue? (y/N): ")

			reader := bufio.NewReader(os.Stdin)
			response, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read confirmation: %w", err)
			}

			response = strings.ToLower(strings.TrimSpace(response))
			if response != "y" && response != "yes" {
				fmt.Fprintln(cmd.OutOrStdout(), "Operation cancelled.")
				return nil
			}
		}

		// Delete models
		var deletedCount int
		var errors []string

		for _, modelID := range modelsToDelete {
			err := apiClient.DeleteHelixModel(cmd.Context(), modelID)
			if err != nil {
				errors = append(errors, fmt.Sprintf("failed to delete %s: %v", modelID, err))
				continue
			}
			deletedCount++
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted model: %s\n", modelID)
		}

		// Report results
		if len(errors) > 0 {
			fmt.Fprintf(cmd.OutOrStderr(), "\nErrors occurred during deletion:\n")
			for _, errMsg := range errors {
				fmt.Fprintf(cmd.OutOrStderr(), "  - %s\n", errMsg)
			}
		}

		if deletedCount > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "\nSuccessfully deleted %d model(s)\n", deletedCount)
		}

		if len(errors) > 0 {
			return fmt.Errorf("some deletions failed")
		}

		return nil
	},
}
