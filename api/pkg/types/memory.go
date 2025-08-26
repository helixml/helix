package types

import "github.com/helixml/helix/api/pkg/memory"

// Important: NumGPU vs GPU Count Distinction
// - NumGPU field in EstimateOptions controls how many LAYERS to offload to GPU (-1 = auto-detect all that fit)
// - Number of GPUs is controlled by the GPU configuration array passed separately to the estimation function
// - These are two different concepts that are easy to confuse

// Constants for memory estimation to avoid duplication
const (
	// DefaultKVCacheType is the KV cache type we use for Ollama models
	DefaultKVCacheType = "q8_0"

	// DefaultBatchSize is the standard batch size for memory estimation
	DefaultBatchSize = 512

	// DefaultParallelSequences is the standard number of parallel sequences
	DefaultParallelSequences = 1

	// AutoDetectLayers means auto-detect how many layers fit on GPU
	AutoDetectLayers = -1
)

// CreateOllamaEstimateOptions creates EstimateOptions for Ollama models with sensible defaults
// contextLength: the model's context length
// numLayersOnGPU: number of layers to offload (-1 for auto-detect, which allows all layers that fit)
func CreateOllamaEstimateOptions(contextLength int64, numLayersOnGPU int) memory.EstimateOptions {
	return memory.EstimateOptions{
		NumCtx:      int(contextLength),
		NumBatch:    DefaultBatchSize,
		NumParallel: DefaultParallelSequences,
		NumGPU:      numLayersOnGPU,
		KVCacheType: DefaultKVCacheType,
	}
}

// CreateAutoEstimateOptions creates EstimateOptions with auto-detection (all layers that fit)
// This is the most common case for scheduling and dashboard display
func CreateAutoEstimateOptions(contextLength int64) memory.EstimateOptions {
	return CreateOllamaEstimateOptions(contextLength, AutoDetectLayers)
}

// CreateEstimateOptionsForGPUArray creates EstimateOptions when using a GPU configuration array
// This is used when the number of GPUs is specified by the GPU config array passed separately.
// The NumGPU field is set to auto-detect layers since GPU count is handled elsewhere.
func CreateEstimateOptionsForGPUArray(contextLength int64) memory.EstimateOptions {
	// Always use auto-detect for layers when GPU array specifies the hardware config
	return CreateOllamaEstimateOptions(contextLength, AutoDetectLayers)
}
