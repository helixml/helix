package scheduler

import (
	"context"
	"fmt"
	"testing"

	"github.com/helixml/helix/api/pkg/memory"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockMemoryEstimationServiceForAllocation provides memory estimates for model allocation testing
type MockMemoryEstimationServiceForAllocation struct {
	modelMemory map[string]*memory.EstimationResult
}

func NewMockMemoryEstimationServiceForAllocation() *MockMemoryEstimationServiceForAllocation {
	return &MockMemoryEstimationServiceForAllocation{
		modelMemory: map[string]*memory.EstimationResult{
			"ollama-small:7b": {
				Recommendation: "single_gpu",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  7 * 1024 * 1024 * 1024,
					TotalSize: 7 * 1024 * 1024 * 1024,
				},
			},
			"ollama-large:70b": {
				Recommendation: "tensor_parallel",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  70 * 1024 * 1024 * 1024,
					TotalSize: 70 * 1024 * 1024 * 1024,
				},
				TensorParallel: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  70 * 1024 * 1024 * 1024,
					TotalSize: 70 * 1024 * 1024 * 1024,
					GPUSizes:  []uint64{35 * 1024 * 1024 * 1024, 35 * 1024 * 1024 * 1024}, // 35GB per GPU
				},
			},
			"ollama-workflow:70b": {
				Recommendation: "tensor_parallel",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  70 * 1024 * 1024 * 1024,
					TotalSize: 70 * 1024 * 1024 * 1024,
				},
				TensorParallel: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  70 * 1024 * 1024 * 1024,
					TotalSize: 70 * 1024 * 1024 * 1024,
					GPUSizes:  []uint64{35 * 1024 * 1024 * 1024, 35 * 1024 * 1024 * 1024}, // 35GB per GPU
				},
			},
			"hot-path-test": {
				Recommendation: "single_gpu",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  7 * 1024 * 1024 * 1024,
					TotalSize: 7 * 1024 * 1024 * 1024,
				},
			},
			"consistency-test": {
				Recommendation: "single_gpu",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  7 * 1024 * 1024 * 1024,
					TotalSize: 7 * 1024 * 1024 * 1024,
				},
			},
			"qwen3:8b": {
				Recommendation: "single_gpu",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  10 * 1024 * 1024 * 1024,
					TotalSize: 10 * 1024 * 1024 * 1024,
				},
			},
			"ollama-impossible:200b": {
				Recommendation: "insufficient_memory",
			},
		},
	}
}

func (m *MockMemoryEstimationServiceForAllocation) EstimateModelMemory(ctx context.Context, modelName string, opts memory.EstimateOptions) (*memory.EstimationResult, error) {
	result, ok := m.modelMemory[modelName]
	if !ok {
		return nil, fmt.Errorf("model %s not found in mock", modelName)
	}
	return result, nil
}

func TestNewModelForGPUAllocation_VLLMModels(t *testing.T) {
	memoryService := NewMockMemoryEstimationServiceForAllocation()

	t.Run("valid VLLM single GPU", func(t *testing.T) {
		baseModel := &types.Model{
			ID:      "vllm-model:30b",
			Runtime: types.RuntimeVLLM,
			Memory:  30 * 1024 * 1024 * 1024, // 30GB admin-configured
		}

		allocation := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{0},
		}

		configuredModel, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.NoError(t, err)

		// Verify allocation info is set correctly
		assert.True(t, configuredModel.IsAllocationConfigured())
		assert.Equal(t, uint64(30*1024*1024*1024), configuredModel.GetMemoryForAllocation())
		assert.Equal(t, 1, configuredModel.GetGPUCount())
		assert.Equal(t, 1, configuredModel.GetTensorParallelSize())
		assert.Equal(t, []uint64{30 * 1024 * 1024 * 1024}, configuredModel.GetPerGPUMemory())
		assert.Equal(t, []int{0}, configuredModel.GetSpecificGPUs())
	})

	t.Run("valid VLLM multi GPU", func(t *testing.T) {
		baseModel := &types.Model{
			ID:      "vllm-large:70b",
			Runtime: types.RuntimeVLLM,
			Memory:  70 * 1024 * 1024 * 1024, // 70GB admin-configured
		}

		allocation := GPUAllocationConfig{
			GPUCount:     2,
			SpecificGPUs: []int{0, 1},
		}

		configuredModel, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.NoError(t, err)

		// Verify allocation info is set correctly for multi-GPU
		assert.True(t, configuredModel.IsAllocationConfigured())
		assert.Equal(t, uint64(70*1024*1024*1024), configuredModel.GetMemoryForAllocation())
		assert.Equal(t, 2, configuredModel.GetGPUCount())
		assert.Equal(t, 2, configuredModel.GetTensorParallelSize())
		assert.Equal(t, []uint64{35 * 1024 * 1024 * 1024, 35 * 1024 * 1024 * 1024}, configuredModel.GetPerGPUMemory()) // 35GB per GPU
		assert.Equal(t, []int{0, 1}, configuredModel.GetSpecificGPUs())
	})

	t.Run("invalid VLLM - no memory", func(t *testing.T) {
		baseModel := &types.Model{
			ID:      "vllm-invalid",
			Runtime: types.RuntimeVLLM,
			Memory:  0, // Invalid for VLLM
		}

		allocation := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{0},
		}

		_, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have admin-configured Memory>0 in database")
	})
}

