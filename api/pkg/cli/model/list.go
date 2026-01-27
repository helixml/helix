package model

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

var (
	listRuntime    string
	listType       string
	listEnabledStr string
	listAll        bool
	listQuiet      bool
	listNoTrunc    bool
)

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().StringVar(&listRuntime, "runtime", "", "Filter by runtime (ollama, vllm, diffusers)")
	listCmd.Flags().StringVar(&listType, "type", "", "Filter by type (chat, image, embed)")
	listCmd.Flags().StringVar(&listEnabledStr, "enabled", "", "Filter by enabled status (true/false)")
	listCmd.Flags().BoolVarP(&listAll, "all", "a", false, "Show all models including hidden ones")
	listCmd.Flags().BoolVarP(&listQuiet, "quiet", "q", false, "Only show model IDs")
	listCmd.Flags().BoolVar(&listNoTrunc, "no-trunc", false, "Don't truncate output")
}

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List helix models",
	Long: `List all available Helix models with filtering options.

Examples:
  # List all models
  helix model list

  # List only VLLM models
  helix model list --runtime vllm

  # List only chat models
  helix model list --type chat

  # List only enabled models
  helix model list --enabled true

  # Show all models including hidden ones
  helix model list --all

  # Show only model IDs (quiet mode)
  helix model list --quiet`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		query := &store.ListModelsQuery{}

		// Apply filters
		if listRuntime != "" {
			query.Runtime = types.Runtime(listRuntime)
		}
		if listType != "" {
			query.Type = types.ModelType(listType)
		}
		if listEnabledStr != "" {
			if listEnabledStr == "true" {
				enabled := true
				query.Enabled = &enabled
			} else if listEnabledStr == "false" {
				enabled := false
				query.Enabled = &enabled
			}
		}
		// Note: Hide filtering will be handled client-side since the query doesn't support it

		allModels, err := apiClient.ListHelixModels(cmd.Context(), query)
		if err != nil {
			return fmt.Errorf("failed to list models: %w", err)
		}

		// Filter hidden models client-side if not showing all
		var models []*types.Model
		for _, model := range allModels {
			if !listAll && model.Hide {
				continue // Skip hidden models unless --all is specified
			}
			models = append(models, model)
		}

		if listQuiet {
			for _, model := range models {
				fmt.Fprintln(cmd.OutOrStdout(), model.ID)
			}
			return nil
		}

		if len(models) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No models found")
			return nil
		}

		table := cli.NewSimpleTable(cmd.OutOrStdout(), []string{"ID", "NAME", "TYPE", "RUNTIME", "MEMORY", "CONTEXT", "ENABLED", "CREATED"})

		for _, model := range models {
			id := model.ID
			name := model.Name
			modelType := string(model.Type)
			runtime := string(model.Runtime)
			memory := formatMemory(model.Memory)
			context := formatContext(model.ContextLength)
			enabled := formatEnabled(model.Enabled)
			created := model.Created.Format("2006-01-02 15:04")

			// Truncate long fields unless --no-trunc is specified
			if !listNoTrunc {
				id = truncateString(id, 20)
				name = truncateString(name, 25)
			}

			row := []string{
				id,
				name,
				modelType,
				runtime,
				memory,
				context,
				enabled,
				created,
			}

			cli.AppendRow(table, row)
		}

		cli.RenderTable(table)

		return nil
	},
}

// Helper functions
func formatMemory(bytes uint64) string {
	if bytes == 0 {
		return "-"
	}

	gb := float64(bytes) / (1024 * 1024 * 1024)
	if gb < 1 {
		mb := float64(bytes) / (1024 * 1024)
		return fmt.Sprintf("%.0fMB", mb)
	}
	return fmt.Sprintf("%.1fGB", gb)
}

func formatContext(length int64) string {
	if length == 0 {
		return "-"
	}
	if length >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(length)/1000000)
	}
	if length >= 1000 {
		return fmt.Sprintf("%.0fK", float64(length)/1000)
	}
	return strconv.FormatInt(length, 10)
}

func formatEnabled(enabled bool) string {
	if enabled {
		return "✓"
	}
	return "✗"
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
