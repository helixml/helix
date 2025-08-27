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

	// DefaultOllamaParallelSequences is the default number of parallel sequences for Ollama models
	// Reduced from 4 to 2 to limit memory usage (parallelism multiplies context allocation)
	DefaultOllamaParallelSequences = 2

	// DefaultVLLMParallelSequences is the default number of parallel sequences for VLLM models
	DefaultVLLMParallelSequences = 256

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

// MemoryEstimationRequest represents a memory estimation request sent from API to runner
// This struct MUST be identical on both API and runner sides to avoid protocol mismatches
type MemoryEstimationRequest struct {
	ModelName     string `json:"model_name"`
	ContextLength int    `json:"context_length"`
	BatchSize     int    `json:"batch_size"`
	NumParallel   int    `json:"num_parallel"`
}

// MemoryEstimationResponse represents the response from runner to API
type MemoryEstimationResponse struct {
	Success        bool                            `json:"success"`
	Error          string                          `json:"error,omitempty"`
	ModelName      string                          `json:"model_name"`
	ModelPath      string                          `json:"model_path"`
	Architecture   string                          `json:"architecture"`
	BlockCount     int                             `json:"block_count"`
	Configurations []MemoryEstimationConfiguration `json:"configurations"`
	ResponseTimeMs int64                           `json:"response_time_ms"`
	RunnerID       string                          `json:"runner_id"`
}

// MemoryEstimationConfiguration represents a single GPU configuration option
type MemoryEstimationConfiguration struct {
	Name          string   `json:"name"`
	GPUCount      int      `json:"gpu_count"`
	GPUSizes      []uint64 `json:"gpu_sizes"`
	TotalMemory   uint64   `json:"total_memory"`
	VRAMRequired  uint64   `json:"vram_required"`
	WeightsMemory uint64   `json:"weights_memory"`
	KVCache       uint64   `json:"kv_cache"`
	GraphMemory   uint64   `json:"graph_memory"`
	TensorSplit   string   `json:"tensor_split"`
	LayersOnGPU   int      `json:"layers_on_gpu"`
	TotalLayers   int      `json:"total_layers"`
	FullyLoaded   bool     `json:"fully_loaded"`
}