func TestNewModelForGPUAllocation_OllamaModels(t *testing.T) {
	memoryService := NewMockMemoryEstimationServiceForAllocation()

	t.Run("valid Ollama single GPU", func(t *testing.T) {
		baseModel := &types.Model{
			ID:            "ollama-small:7b",
			Runtime:       types.RuntimeOllama,
			Memory:        0,    // Must be 0 for Ollama
			ContextLength: 8192, // Required for GGUF estimation
		}

		allocation := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{0},
		}

		configuredModel, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.NoError(t, err)

		// Verify allocation info uses GGUF estimation
		assert.True(t, configuredModel.IsAllocationConfigured())
		assert.Equal(t, uint64(7*1024*1024*1024), configuredModel.GetMemoryForAllocation()) // From GGUF estimation
		assert.Equal(t, 1, configuredModel.GetGPUCount())
		assert.Equal(t, 1, configuredModel.GetTensorParallelSize())
		assert.Equal(t, []uint64{7 * 1024 * 1024 * 1024}, configuredModel.GetPerGPUMemory())
		assert.Equal(t, []int{0}, configuredModel.GetSpecificGPUs())
	})

	t.Run("valid Ollama multi GPU", func(t *testing.T) {
		baseModel := &types.Model{
			ID:            "ollama-large:70b",
			Runtime:       types.RuntimeOllama,
			Memory:        0,    // Must be 0 for Ollama
			ContextLength: 8192, // Required for GGUF estimation
		}

		allocation := GPUAllocationConfig{
			GPUCount:     2,
			SpecificGPUs: []int{0, 1},
		}

		configuredModel, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.NoError(t, err)

		// Verify allocation info uses tensor parallel GGUF estimation
		assert.True(t, configuredModel.IsAllocationConfigured())
		assert.Equal(t, uint64(70*1024*1024*1024), configuredModel.GetMemoryForAllocation()) // Total from tensor parallel
		assert.Equal(t, 2, configuredModel.GetGPUCount())
		assert.Equal(t, 2, configuredModel.GetTensorParallelSize())
		assert.Equal(t, []uint64{35 * 1024 * 1024 * 1024, 35 * 1024 * 1024 * 1024}, configuredModel.GetPerGPUMemory()) // From tensor parallel
		assert.Equal(t, []int{0, 1}, configuredModel.GetSpecificGPUs())
	})

	t.Run("invalid Ollama - non-zero memory", func(t *testing.T) {
		baseModel := &types.Model{
			ID:            "ollama-invalid",
			Runtime:       types.RuntimeOllama,
			Memory:        10 * 1024 * 1024 * 1024, // Invalid for Ollama - should be 0
			ContextLength: 8192,
		}

		allocation := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{0},
		}

		_, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have Memory=0 in database")
	})

	t.Run("invalid Ollama - no context length", func(t *testing.T) {
		baseModel := &types.Model{
			ID:            "ollama-no-context",
			Runtime:       types.RuntimeOllama,
			Memory:        0,
			ContextLength: 0, // Invalid for GGUF estimation
		}

		allocation := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{0},
		}

		_, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Ollama model ollama-no-context must have ContextLength>0")
	})

	t.Run("Ollama model that can't fit", func(t *testing.T) {
		baseModel := &types.Model{
			ID:            "ollama-impossible:200b",
			Runtime:       types.RuntimeOllama,
			Memory:        0,
			ContextLength: 8192,
		}

		allocation := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{0},
		}

		_, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot fit on single GPU")
	})
}

func TestModelAllocation_FailFastBehavior(t *testing.T) {
	// Test that unconfigured models panic as expected
	baseModel := &types.Model{
		ID:      "unconfigured-model",
		Runtime: types.RuntimeVLLM,
		Memory:  10 * 1024 * 1024 * 1024,
	}

	// Accessing methods on unconfigured model should panic
	t.Run("GetMemoryForAllocation panics on unconfigured model", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = baseModel.GetMemoryForAllocation()
		})
	})

	t.Run("GetGPUCount panics on unconfigured model", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = baseModel.GetGPUCount()
		})
	})

	t.Run("GetSpecificGPUs panics on unconfigured model", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = baseModel.GetSpecificGPUs()
		})
	})
}

