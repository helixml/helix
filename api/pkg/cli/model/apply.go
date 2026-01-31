package model

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	rootCmd.AddCommand(applyCmd)

	applyCmd.Flags().StringP("filename", "f", "", "Filename to apply (JSON or YAML)")
	_ = applyCmd.MarkFlagRequired("filename")
}

// applyCmd represents the apply command
var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply a Helix model configuration",
	Long: `Apply a Helix model configuration from a JSON or YAML file

apiVersion: model.aispec.org/v1alpha1
kind: Model
metadata:
  name: llama3.1:8b
spec:
  id: llama3.1:8b
  name: Llama 3.1 8B
  type: chat
  runtime: ollama
  memory: "8GB"  # Human-readable format: 8GB, 16GiB, 80G, etc.
  context_length: 8192
  description: Meta's Llama 3.1 8B model
  enabled: true

AUTOMATIC GPU CONFIGURATION:
Helix automatically handles all GPU-related configuration:

• GPU Selection: Automatically selects optimal GPU(s) based on model memory requirements
• Tensor Parallelism: Automatically sets --tensor-parallel-size for VLLM multi-GPU models
• Memory Ratios: Automatically calculates --gpu-memory-utilization for VLLM (per-GPU basis)
• CUDA Devices: Automatically sets CUDA_VISIBLE_DEVICES environment variable
• Multi-GPU: Models > single GPU memory automatically use multiple GPUs
• Ollama: Single GPU only - evicts models to make room if needed
• VLLM: Configures tensor parallel size based on selected GPU count

You only need to specify the model's total memory requirement - all GPU allocation,
tensor parallelism, and memory ratios are calculated and configured automatically.

Examples:
  # Apply model from YAML file (recommended)
  helix model apply -f model.yaml

  # Apply model from JSON file
  helix model apply -f model.json`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		filename, err := cmd.Flags().GetString("filename")
		if err != nil {
			return err
		}

		// Parse the model file
		model, err := parseModelConfigFile(filename)
		if err != nil {
			return err
		}

		// Validate required fields
		if model.ID == "" {
			return fmt.Errorf("model ID is required")
		}
		if model.Name == "" {
			return fmt.Errorf("model name is required")
		}

		// Create API client
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		// Check if model already exists
		existingModel, err := findExistingModel(cmd.Context(), apiClient, model.ID)
		if err != nil {
			return err
		}

		if existingModel != nil {
			// Update existing model
			err = updateModel(cmd.Context(), apiClient, model)
			if err != nil {
				return err
			}
			fmt.Printf("Model updated: %s\n", model.ID)
		} else {
			// Create new model
			err = createModel(cmd.Context(), apiClient, model)
			if err != nil {
				return err
			}
			fmt.Printf("Model created: %s\n", model.ID)
		}

		return nil
	},
}

