package types

// Important: NumGPU vs GPU Count Distinction
// - NumGPU field in EstimateOptions controls how many LAYERS to offload to GPU (-1 = auto-detect all that fit)
// - Number of GPUs is controlled by the GPU configuration array passed separately to the estimation function
// - These are two different concepts that are easy to confuse

// Constants for VLLM-specific memory estimation
const (
	// DefaultVLLMParallelSequences is the default number of parallel sequences for VLLM models
	DefaultVLLMParallelSequences = 256
)

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