func TestModelAllocation_DatabaseMemoryAccess(t *testing.T) {
	t.Run("VLLM model database memory access", func(t *testing.T) {
		vllmModel := &types.Model{
			ID:      "vllm-test",
			Runtime: types.RuntimeVLLM,
			Memory:  20 * 1024 * 1024 * 1024,
		}

		dbMem, err := vllmModel.GetDatabaseMemory()
		require.NoError(t, err)
		assert.Equal(t, uint64(20*1024*1024*1024), dbMem)
	})

	t.Run("VLLM model with no database memory", func(t *testing.T) {
		vllmModel := &types.Model{
			ID:      "vllm-no-mem",
			Runtime: types.RuntimeVLLM,
			Memory:  0, // Invalid
		}

		_, err := vllmModel.GetDatabaseMemory()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "has no admin-configured memory value")
	})

	t.Run("Ollama model database memory access should fail", func(t *testing.T) {
		ollamaModel := &types.Model{
			ID:      "ollama-test",
			Runtime: types.RuntimeOllama,
			Memory:  0, // Correct for Ollama
		}

		_, err := ollamaModel.GetDatabaseMemory()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "should use GetMemoryForAllocation()")
	})

	t.Run("Ollama model with non-zero database memory should fail", func(t *testing.T) {
		ollamaModel := &types.Model{
			ID:      "ollama-bad",
			Runtime: types.RuntimeOllama,
			Memory:  10 * 1024 * 1024 * 1024, // Invalid - should be 0
		}

		_, err := ollamaModel.GetDatabaseMemory()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "has non-zero database memory")
	})
}

func TestConvertGPUAllocationToConfig(t *testing.T) {
	t.Run("single GPU conversion", func(t *testing.T) {
		singleGPU := 0
		config, err := ConvertGPUAllocationToConfig(&singleGPU, nil)
		require.NoError(t, err)
		assert.Equal(t, 1, config.GPUCount)
		assert.Equal(t, []int{0}, config.SpecificGPUs)
	})

	t.Run("multi GPU conversion", func(t *testing.T) {
		multiGPUs := []int{0, 1, 3}
		config, err := ConvertGPUAllocationToConfig(nil, multiGPUs)
		require.NoError(t, err)
		assert.Equal(t, 3, config.GPUCount)
		assert.Equal(t, []int{0, 1, 3}, config.SpecificGPUs)
	})

	t.Run("no allocation should fail", func(t *testing.T) {
		_, err := ConvertGPUAllocationToConfig(nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no GPU allocation provided")
	})
}

func TestModelAllocation_CompleteWorkflow(t *testing.T) {
	// Test the complete workflow: base model -> allocation decision -> configured model -> usage
	memoryService := NewMockMemoryEstimationServiceForAllocation()

	t.Run("complete VLLM workflow", func(t *testing.T) {
		// STEP 1: Start with base model (from database)
		baseModel := &types.Model{
			ID:      "vllm-workflow:30b",
			Runtime: types.RuntimeVLLM,
			Memory:  30 * 1024 * 1024 * 1024,
		}

		// STEP 2: Scheduler makes GPU allocation decision (simulated)
		allocation := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{2},
		}

		// STEP 3: Configure model for allocation
		configuredModel, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.NoError(t, err)

		// STEP 4: Use configured model in scheduling (error-free!)
		totalMemory := configuredModel.GetMemoryForAllocation()
		gpuCount := configuredModel.GetGPUCount()
		specificGPUs := configuredModel.GetSpecificGPUs()

		// Verify values
		assert.Equal(t, uint64(30*1024*1024*1024), totalMemory)
		assert.Equal(t, 1, gpuCount)
		assert.Equal(t, []int{2}, specificGPUs)
	})

	t.Run("complete Ollama workflow", func(t *testing.T) {
		// STEP 1: Start with base model (from database)
		baseModel := &types.Model{
			ID:            "ollama-workflow:70b",
			Runtime:       types.RuntimeOllama,
			Memory:        0,    // Correct for Ollama
			ContextLength: 8192, // Required for GGUF
		}

		// STEP 2: Scheduler makes GPU allocation decision (simulated)
		allocation := GPUAllocationConfig{
			GPUCount:     2,
			SpecificGPUs: []int{0, 1},
		}

		// STEP 3: Configure model for allocation
		configuredModel, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.NoError(t, err)

		// STEP 4: Use configured model in scheduling (error-free!)
		totalMemory := configuredModel.GetMemoryForAllocation()
		gpuCount := configuredModel.GetGPUCount()
		perGPUMemory := configuredModel.GetPerGPUMemory()

		// Verify values use GGUF estimation
		assert.Equal(t, uint64(70*1024*1024*1024), totalMemory) // From tensor parallel estimate
		assert.Equal(t, 2, gpuCount)
		assert.Equal(t, []uint64{35 * 1024 * 1024 * 1024, 35 * 1024 * 1024 * 1024}, perGPUMemory) // From tensor parallel
	})
}