// parseModelConfigFile parses a model configuration file in CRD format only
func parseModelConfigFile(filename string) (*types.Model, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	// Parse into a flexible structure first to handle memory field
	var rawCRD map[string]interface{}
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &rawCRD); err != nil {
			return nil, fmt.Errorf("failed to parse YAML CRD: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &rawCRD); err != nil {
			return nil, fmt.Errorf("failed to parse JSON CRD: %w", err)
		}
	default:
		// Try JSON first, then YAML
		if err := json.Unmarshal(data, &rawCRD); err != nil {
			if yamlErr := yaml.Unmarshal(data, &rawCRD); yamlErr != nil {
				return nil, fmt.Errorf("failed to parse CRD as JSON (%w) or YAML (%w)", err, yamlErr)
			}
		}
	}

	// Extract and validate basic CRD structure
	apiVersion, _ := rawCRD["apiVersion"].(string)
	kind, _ := rawCRD["kind"].(string)
	metadataRaw := rawCRD["metadata"]
	var metadata map[string]interface{}
	if metadataRaw != nil {
		// Handle both map[string]interface{} and map[interface{}]interface{} from YAML
		if m, ok := metadataRaw.(map[string]interface{}); ok {
			metadata = m
		} else if m, ok := metadataRaw.(map[interface{}]interface{}); ok {
			metadata = make(map[string]interface{})
			for k, v := range m {
				if key, ok := k.(string); ok {
					metadata[key] = v
				}
			}
		}
	}
	specRaw := rawCRD["spec"]
	var spec map[string]interface{}
	if specRaw != nil {
		// Handle both map[string]interface{} and map[interface{}]interface{} from YAML
		if m, ok := specRaw.(map[string]interface{}); ok {
			spec = m
		} else if m, ok := specRaw.(map[interface{}]interface{}); ok {
			spec = make(map[string]interface{})
			for k, v := range m {
				if key, ok := k.(string); ok {
					spec[key] = v
				}
			}
		}
	}

	// Recursively convert all map[interface{}]interface{} to map[string]interface{}
	// This is needed because YAML unmarshaling creates nested maps as map[interface{}]interface{}
	// but JSON marshaling requires map[string]interface{}
	spec = convertYAMLMapToStringMap(spec)

	if apiVersion == "" {
		return nil, fmt.Errorf("apiVersion is required")
	}
	if kind == "" {
		return nil, fmt.Errorf("kind is required")
	}

	// Support both "Model", "Agent", and "AIApp" as synonyms
	validKinds := []string{"Model", "Agent", "AIApp"}
	isValidKind := false
	for _, validKind := range validKinds {
		if kind == validKind {
			isValidKind = true
			break
		}
	}
	if !isValidKind {
		return nil, fmt.Errorf("expected kind 'Model', 'Agent', or 'AIApp', got '%s'", kind)
	}

	if metadata == nil {
		return nil, fmt.Errorf("metadata is required")
	}
	metadataName, _ := metadata["name"].(string)
	if metadataName == "" {
		return nil, fmt.Errorf("metadata.name is required")
	}

	if spec == nil {
		return nil, fmt.Errorf("spec is required")
	}

	// Handle memory field specially
	if memoryRaw, exists := spec["memory"]; exists {
		memoryBytes, err := parseMemoryField(memoryRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid memory format: %w", err)
		}
		spec["memory"] = memoryBytes
	}

	// Marshal spec back to JSON and unmarshal into Model struct
	specBytes, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal spec: %w", err)
	}

	var model types.Model
	if err := json.Unmarshal(specBytes, &model); err != nil {
		return nil, fmt.Errorf("failed to unmarshal model: %w", err)
	}

	// Use metadata name if spec ID is empty
	if model.ID == "" {
		model.ID = metadataName
	}

	// Use metadata name if spec name is empty
	if model.Name == "" {
		model.Name = metadataName
	}

	return &model, nil
}

// parseMemoryField handles both uint64 and string memory formats
func parseMemoryField(memory interface{}) (uint64, error) {
	switch v := memory.(type) {
	case uint64:
		return v, nil
	case int64:
		return uint64(v), nil
	case int:
		return uint64(v), nil
	case float64:
		return uint64(v), nil
	case string:
		return parseMemoryString(v)
	case nil:
		return 0, fmt.Errorf("memory field is required")
	default:
		return 0, fmt.Errorf("unsupported memory type: %T", memory)
	}
}

