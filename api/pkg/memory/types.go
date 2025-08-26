package memory

import (
	"time"
)

// ModelMetadata represents the essential metadata extracted from a GGUF model file
type ModelMetadata struct {
	Architecture    string                 `json:"architecture"`
	FileType        string                 `json:"file_type"`
	BlockCount      uint64                 `json:"block_count"`
	EmbeddingLength uint64                 `json:"embedding_length"`
	ContextLength   uint64                 `json:"context_length"`
	HeadCount       uint64                 `json:"head_count"`
	HeadCountKV     uint64                 `json:"head_count_kv"`
	KeyLength       uint64                 `json:"key_length"`
	ValueLength     uint64                 `json:"value_length"`
	FFLength        uint64                 `json:"ff_length"`
	VocabSize       uint64                 `json:"vocab_size"`
	Layers          map[string]LayerInfo   `json:"layers"`
	AdditionalKV    map[string]interface{} `json:"additional_kv"` // For architecture-specific metadata
}

// LayerInfo represents information about a model layer
type LayerInfo struct {
	Tensors map[string]TensorInfo `json:"tensors"`
}

// TensorInfo represents information about an individual tensor
type TensorInfo struct {
	Shape []uint64 `json:"shape"`
	Type  string   `json:"type"`
	Size  uint64   `json:"size"`
}

// GPUInfo represents information about a GPU for memory estimation
// This extends the existing runner.GPUInfo with additional fields needed for estimation
type GPUInfo struct {
	ID            string `json:"id"`
	Index         int    `json:"index"`
	Library       string `json:"library"` // "cuda", "rocm", "metal", "cpu"
	FreeMemory    uint64 `json:"free_memory"`
	TotalMemory   uint64 `json:"total_memory"`
	MinimumMemory uint64 `json:"minimum_memory"`

	// Additional fields for compatibility with Ollama's estimation
	Variant              string      `json:"variant,omitempty"`
	Compute              string      `json:"compute,omitempty"`
	DriverMajor          int         `json:"driver_major,omitempty"`
	DriverMinor          int         `json:"driver_minor,omitempty"`
	Name                 string      `json:"name,omitempty"`
	DependencyPath       []string    `json:"dependency_path,omitempty"`
	EnvWorkarounds       [][2]string `json:"env_workarounds,omitempty"`
	UnreliableFreeMemory bool        `json:"unreliable_free_memory,omitempty"`
}

// EstimateOptions represents options for memory estimation
type EstimateOptions struct {
	NumCtx      int `json:"num_ctx"`      // Context size
	NumBatch    int `json:"num_batch"`    // Batch size
	NumParallel int `json:"num_parallel"` // Number of parallel sequences

	// ⚠️  CRITICAL CONFUSION WARNING ⚠️
	// NumGPU is NOT the number of GPUs in your hardware configuration!
	// NumGPU is the number of MODEL LAYERS to offload to GPU (-1 for auto-detect all that fit)
	//
	// Examples:
	// - NumGPU = -1: Auto-detect max layers that fit (RECOMMENDED - gives full model memory)
	// - NumGPU = 1:  Only offload 1 layer to GPU (gives tiny memory estimate)
	// - NumGPU = 0:  CPU only (no GPU layers)
	//
	// To estimate for different GPU hardware configs (1 GPU vs 4 GPUs),
	// you pass different GPU configuration arrays to the estimation function,
	// NOT different NumGPU values!
	NumGPU int `json:"num_gpu"` // Number of layers to offload (-1 for auto)

	// Advanced options
	FlashAttention bool   `json:"flash_attention,omitempty"`
	KVCacheType    string `json:"kv_cache_type,omitempty"` // "f16", "q8_0", "q4_0"
}

