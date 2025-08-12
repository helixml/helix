package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

var (
	inspectFormat string
	inspectPretty bool
)

func init() {
	rootCmd.AddCommand(inspectCmd)

	inspectCmd.Flags().StringVarP(&inspectFormat, "format", "f", "table", "Output format (table, json, yaml)")
	inspectCmd.Flags().BoolVar(&inspectPretty, "pretty", true, "Pretty print JSON output")
}

// inspectCmd represents the inspect command
var inspectCmd = &cobra.Command{
	Use:   "inspect MODEL_ID [MODEL_ID...]",
	Short: "Display detailed information about one or more models",
	Long: `Display detailed information about one or more models.

Examples:
  # Inspect a single model
  helix model inspect llama3.1:8b

  # Inspect multiple models
  helix model inspect llama3.1:8b qwen2.5:7b

  # Output as JSON
  helix model inspect llama3.1:8b --format json

  # Output as compact JSON
  helix model inspect llama3.1:8b --format json --pretty=false`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		var models []*types.Model

		for _, modelID := range args {
			// Try to find the model by ID
			query := &store.ListModelsQuery{}
			allModels, err := apiClient.ListHelixModels(cmd.Context(), query)
			if err != nil {
				return fmt.Errorf("failed to list models: %w", err)
			}

			var found *types.Model
			for _, model := range allModels {
				if model.ID == modelID {
					found = model
					break
				}
			}

			if found == nil {
				return fmt.Errorf("model not found: %s", modelID)
			}

			models = append(models, found)
		}

		switch inspectFormat {
		case "json":
			return outputJSON(cmd, models)
		case "yaml":
			return outputYAML(cmd, models)
		default:
			return outputTable(cmd, models)
		}
	},
}

func outputJSON(cmd *cobra.Command, models []*types.Model) error {
	var output interface{}

	// Convert to CRD format for consistency with helix model apply
	if len(models) == 1 {
		output = modelToCRD(models[0])
	} else {
		var crds []types.ModelCRD
		for _, model := range models {
			crds = append(crds, modelToCRD(model))
		}
		output = crds
	}

	var data []byte
	var err error

	if inspectPretty {
		data, err = json.MarshalIndent(output, "", "  ")
	} else {
		data, err = json.Marshal(output)
	}

	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

func outputYAML(cmd *cobra.Command, models []*types.Model) error {
	var output interface{}

	// Convert to CRD format for consistency with helix model apply
	if len(models) == 1 {
		output = modelToCRD(models[0])
	} else {
		var crds []types.ModelCRD
		for _, model := range models {
			crds = append(crds, modelToCRD(model))
		}
		output = crds
	}

	data, err := yaml.Marshal(output)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

func outputTable(cmd *cobra.Command, models []*types.Model) error {
	for i, model := range models {
		if i > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "")
		}

		fmt.Fprintf(cmd.OutOrStdout(), "ID:              %s\n", model.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Name:            %s\n", model.Name)
		fmt.Fprintf(cmd.OutOrStdout(), "Type:            %s\n", model.Type)
		fmt.Fprintf(cmd.OutOrStdout(), "Runtime:         %s\n", model.Runtime)
		fmt.Fprintf(cmd.OutOrStdout(), "Memory:          %s\n", formatMemory(model.Memory))
		fmt.Fprintf(cmd.OutOrStdout(), "Context Length:  %s\n", formatContext(model.ContextLength))
		fmt.Fprintf(cmd.OutOrStdout(), "Enabled:         %t\n", model.Enabled)
		fmt.Fprintf(cmd.OutOrStdout(), "Hidden:          %t\n", model.Hide)
		fmt.Fprintf(cmd.OutOrStdout(), "Auto Pull:       %t\n", model.AutoPull)
		fmt.Fprintf(cmd.OutOrStdout(), "Prewarm:         %t\n", model.Prewarm)
		fmt.Fprintf(cmd.OutOrStdout(), "User Modified:   %t\n", model.UserModified)
		fmt.Fprintf(cmd.OutOrStdout(), "Sort Order:      %d\n", model.SortOrder)
		fmt.Fprintf(cmd.OutOrStdout(), "Created:         %s\n", model.Created.Format(time.RFC3339))
		fmt.Fprintf(cmd.OutOrStdout(), "Updated:         %s\n", model.Updated.Format(time.RFC3339))

		if model.Description != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Description:     %s\n", model.Description)
		}

		if len(model.RuntimeArgs) > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "Runtime Args:")
			runtimeArgsJSON, err := json.MarshalIndent(model.RuntimeArgs, "  ", "  ")
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  (error formatting runtime args: %v)\n", err)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", string(runtimeArgsJSON))
			}
		}
	}

	return nil
}

// modelToCRD converts a Model to ModelCRD format for consistent output
func modelToCRD(model *types.Model) types.ModelCRD {
	return types.ModelCRD{
		APIVersion: "model.aispec.org/v1alpha1",
		Kind:       "Model",
		Metadata: types.ModelMetadata{
			Name: model.ID,
		},
		Spec: *model,
	}
}
