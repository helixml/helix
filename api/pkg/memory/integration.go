package memory

import (
	"fmt"
	"path/filepath"
	"runtime"
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

// EstimateMemoryForModel estimates memory requirements for a given model
func (e *ModelMemoryEstimator) EstimateMemoryForModel(modelName string, gpuInfos []GPUInfo, opts EstimateOptions) (*EstimationResult, error) {
	// Find model file
	modelPath, err := e.findModelPath(modelName)
	if err != nil {
		return nil, err
	}

	// Validate options
	if err := ValidateEstimateOptions(opts); err != nil {
		return nil, err
	}

	// Perform estimation
	return EstimateModelMemory(modelPath, gpuInfos, opts)
}

// EstimateMemoryForModelPath estimates memory requirements for a model at a specific path
func (e *ModelMemoryEstimator) EstimateMemoryForModelPath(modelPath string, gpuInfos []GPUInfo, opts EstimateOptions) (*EstimationResult, error) {
	// Validate options
	if err := ValidateEstimateOptions(opts); err != nil {
		return nil, err
	}

	// Perform estimation
	return EstimateModelMemory(modelPath, gpuInfos, opts)
}

// GetDefaultEstimateOptions returns reasonable default options for memory estimation
func GetDefaultEstimateOptions() EstimateOptions {
	return EstimateOptions{
		NumCtx:      4096,
		NumBatch:    512,
		NumParallel: 1,
		NumGPU:      -1, // Auto-detect
		KVCacheType: "f16",
	}
}

// detectGPULibrary detects the GPU library based on the system
func detectGPULibrary() string {
	switch runtime.GOOS {
	case "linux":
		// Could check for CUDA/ROCm, but default to CUDA for now
		return "cuda"
	case "darwin":
		return "metal"
	case "windows":
		return "cuda"
	default:
		return "cpu"
	}
}

// findModelPath finds the model file path for a given model name
func (e *ModelMemoryEstimator) findModelPath(modelName string) (string, error) {
	// This is a simplified implementation - in practice, you'd want to:
	// 1. Check Ollama's manifest system
	// 2. Look in blob storage
	// 3. Handle model name resolution

	// For now, try common patterns
	possiblePaths := []string{
		filepath.Join(e.modelsPath, modelName),
		filepath.Join(e.modelsPath, "blobs", modelName),
		filepath.Join(e.modelsPath, "models", modelName),
	}

	// Try with common extensions
	extensions := []string{"", ".gguf", ".bin"}

	for _, basePath := range possiblePaths {
		for _, ext := range extensions {
			fullPath := basePath + ext
			if fileExists(fullPath) {
				return fullPath, nil
			}
		}
	}

	return "", &EstimationError{
		Type:    "model_not_found",
		Message: "model file not found",
		Details: fmt.Sprintf("searched for model '%s' in %s", modelName, e.modelsPath),
	}
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := filepath.Abs(path)
	return err == nil
}

// GetRecommendedGPUConfiguration returns the recommended GPU configuration for a model
func GetRecommendedGPUConfiguration(result *EstimationResult) *GPUConfiguration {
	if result == nil {
		return nil
	}

	config := &GPUConfiguration{
		ModelName:      result.ModelName,
		Recommendation: result.Recommendation,
	}

	switch result.Recommendation {
	case "single_gpu":
		if result.SingleGPU != nil {
			config.GPUCount = 1
			config.MemoryPerGPU = result.SingleGPU.VRAMSize
			config.TotalMemory = result.SingleGPU.VRAMSize
			config.LayersOffloaded = result.SingleGPU.Layers
			config.FullyOffloaded = result.SingleGPU.FullyLoaded
		}

	case "tensor_parallel":
		if result.TensorParallel != nil {
			config.GPUCount = len(result.TensorParallel.GPUSizes)
			config.TotalMemory = result.TensorParallel.VRAMSize
			config.LayersOffloaded = result.TensorParallel.Layers
			config.FullyOffloaded = result.TensorParallel.FullyLoaded
			config.TensorSplit = result.TensorParallel.TensorSplit

			// Calculate memory per GPU
			if config.GPUCount > 0 {
				config.MemoryPerGPU = config.TotalMemory / uint64(config.GPUCount)
			}
		}

	case "cpu_only":
		if result.CPUOnly != nil {
			config.GPUCount = 0
			config.TotalMemory = result.CPUOnly.TotalSize
			config.LayersOffloaded = 0
			config.FullyOffloaded = false
		}
	}

	return config
}

// GPUConfiguration represents a recommended GPU configuration for a model
type GPUConfiguration struct {
	ModelName       string `json:"model_name"`
	Recommendation  string `json:"recommendation"`
	GPUCount        int    `json:"gpu_count"`
	MemoryPerGPU    uint64 `json:"memory_per_gpu"`
	TotalMemory     uint64 `json:"total_memory"`
	LayersOffloaded int    `json:"layers_offloaded"`
	FullyOffloaded  bool   `json:"fully_offloaded"`
	TensorSplit     []int  `json:"tensor_split,omitempty"`
}

// FormatMemorySize formats memory size in human-readable format
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

// GetMemoryEfficiency calculates the memory efficiency of a configuration
func GetMemoryEfficiency(result *EstimationResult) float64 {
	if result == nil || result.Metadata == nil {
		return 0.0
	}

	totalLayers := int(result.Metadata.BlockCount) + 1

	var layersOffloaded int
	switch result.Recommendation {
	case "single_gpu":
		if result.SingleGPU != nil {
			layersOffloaded = result.SingleGPU.Layers
		}
	case "tensor_parallel":
		if result.TensorParallel != nil {
			layersOffloaded = result.TensorParallel.Layers
		}
	default:
		return 0.0
	}

	if totalLayers == 0 {
		return 0.0
	}

	return float64(layersOffloaded) / float64(totalLayers)
}
