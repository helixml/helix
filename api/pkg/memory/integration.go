package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

// ModelMemoryEstimator provides high-level interface for model memory estimation
type ModelMemoryEstimator struct {
	modelsPath string
}

// NewModelMemoryEstimator creates a new model memory estimator
func NewModelMemoryEstimator(modelsPath string) *ModelMemoryEstimator {
	return &ModelMemoryEstimator{
		modelsPath: modelsPath,
	}
}

// EstimateMemoryForModel estimates memory requirements for a given model by name
func (e *ModelMemoryEstimator) EstimateMemoryForModel(modelName string, gpuInfos []GPUInfo, opts EstimateOptions) (*EstimationResult, error) {
	// Find model file
	modelPath, err := e.findModelPath(modelName)
	if err != nil {
		return nil, err
	}

	// Load metadata from GGUF file
	metadata, err := LoadModelMetadata(modelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load model metadata: %w", err)
	}

	// Perform estimation
	return EstimateModelMemory(metadata, gpuInfos, opts), nil
}

// EstimateMemoryForModelPath estimates memory requirements for a model at a specific path
func (e *ModelMemoryEstimator) EstimateMemoryForModelPath(modelPath string, gpuInfos []GPUInfo, opts EstimateOptions) (*EstimationResult, error) {
	// Load metadata from GGUF file
	metadata, err := LoadModelMetadata(modelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load model metadata: %w", err)
	}

	// Perform estimation
	return EstimateModelMemory(metadata, gpuInfos, opts), nil
}

// GetDefaultEstimateOptions returns reasonable default options for memory estimation
func GetDefaultEstimateOptions() EstimateOptions {
	return EstimateOptions{
		NumCtx:      4096,
		NumBatch:    512,
		NumParallel: 1,
		NumGPU:      -1, // Use all available GPUs by default
		KVCacheType: "q8_0",
	}
}

// findModelPath finds the full path to a model file
func (e *ModelMemoryEstimator) findModelPath(modelName string) (string, error) {
	// Common GGUF file extensions
	extensions := []string{".gguf", ".ggml", ".bin"}

	// Search patterns
	patterns := []string{
		modelName,                     // Exact name
		fmt.Sprintf("%s*", modelName), // Prefix match
	}

	for _, pattern := range patterns {
		for _, ext := range extensions {
			// Try with extension
			testPath := filepath.Join(e.modelsPath, pattern+ext)
			if matches, err := filepath.Glob(testPath); err == nil && len(matches) > 0 {
				// Return first match
				return matches[0], nil
			}

			// Try pattern as-is (might already have extension)
			testPath = filepath.Join(e.modelsPath, pattern)
			if matches, err := filepath.Glob(testPath); err == nil && len(matches) > 0 {
				// Check if it's a file and has a valid extension
				for _, match := range matches {
					if info, err := os.Stat(match); err == nil && !info.IsDir() {
						for _, validExt := range extensions {
							if filepath.Ext(match) == validExt {
								return match, nil
							}
						}
					}
				}
			}
		}
	}

	return "", fmt.Errorf("model file not found for model: %s", modelName)
}

// ValidateEstimateOptions validates estimation options
func ValidateEstimateOptions(opts EstimateOptions) error {
	if opts.NumCtx < 1 || opts.NumCtx > 1000000 {
		return fmt.Errorf("invalid context size: must be between 1 and 1,000,000, got %d", opts.NumCtx)
	}

	if opts.NumBatch < 1 || opts.NumBatch > 10000 {
		return fmt.Errorf("invalid batch size: must be between 1 and 10,000, got %d", opts.NumBatch)
	}

	if opts.NumParallel < 1 || opts.NumParallel > 100 {
		return fmt.Errorf("invalid parallel count: must be between 1 and 100, got %d", opts.NumParallel)
	}

	if opts.KVCacheType != "" && opts.KVCacheType != "f16" && opts.KVCacheType != "q8_0" && opts.KVCacheType != "q4_0" {
		return fmt.Errorf("invalid KV cache type: must be one of f16, q8_0, q4_0, got %s", opts.KVCacheType)
	}

	return nil
}

// CreateTestEstimateOptions creates EstimateOptions for testing with reasonable defaults
// contextLength: test context length (common values: 4096, 131072)
func CreateTestEstimateOptions(contextLength int) EstimateOptions {
	return EstimateOptions{
		NumCtx:      contextLength,
		NumBatch:    512,
		NumParallel: 1,
		NumGPU:      -1, // Auto-detect all layers that fit
		KVCacheType: "q8_0",
	}
}

// FormatMemorySize formats bytes as human readable string
func FormatMemorySize(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
