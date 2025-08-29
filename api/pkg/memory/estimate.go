package memory

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// EstimateGPULayers estimates how many layers can be loaded on the given GPUs
// This function uses the exact Ollama memory estimation algorithm via type adapters
func EstimateGPULayers(gpus []GPUInfo, metadata *ModelMetadata, opts EstimateOptions) *MemoryEstimate {
	// Memory estimation is now done on the runner using exact Ollama code
	// This function should not be used anymore
	log.Warn().
		Str("architecture", metadata.Architecture).
		Msg("EstimateGPULayers called but memory estimation should now be done on runner - function deprecated")

	return nil
}

// CPU-only estimation removed - not properly supported and adds confusion
// All memory estimation should be GPU-based since we don't properly support CPU inference

// EstimateModelMemory creates comprehensive memory estimates for different GPU configurations
func EstimateModelMemory(metadata *ModelMetadata, gpuConfig []GPUInfo, opts EstimateOptions) *EstimationResult {
	result := &EstimationResult{
		ModelName:      fmt.Sprintf("%s (%s)", metadata.Architecture, metadata.FileType),
		Metadata:       metadata,
		EstimatedAt:    time.Now(),
		Recommendation: "insufficient_memory", // Default until we have working GPU estimation
	}

	// This function is deprecated - memory estimation should be done on the runner
	log.Warn().
		Str("architecture", metadata.Architecture).
		Msg("EstimateModelMemory called but estimation should be done on runner - function deprecated")

	return result
}