// MemoryEstimate represents the result of memory estimation
type MemoryEstimate struct {
	// Core results
	Layers      int      `json:"layers"`       // Number of layers that can be offloaded
	Graph       uint64   `json:"graph"`        // Graph memory requirement
	VRAMSize    uint64   `json:"vram_size"`    // Total VRAM usage
	TotalSize   uint64   `json:"total_size"`   // Total memory requirement
	TensorSplit []int    `json:"tensor_split"` // Layers per GPU for tensor parallel
	GPUSizes    []uint64 `json:"gpu_sizes"`    // Memory allocation per GPU

	// Breakdown for analysis
	KVCache    uint64 `json:"kv_cache"`   // KV cache memory
	Weights    uint64 `json:"weights"`    // Model weights memory
	GraphMem   uint64 `json:"graph_mem"`  // Graph computation memory
	Projectors uint64 `json:"projectors"` // Projector weights (for multimodal)

	// Metadata
	Architecture     string    `json:"architecture"`
	ModelPath        string    `json:"model_path,omitempty"`
	EstimatedAt      time.Time `json:"estimated_at"`
	FullyLoaded      bool      `json:"fully_loaded"`      // Whether all layers fit on GPU
	RequiresFallback bool      `json:"requires_fallback"` // Whether CPU fallback is needed

	// Configuration used for estimation
	Options EstimateOptions `json:"options"`
	GPUs    []GPUInfo       `json:"gpus"`
}

// EstimationResult represents the complete result including single-GPU and tensor-parallel estimates
type EstimationResult struct {
	ModelName string         `json:"model_name"`
	ModelPath string         `json:"model_path"`
	Metadata  *ModelMetadata `json:"metadata"`

	// Estimation results
	SingleGPU      *MemoryEstimate `json:"single_gpu,omitempty"`
	TensorParallel *MemoryEstimate `json:"tensor_parallel,omitempty"`
	// CPUOnly removed - not properly supported and adds confusion

	// Recommendations
	Recommendation string `json:"recommendation"` // "single_gpu", "tensor_parallel", "insufficient_memory"

	EstimatedAt time.Time `json:"estimated_at"`
	Error       string    `json:"error,omitempty"`
}

// Constants for memory calculations
const (
	// Memory overhead constants
	DefaultGPUOverhead = 512 * 1024 * 1024 // 512MB default GPU overhead

	// Quantization type sizes (bytes per element)
	TypeSizeF32  = 4
	TypeSizeF16  = 2
	TypeSizeBF16 = 2
	TypeSizeQ8_0 = 1
	TypeSizeQ4_0 = 0.5

	// KV cache type multipliers
	KVCacheF16  = 2.0
	KVCacheQ8_0 = 1.0
	KVCacheQ4_0 = 0.5

	// Architecture detection patterns
	ArchitectureLlama     = "llama"
	ArchitectureLlama4    = "llama4"
	ArchitectureQwen2     = "qwen2"
	ArchitectureQwen3     = "qwen3"
	ArchitectureGemma     = "gemma"
	ArchitectureGemma2    = "gemma2"
	ArchitectureGemma3    = "gemma3"
	ArchitectureMllama    = "mllama"
	ArchitectureCommandR  = "command-r"
	ArchitecturePhi2      = "phi2"
	ArchitectureStableLM  = "stablelm"
	ArchitectureDeepSeek2 = "deepseek2"
	ArchitectureChatGLM   = "chatglm"
	ArchitectureGPTOSS    = "gptoss"
)

// Error types
type EstimationError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *EstimationError) Error() string {
	if e.Details != "" {
		return e.Message + ": " + e.Details
	}
	return e.Message
}

// Common error types
var (
	ErrModelNotFound   = &EstimationError{Type: "model_not_found", Message: "model file not found"}
	ErrInvalidGGUF     = &EstimationError{Type: "invalid_gguf", Message: "invalid or corrupted GGUF file"}
	ErrUnsupportedArch = &EstimationError{Type: "unsupported_architecture", Message: "unsupported model architecture"}
	ErrInsufficientGPU = &EstimationError{Type: "insufficient_gpu", Message: "insufficient GPU memory"}
	ErrInvalidOptions  = &EstimationError{Type: "invalid_options", Message: "invalid estimation options"}
)
