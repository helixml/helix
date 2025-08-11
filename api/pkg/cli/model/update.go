package model

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

var (
	updateName        string
	updateDescription string
	updateType        string
	updateRuntime     string
	updateMemory      string
	updateContext     int64
	updateEnabledStr  string
	updateHideStr     string
	updateAutoPullStr string
	updatePrewarmStr  string
	updateRuntimeArgs string
	updateFromFile    string
	updateClearArgs   bool
)

func init() {
	rootCmd.AddCommand(updateCmd)

	updateCmd.Flags().StringVar(&updateName, "name", "", "Update human-readable name for the model")
	updateCmd.Flags().StringVar(&updateDescription, "description", "", "Update description of the model")
	updateCmd.Flags().StringVar(&updateType, "type", "", "Update model type (chat, image, embed)")
	updateCmd.Flags().StringVar(&updateRuntime, "runtime", "", "Update runtime to use (ollama, vllm, diffusers)")
	updateCmd.Flags().StringVar(&updateMemory, "memory", "", "Update memory requirement (e.g., 8GB, 16GB)")
	updateCmd.Flags().Int64Var(&updateContext, "context", -1, "Update context length (tokens)")
	updateCmd.Flags().StringVar(&updateEnabledStr, "enabled", "", "Update enabled status (true/false)")
	updateCmd.Flags().StringVar(&updateHideStr, "hide", "", "Update hidden status (true/false)")
	updateCmd.Flags().StringVar(&updateAutoPullStr, "auto-pull", "", "Update auto-pull setting (true/false)")
	updateCmd.Flags().StringVar(&updatePrewarmStr, "prewarm", "", "Update prewarm setting (true/false)")
	updateCmd.Flags().StringVar(&updateRuntimeArgs, "runtime-args", "", "Update runtime-specific arguments as JSON")
	updateCmd.Flags().StringVarP(&updateFromFile, "file", "f", "", "Update model from JSON file")
	updateCmd.Flags().BoolVar(&updateClearArgs, "clear-runtime-args", false, "Clear all runtime arguments")
}

// updateCmd represents the update command
var updateCmd = &cobra.Command{
	Use:   "update MODEL_ID",
	Short: "Update an existing helix model",
	Long: `Update an existing Helix model configuration.

Examples:
  # Update model name and memory
  helix model update llama3.1:8b --name "Updated Llama 3.1 8B" --memory 16GB

  # Enable a model
  helix model update my-model --enabled true

  # Update VLLM runtime arguments
  helix model update Qwen/Qwen2.5-VL-7B-Instruct \
    --runtime-args '["--trust-remote-code", "--max-model-len", "65536"]'

  # Clear runtime arguments
  helix model update my-model --clear-runtime-args

  # Update model from JSON file
  helix model update my-model --file updated-model.json

Note: Only specified fields will be updated. Existing values remain unchanged unless explicitly set.`,
	Args: func(_ *cobra.Command, args []string) error {
		if updateFromFile != "" {
			if len(args) > 1 {
				return fmt.Errorf("cannot specify MODEL_ID when using --file")
			}
			return nil
		}
		if len(args) != 1 {
			return fmt.Errorf("requires exactly one argument (MODEL_ID) when not using --file")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		var modelID string
		var updates types.Model

		if updateFromFile != "" {
			// Load updates from file
			data, err := os.ReadFile(updateFromFile)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", updateFromFile, err)
			}

			if err := json.Unmarshal(data, &updates); err != nil {
				return fmt.Errorf("failed to parse JSON file: %w", err)
			}

			modelID = updates.ID
			if modelID == "" {
				return fmt.Errorf("model ID is required in JSON file")
			}
		} else {
			modelID = args[0]

			// Get existing model first
			query := &store.ListModelsQuery{}
			allModels, err := apiClient.ListHelixModels(cmd.Context(), query)
			if err != nil {
				return fmt.Errorf("failed to list models: %w", err)
			}

			var existingModel *types.Model
			for _, model := range allModels {
				if model.ID == modelID {
					existingModel = model
					break
				}
			}

			if existingModel == nil {
				return fmt.Errorf("model not found: %s", modelID)
			}

			// Start with existing model and apply updates
			updates = *existingModel

			// Apply flag updates
			if updateName != "" {
				updates.Name = updateName
			}
			if updateDescription != "" {
				updates.Description = updateDescription
			}
			if updateType != "" {
				updates.Type = types.ModelType(updateType)
			}
			if updateRuntime != "" {
				updates.Runtime = types.Runtime(updateRuntime)
			}
			if updateMemory != "" {
				memoryBytes, err := parseMemory(updateMemory)
				if err != nil {
					return fmt.Errorf("invalid memory format: %w", err)
				}
				updates.Memory = memoryBytes
			}
			if updateContext >= 0 {
				updates.ContextLength = updateContext
			}
			if updateEnabledStr != "" {
				if updateEnabledStr == "true" {
					updates.Enabled = true
				} else if updateEnabledStr == "false" {
					updates.Enabled = false
				}
			}
			if updateHideStr != "" {
				if updateHideStr == "true" {
					updates.Hide = true
				} else if updateHideStr == "false" {
					updates.Hide = false
				}
			}
			if updateAutoPullStr != "" {
				if updateAutoPullStr == "true" {
					updates.AutoPull = true
				} else if updateAutoPullStr == "false" {
					updates.AutoPull = false
				}
			}
			if updatePrewarmStr != "" {
				if updatePrewarmStr == "true" {
					updates.Prewarm = true
				} else if updatePrewarmStr == "false" {
					updates.Prewarm = false
				}
			}
			if updateClearArgs {
				updates.RuntimeArgs = nil
			} else if updateRuntimeArgs != "" {
				var runtimeArgs map[string]interface{}
				if err := json.Unmarshal([]byte(updateRuntimeArgs), &runtimeArgs); err != nil {
					return fmt.Errorf("invalid runtime-args JSON: %w", err)
				}
				updates.RuntimeArgs = runtimeArgs
			}
		}

		updatedModel, err := apiClient.UpdateHelixModel(cmd.Context(), modelID, &updates)
		if err != nil {
			return fmt.Errorf("failed to update model: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Model updated successfully: %s\n", updatedModel.ID)
		return nil
	},
}