// parseMemoryString converts human-readable memory strings to bytes
func parseMemoryString(memStr string) (uint64, error) {
	memStr = strings.TrimSpace(memStr)
	if memStr == "" {
		return 0, fmt.Errorf("empty memory string")
	}

	// Handle different suffixes
	var multiplier uint64 = 1
	var numStr string

	// Convert to uppercase for case-insensitive comparison
	upper := strings.ToUpper(memStr)

	switch {
	case strings.HasSuffix(upper, "GIB") || strings.HasSuffix(upper, "G"):
		multiplier = 1024 * 1024 * 1024 // GiB (binary)
		if strings.HasSuffix(upper, "GIB") {
			numStr = strings.TrimSuffix(memStr, memStr[len(memStr)-3:])
		} else {
			numStr = strings.TrimSuffix(memStr, memStr[len(memStr)-1:])
		}
	case strings.HasSuffix(upper, "GB"):
		multiplier = 1000 * 1000 * 1000 // GB (decimal)
		numStr = strings.TrimSuffix(memStr, memStr[len(memStr)-2:])
	case strings.HasSuffix(upper, "MIB") || strings.HasSuffix(upper, "M"):
		multiplier = 1024 * 1024 // MiB (binary)
		if strings.HasSuffix(upper, "MIB") {
			numStr = strings.TrimSuffix(memStr, memStr[len(memStr)-3:])
		} else {
			numStr = strings.TrimSuffix(memStr, memStr[len(memStr)-1:])
		}
	case strings.HasSuffix(upper, "MB"):
		multiplier = 1000 * 1000 // MB (decimal)
		numStr = strings.TrimSuffix(memStr, memStr[len(memStr)-2:])
	case strings.HasSuffix(upper, "KIB") || strings.HasSuffix(upper, "K"):
		multiplier = 1024 // KiB (binary)
		if strings.HasSuffix(upper, "KIB") {
			numStr = strings.TrimSuffix(memStr, memStr[len(memStr)-3:])
		} else {
			numStr = strings.TrimSuffix(memStr, memStr[len(memStr)-1:])
		}
	case strings.HasSuffix(upper, "KB"):
		multiplier = 1000 // KB (decimal)
		numStr = strings.TrimSuffix(memStr, memStr[len(memStr)-2:])
	case strings.HasSuffix(upper, "B"):
		multiplier = 1 // Bytes
		numStr = strings.TrimSuffix(memStr, memStr[len(memStr)-1:])
	default:
		// Assume bytes if no suffix
		numStr = memStr
	}

	num, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", numStr)
	}

	if num < 0 {
		return 0, fmt.Errorf("memory cannot be negative")
	}

	return uint64(num * float64(multiplier)), nil
}

// findExistingModel finds an existing model by ID
func findExistingModel(ctx context.Context, apiClient client.Client, modelID string) (*types.Model, error) {
	query := &store.ListModelsQuery{}
	allModels, err := apiClient.ListHelixModels(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	for _, model := range allModels {
		if model.ID == modelID {
			return model, nil
		}
	}

	return nil, nil
}

// createModel creates a new model
func createModel(ctx context.Context, apiClient client.Client, model *types.Model) error {
	_, err := apiClient.CreateHelixModel(ctx, model)
	if err != nil {
		return fmt.Errorf("failed to create model: %w", err)
	}
	return nil
}

// updateModel updates an existing model
func updateModel(ctx context.Context, apiClient client.Client, model *types.Model) error {
	_, err := apiClient.UpdateHelixModel(ctx, model.ID, model)
	if err != nil {
		return fmt.Errorf("failed to update model: %w", err)
	}
	return nil
}

// convertYAMLMapToStringMap recursively converts map[interface{}]interface{} to map[string]interface{}
func convertYAMLMapToStringMap(input map[string]interface{}) map[string]interface{} {
	output := make(map[string]interface{})
	for key, value := range input {
		output[key] = convertYAMLValue(value)
	}
	return output
}

// convertYAMLValue recursively converts YAML values to JSON-compatible types
func convertYAMLValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[interface{}]interface{}:
		result := make(map[string]interface{})
		for key, val := range v {
			if strKey, ok := key.(string); ok {
				result[strKey] = convertYAMLValue(val)
			}
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, val := range v {
			result[key] = convertYAMLValue(val)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = convertYAMLValue(val)
		}
		return result
	default:
		return v
	}
}
