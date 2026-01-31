package scheduler

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/memory"
	"github.com/helixml/helix/api/pkg/types"
)

// GPUAllocationConfig represents a GPU allocation decision made by the scheduler
type GPUAllocationConfig struct {
	GPUCount     int   // Number of GPUs to use (1, 2, 4, 8, etc.)
	SpecificGPUs []int // Which specific GPU indices to use (e.g., [0, 1, 3])
}

// NewModelForGPUAllocation creates a properly configured model for a specific GPU allocation.
// This is the ONLY way to create models for scheduling - it ensures memory values are consistent
// and allocation info is properly set based on the scheduler's GPU allocation decision.
func NewModelForGPUAllocation(baseModel *types.Model, allocation GPUAllocationConfig, memoryEstimationService MemoryEstimationService) (*types.Model, error) {
	// Validate the base model first
	if err := validateModelForAllocation(baseModel); err != nil {
		return nil, fmt.Errorf("invalid base model %s: %w", baseModel.ID, err)
	}

	// Validate allocation config
	if allocation.GPUCount <= 0 {
		return nil, fmt.Errorf("invalid GPU count %d for model %s", allocation.GPUCount, baseModel.ID)
	}
	if len(allocation.SpecificGPUs) != allocation.GPUCount {
		return nil, fmt.Errorf("GPU count %d doesn't match specific GPUs %v for model %s",
			allocation.GPUCount, allocation.SpecificGPUs, baseModel.ID)
	}

	// Create configured model by copying all base fields
	configuredModel := *baseModel

	switch baseModel.Runtime {
	case types.RuntimeVLLM:
		// VLLM: Use admin-configured memory from database
		if baseModel.Memory == 0 {
			return nil, fmt.Errorf("VLLM model %s has no admin-configured memory", baseModel.ID)
		}

		// Apply the scheduler's allocation decision
		configuredModel.AllocatedMemory = baseModel.Memory // Total memory for this model
		configuredModel.AllocatedGPUCount = allocation.GPUCount
		configuredModel.AllocatedSpecificGPUs = allocation.SpecificGPUs

		// For VLLM, distribute memory evenly across GPUs for tensor parallel setups
		if allocation.GPUCount > 1 {
			memoryPerGPU := baseModel.Memory / uint64(allocation.GPUCount)
			configuredModel.AllocatedPerGPUMemory = make([]uint64, allocation.GPUCount)
			for i := range configuredModel.AllocatedPerGPUMemory {
				configuredModel.AllocatedPerGPUMemory[i] = memoryPerGPU
			}
		} else {
			configuredModel.AllocatedPerGPUMemory = []uint64{baseModel.Memory}
		}

	case types.RuntimeOllama:
		// Ollama: Use GGUF estimation, database Memory MUST be 0
		if baseModel.Memory != 0 {
			return nil, fmt.Errorf("CRITICAL: Ollama model %s has non-zero database memory (%d) - should be 0",
				baseModel.ID, baseModel.Memory)
		}

		// Get authoritative GGUF estimation with proper options from model store
		estimateOptions := memory.CreateAutoEstimateOptions(baseModel.ContextLength)
		// Override concurrency if specified in model
		if baseModel.Concurrency > 0 {
			estimateOptions.NumParallel = int(baseModel.Concurrency)
		} else {
			estimateOptions.NumParallel = memory.DefaultOllamaParallelSequences
		}

		result, err := memoryEstimationService.EstimateModelMemory(context.Background(), baseModel.ID, estimateOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to estimate memory for Ollama model %s: %w", baseModel.ID, err)
		}

		// Apply the scheduler's allocation decision
		configuredModel.AllocatedGPUCount = allocation.GPUCount
		configuredModel.AllocatedSpecificGPUs = allocation.SpecificGPUs

		if allocation.GPUCount == 1 {
			// Single GPU allocation
			if result.SingleGPU == nil {
				return nil, fmt.Errorf("Ollama model %s cannot fit on single GPU", baseModel.ID)
			}
			configuredModel.AllocatedMemory = result.SingleGPU.TotalSize
			configuredModel.AllocatedPerGPUMemory = []uint64{result.SingleGPU.VRAMSize}
		} else {
			// Multi-GPU allocation
			if result.TensorParallel == nil {
				return nil, fmt.Errorf("Ollama model %s cannot use tensor parallel with %d GPUs", baseModel.ID, allocation.GPUCount)
			}
			if len(result.TensorParallel.GPUSizes) != allocation.GPUCount {
				return nil, fmt.Errorf("Ollama model %s tensor parallel GPU sizes (%d) don't match allocation (%d)",
					baseModel.ID, len(result.TensorParallel.GPUSizes), allocation.GPUCount)
			}
			configuredModel.AllocatedMemory = result.TensorParallel.TotalSize
			configuredModel.AllocatedPerGPUMemory = result.TensorParallel.GPUSizes
		}

	default:
		return nil, fmt.Errorf("unsupported runtime %s for model %s", baseModel.Runtime, baseModel.ID)
	}

	// Mark as properly configured
	configuredModel.AllocationConfigured = true

	return &configuredModel, nil
}

// validateModelForAllocation ensures a model is properly set up for allocation
func validateModelForAllocation(model *types.Model) error {
	if model == nil {
		return fmt.Errorf("model is nil")
	}
	if model.ID == "" {
		return fmt.Errorf("model ID is empty")
	}

	switch model.Runtime {
	case types.RuntimeOllama:
		if model.Memory != 0 {
			return fmt.Errorf("Ollama model %s must have Memory=0 in database, found %d", model.ID, model.Memory)
		}
		if model.ContextLength == 0 {
			return fmt.Errorf("Ollama model %s must have ContextLength>0 for GGUF estimation", model.ID)
		}
	case types.RuntimeVLLM:
		if model.Memory == 0 {
			return fmt.Errorf("VLLM model %s must have admin-configured Memory>0 in database", model.ID)
		}
	default:
		return fmt.Errorf("unsupported runtime: %s", model.Runtime)
	}
	return nil
}

// ConvertGPUAllocationToConfig converts scheduler's GPU allocation results to GPUAllocationConfig
func ConvertGPUAllocationToConfig(singleGPU *int, multiGPUs []int) (GPUAllocationConfig, error) {
	if singleGPU != nil {
		return GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{*singleGPU},
		}, nil
	} else if len(multiGPUs) > 0 {
		return GPUAllocationConfig{
			GPUCount:     len(multiGPUs),
			SpecificGPUs: multiGPUs,
		}, nil
	} else {
		return GPUAllocationConfig{}, fmt.Errorf("no GPU allocation provided")
	}
}