func TestModelAllocation_ValidateModelForAllocation(t *testing.T) {
	t.Run("valid Ollama model", func(t *testing.T) {
		model := &types.Model{
			ID:            "valid-ollama",
			Runtime:       types.RuntimeOllama,
			Memory:        0,
			ContextLength: 8192,
		}
		err := validateModelForAllocation(model)
		assert.NoError(t, err)
	})

	t.Run("valid VLLM model", func(t *testing.T) {
		model := &types.Model{
			ID:      "valid-vllm",
			Runtime: types.RuntimeVLLM,
			Memory:  10 * 1024 * 1024 * 1024,
		}
		err := validateModelForAllocation(model)
		assert.NoError(t, err)
	})

	t.Run("nil model", func(t *testing.T) {
		err := validateModelForAllocation(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model is nil")
	})

	t.Run("empty model ID", func(t *testing.T) {
		model := &types.Model{
			Runtime: types.RuntimeVLLM,
		}
		err := validateModelForAllocation(model)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model ID is empty")
	})

	t.Run("unsupported runtime", func(t *testing.T) {
		model := &types.Model{
			ID:      "unknown-runtime",
			Runtime: "unknown",
		}
		err := validateModelForAllocation(model)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported runtime")
	})
}

func TestModelAllocation_Benefits(t *testing.T) {
	// Demonstrate the benefits of the new architecture
	memoryService := NewMockMemoryEstimationServiceForAllocation()

	t.Run("eliminates memory calculation errors in hot paths", func(t *testing.T) {
		baseModel := &types.Model{
			ID:            "hot-path-test",
			Runtime:       types.RuntimeOllama,
			Memory:        0,
			ContextLength: 8192,
		}

		allocation := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{0},
		}

		configuredModel, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.NoError(t, err)

		// OLD WAY: memory, err := getModelMemory(model.ID) - could fail!
		// NEW WAY: No errors possible in hot paths!
		for i := 0; i < 1000; i++ {
			memory := configuredModel.GetMemoryForAllocation() // Never fails!
			gpuCount := configuredModel.GetGPUCount()          // Never fails!
			assert.Greater(t, memory, uint64(0))
			assert.Greater(t, gpuCount, 0)
		}
	})

	t.Run("prevents inconsistent memory values", func(t *testing.T) {
		// Demonstrate that database vs GGUF inconsistency is eliminated
		baseModel := &types.Model{
			ID:            "consistency-test",
			Runtime:       types.RuntimeOllama,
			Memory:        0, // Database value
			ContextLength: 8192,
		}

		allocation := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{0},
		}

		configuredModel, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.NoError(t, err)

		// The configured model always uses authoritative values
		authoritativeMemory := configuredModel.GetMemoryForAllocation()

		// OLD: Could get different values from model.Memory vs getModelMemory()
		// NEW: Always consistent!
		assert.Equal(t, uint64(7*1024*1024*1024), authoritativeMemory) // From GGUF, not DB
	})

	t.Run("carries GPU allocation info with model", func(t *testing.T) {
		baseModel := &types.Model{
			ID:      "gpu-info-test",
			Runtime: types.RuntimeVLLM,
			Memory:  40 * 1024 * 1024 * 1024,
		}

		allocation := GPUAllocationConfig{
			GPUCount:     2,
			SpecificGPUs: []int{1, 3},
		}

		configuredModel, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.NoError(t, err)

		// GPU allocation info travels with the model
		assert.Equal(t, 2, configuredModel.GetGPUCount())
		assert.Equal(t, 2, configuredModel.GetTensorParallelSize())
		assert.Equal(t, []int{1, 3}, configuredModel.GetSpecificGPUs())
		assert.Equal(t, []uint64{20 * 1024 * 1024 * 1024, 20 * 1024 * 1024 * 1024}, configuredModel.GetPerGPUMemory())

		// OLD: Would need separate tracking of GPU allocation
		// NEW: Everything is encapsulated in the model!
	})
}
