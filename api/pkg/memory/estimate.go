package memory

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// EstimateGPULayers estimates how many layers can be loaded on the given GPUs
// This function uses the exact Ollama memory estimation algorithm via type adapters
func EstimateGPULayers(gpus []GPUInfo, metadata *ModelMetadata, opts EstimateOptions) *MemoryEstimate {
	if len(gpus) == 0 || (len(gpus) == 1 && gpus[0].Library == "cpu") {
		return estimateCPUOnly(metadata, opts)
	}

	// Use exact Ollama estimation algorithm
	estimate := EstimateGPULayersUsingOllama(gpus, metadata, opts)

	log.Debug().
		Str("architecture", metadata.Architecture).
		Int("layers", estimate.Layers).
		Uint64("vram_mb", estimate.VRAMSize/(1024*1024)).
		Uint64("total_mb", estimate.TotalSize/(1024*1024)).
		Bool("fully_loaded", estimate.FullyLoaded).
		Msg("Exact Ollama memory estimation completed")

	return estimate
}

// estimateCPUOnly returns a CPU-only estimation
func estimateCPUOnly(metadata *ModelMetadata, opts EstimateOptions) *MemoryEstimate {
	// Simple CPU estimation - assume we can load everything but it will be slow
	totalSize := uint64(0)

	// Estimate total model size from layers
	for _, layer := range metadata.Layers {
		for _, tensor := range layer.Tensors {
			totalSize += tensor.Size
		}
	}

	return &MemoryEstimate{
		Architecture:     metadata.Architecture,
		Layers:           int(metadata.BlockCount),
		VRAMSize:         0, // No VRAM used in CPU mode
		TotalSize:        totalSize,
		Graph:            totalSize / 10,             // Rough estimate for graph memory
		KVCache:          uint64(opts.NumCtx * 1024), // Rough KV cache estimate
		Weights:          totalSize,
		Projectors:       0,
		FullyLoaded:      true,
		RequiresFallback: false, // CPU can handle any model size (just slowly)
		EstimatedAt:      time.Now(),
		Options:          opts,
		GPUs:             []GPUInfo{{Library: "cpu"}},
		GPUSizes:         []uint64{0},
		TensorSplit:      []int{int(metadata.BlockCount)},
	}
}

// EstimateModelMemory creates comprehensive memory estimates for different GPU configurations
func EstimateModelMemory(metadata *ModelMetadata, gpuConfig []GPUInfo, opts EstimateOptions) *EstimationResult {
	result := &EstimationResult{
		ModelName:      fmt.Sprintf("%s (%s)", metadata.Architecture, metadata.FileType),
		Metadata:       metadata,
		EstimatedAt:    time.Now(),
		Recommendation: "cpu_only", // Default to CPU until we have working GPU estimation
	}

	// Single GPU estimation
	if len(gpuConfig) >= 1 {
		singleGPU := []GPUInfo{gpuConfig[0]}
		result.SingleGPU = EstimateGPULayers(singleGPU, metadata, opts)

		// If single GPU can handle it, that's our recommendation
		if !result.SingleGPU.RequiresFallback {
			result.Recommendation = "single_gpu"
		}
	}

	// Multi-GPU tensor parallel estimation
	if len(gpuConfig) > 1 {
		result.TensorParallel = EstimateGPULayers(gpuConfig, metadata, opts)

		// If tensor parallel works better, use that
		if !result.TensorParallel.RequiresFallback &&
			(result.SingleGPU == nil || result.SingleGPU.RequiresFallback) {
			result.Recommendation = "tensor_parallel"
		}
	}

	// CPU-only fallback
	result.CPUOnly = estimateCPUOnly(metadata, opts)

	return result
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
