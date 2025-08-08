package model

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

var (
	createName        string
	createDescription string
	createType        string
	createRuntime     string
	createMemory      string
	createContext     int64
	createEnabled     bool
	createHide        bool
	createAutoPull    bool
	createPrewarm     bool
	createRuntimeArgs string
	createFromFile    string
)

func init() {
	rootCmd.AddCommand(createCmd)

	createCmd.Flags().StringVar(&createName, "name", "", "Human-readable name for the model")
	createCmd.Flags().StringVar(&createDescription, "description", "", "Description of the model")
	createCmd.Flags().StringVar(&createType, "type", "chat", "Model type (chat, image, embed)")
	createCmd.Flags().StringVar(&createRuntime, "runtime", "ollama", "Runtime to use (ollama, vllm, diffusers)")
	createCmd.Flags().StringVar(&createMemory, "memory", "", "Memory requirement (e.g., 8GB, 16GB)")
	createCmd.Flags().Int64Var(&createContext, "context", 0, "Context length (tokens)")
	createCmd.Flags().BoolVar(&createEnabled, "enabled", true, "Enable the model")
	createCmd.Flags().BoolVar(&createHide, "hide", false, "Hide the model from default lists")
	createCmd.Flags().BoolVar(&createAutoPull, "auto-pull", false, "Automatically pull model if missing")
	createCmd.Flags().BoolVar(&createPrewarm, "prewarm", false, "Prewarm model to fill free GPU memory")
	createCmd.Flags().StringVar(&createRuntimeArgs, "runtime-args", "", "Runtime-specific arguments as JSON")
	createCmd.Flags().StringVarP(&createFromFile, "file", "f", "", "Create model from JSON file")

	createCmd.MarkFlagRequired("name")
}

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create MODEL_ID",
	Short: "Create a new helix model",
	Long: `Create a new Helix model configuration.

Examples:
  # Create a basic Ollama model
  helix model create llama3.1:8b --name "Llama 3.1 8B" --memory 8GB

  # Create a VLLM model with runtime arguments
  helix model create Qwen/Qwen2.5-VL-7B-Instruct \
    --name "Qwen 2.5 VL 7B" \
    --runtime vllm \
    --memory 39GB \
    --context 32768 \
    --runtime-args '["--trust-remote-code", "--max-model-len", "32768"]'

  # Create model from JSON file
  helix model create --file model.json

JSON file format examples:

Basic Ollama model:
{
  "id": "llama3.1:8b",
  "name": "Llama 3.1 8B",
  "type": "chat",
  "runtime": "ollama",
  "memory": 8589934592,
  "context_length": 8192,
  "description": "Meta's Llama 3.1 8B model",
  "enabled": true
}

VLLM model with comprehensive configuration:
{
  "id": "Qwen/Qwen2.5-VL-7B-Instruct",
  "name": "Qwen 2.5 VL 7B",
  "type": "chat",
  "runtime": "vllm",
  "memory": "39GB",
  "context_length": 32768,
  "description": "Multi-modal vision-language model from Alibaba",
  "enabled": true,
  "auto_pull": true,
  "prewarm": true,
  "runtime_args": [
    "--trust-remote-code",
    "--max-model-len", "32768",
    "--gpu-memory-utilization", "{{.DynamicMemoryUtilizationRatio}}",
    "--limit-mm-per-prompt", "{\"image\":10}",
    "--enforce-eager",
    "--max-num-seqs", "64"
  ]
}

VLLM embedding model:
{
  "id": "BAAI/bge-large-en-v1.5",
  "name": "BGE Large EN v1.5",
  "type": "embed",
  "runtime": "vllm",
  "memory": "5GB",
  "context_length": 512,
  "description": "High-quality embedding model for RAG applications",
  "enabled": true,
  "runtime_args": [
    "--task", "embed",
    "--max-model-len", "512",
    "--trust-remote-code"
  ]
}`,
	Args: func(cmd *cobra.Command, args []string) error {
		if createFromFile != "" {
			return nil // No args needed when using file
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

		var model types.Model

		if createFromFile != "" {
			// Load model from file
			data, err := os.ReadFile(createFromFile)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", createFromFile, err)
			}

			if err := json.Unmarshal(data, &model); err != nil {
				return fmt.Errorf("failed to parse JSON file: %w", err)
			}
		} else {
			// Build model from flags
			modelID := args[0]

			// Parse memory
			var memoryBytes uint64
			if createMemory != "" {
				memoryBytes, err = parseMemory(createMemory)
				if err != nil {
					return fmt.Errorf("invalid memory format: %w", err)
				}
			}

			// Parse runtime args
			var runtimeArgs map[string]interface{}
			if createRuntimeArgs != "" {
				if err := json.Unmarshal([]byte(createRuntimeArgs), &runtimeArgs); err != nil {
					return fmt.Errorf("invalid runtime-args JSON: %w", err)
				}
			}

			model = types.Model{
				ID:            modelID,
				Name:          createName,
				Description:   createDescription,
				Type:          types.ModelType(createType),
				Runtime:       types.Runtime(createRuntime),
				Memory:        memoryBytes,
				ContextLength: createContext,
				Enabled:       createEnabled,
				Hide:          createHide,
				AutoPull:      createAutoPull,
				Prewarm:       createPrewarm,
				RuntimeArgs:   runtimeArgs,
			}
		}

		// Validate required fields
		if model.ID == "" {
			return fmt.Errorf("model ID is required")
		}
		if model.Name == "" {
			return fmt.Errorf("model name is required")
		}

		createdModel, err := apiClient.CreateHelixModel(cmd.Context(), &model)
		if err != nil {
			return fmt.Errorf("failed to create model: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Model created successfully: %s\n", createdModel.ID)
		return nil
	},
}

// parseMemory converts human-readable memory strings to bytes
func parseMemory(memStr string) (uint64, error) {
	memStr = strings.ToUpper(strings.TrimSpace(memStr))

	var multiplier uint64 = 1
	var numStr string

	if strings.HasSuffix(memStr, "GIB") {
		multiplier = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(memStr, "GIB")
	} else if strings.HasSuffix(memStr, "MIB") {
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(memStr, "MIB")
	} else if strings.HasSuffix(memStr, "TIB") {
		multiplier = 1024 * 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(memStr, "TIB")
	} else if strings.HasSuffix(memStr, "KIB") {
		multiplier = 1024
		numStr = strings.TrimSuffix(memStr, "KIB")
	} else if strings.HasSuffix(memStr, "GB") {
		multiplier = 1000 * 1000 * 1000 // Decimal GB
		numStr = strings.TrimSuffix(memStr, "GB")
	} else if strings.HasSuffix(memStr, "MB") {
		multiplier = 1000 * 1000 // Decimal MB
		numStr = strings.TrimSuffix(memStr, "MB")
	} else if strings.HasSuffix(memStr, "TB") {
		multiplier = 1000 * 1000 * 1000 * 1000 // Decimal TB
		numStr = strings.TrimSuffix(memStr, "TB")
	} else if strings.HasSuffix(memStr, "KB") {
		multiplier = 1000 // Decimal KB
		numStr = strings.TrimSuffix(memStr, "KB")
	} else if strings.HasSuffix(memStr, "G") {
		multiplier = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(memStr, "G")
	} else if strings.HasSuffix(memStr, "M") {
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(memStr, "M")
	} else {
		// Assume bytes if no suffix
		numStr = memStr
	}

	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", numStr)
	}

	return uint64(num * float64(multiplier)), nil
}
