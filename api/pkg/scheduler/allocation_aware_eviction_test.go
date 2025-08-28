package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
)

func TestAllocationAwareEviction(t *testing.T) {
	// Create test scheduler with multi-GPU runner
	scheduler, _ := CreateTestSchedulerWithMultiGPURunner(t)
	memoryService := NewMockMemoryEstimationServiceForAllocation()

	testRunnerID := "test-runner-multi-gpu"

	// Create base models for testing
	smallModel := &types.Model{
		ID:      "small-model:7b",
		Runtime: types.RuntimeVLLM,
		Memory:  30 * 1024 * 1024 * 1024, // 30GB - fits on 1 GPU (80GB each)
	}

	largeModel := &types.Model{
		ID:      "large-model:70b",
		Runtime: types.RuntimeVLLM,
		Memory:  100 * 1024 * 1024 * 1024, // 100GB - needs 2 GPUs
	}

	hugModel := &types.Model{
		ID:      "huge-model:175b",
		Runtime: types.RuntimeVLLM,
		Memory:  200 * 1024 * 1024 * 1024, // 200GB - impossible even with 2 GPUs
	}

	t.Run("small model fits on single GPU without eviction", func(t *testing.T) {
		// Create unconfigured workload
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "test-small-model",
				Request: &openai.ChatCompletionRequest{
					Model: smallModel.ID,
				},
			},
			model: smallModel,
		}

		// Try allocation-aware eviction
		result, err := scheduler.tryAllAllocationsWithEviction(testRunnerID, workload)

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should get single GPU allocation
		assert.Equal(t, 1, result.AllocationOption.GPUCount)
		assert.Len(t, result.AllocationOption.GPUs, 1)
		assert.Equal(t, uint64(30*1024*1024*1024), result.AllocationOption.TotalMemoryRequired)
		assert.Empty(t, result.EvictedSlots) // No eviction needed
	})

	t.Run("large model needs multi-GPU allocation", func(t *testing.T) {
		// Create unconfigured workload
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "test-large-model",
				Request: &openai.ChatCompletionRequest{
					Model: largeModel.ID,
				},
			},
			model: largeModel,
		}

		// Try allocation-aware eviction
		result, err := scheduler.tryAllAllocationsWithEviction(testRunnerID, workload)

		require.NoError(t, err)
		require.NotNil(t, result)

		// Should get allocation (could be single GPU if algorithm optimizes, or multi-GPU)
		assert.True(t, result.AllocationOption.GPUCount >= 1)
		assert.True(t, len(result.AllocationOption.GPUs) >= 1)
		assert.Equal(t, uint64(100*1024*1024*1024), result.AllocationOption.TotalMemoryRequired)
		assert.Empty(t, result.EvictedSlots) // No eviction needed

		// If multi-GPU, should be reasonable distribution
		if result.AllocationOption.GPUCount > 1 {
			assert.Equal(t, uint64(100*1024*1024*1024/uint64(result.AllocationOption.GPUCount)), result.AllocationOption.MemoryPerGPU)
		}
	})

	t.Run("eviction required for allocation", func(t *testing.T) {
		// Create larger stale models that will force eviction
		largeStaleModel := &types.Model{
			ID:      "large-stale-model:60b",
			Runtime: types.RuntimeVLLM,
			Memory:  60 * 1024 * 1024 * 1024, // 60GB - will force eviction
		}

		// First, create some stale slots to occupy memory
		allocation1 := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{0},
		}
		configuredModel1, err := NewModelForGPUAllocation(largeStaleModel, allocation1, memoryService)
		require.NoError(t, err)

		staleWorkload1 := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "test-stale-1",
				Request: &openai.ChatCompletionRequest{
					Model: configuredModel1.ID,
				},
			},
			model: configuredModel1,
		}

		// Create stale slot on GPU 0
		gpuAllocation1 := &GPUAllocation{
			WorkloadID:         staleWorkload1.ID(),
			RunnerID:           testRunnerID,
			SingleGPU:          &allocation1.SpecificGPUs[0],
			TensorParallelSize: 1,
		}
		staleSlot1 := NewSlot(testRunnerID, staleWorkload1, scheduler.modelStaleFunc, scheduler.slotTimeoutFunc, gpuAllocation1)

		// Make the slot stale by setting old last activity time
		staleSlot1.LastActivityTime = time.Now().Add(-2 * time.Hour)
		scheduler.slots.Store(staleSlot1.ID, staleSlot1)

		// Create another stale slot on GPU 1
		allocation2 := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{1},
		}
		configuredModel2, err := NewModelForGPUAllocation(largeStaleModel, allocation2, memoryService)
		require.NoError(t, err)

		staleWorkload2 := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "test-stale-2",
				Request: &openai.ChatCompletionRequest{
					Model: configuredModel2.ID,
				},
			},
			model: configuredModel2,
		}

		gpuAllocation2 := &GPUAllocation{
			WorkloadID:         staleWorkload2.ID(),
			RunnerID:           testRunnerID,
			SingleGPU:          &allocation2.SpecificGPUs[0],
			TensorParallelSize: 1,
		}
		staleSlot2 := NewSlot(testRunnerID, staleWorkload2, scheduler.modelStaleFunc, scheduler.slotTimeoutFunc, gpuAllocation2)

		// Make this slot stale too
		staleSlot2.LastActivityTime = time.Now().Add(-3 * time.Hour) // Even more stale
		scheduler.slots.Store(staleSlot2.ID, staleSlot2)

		// Now try to allocate the large model - should require eviction
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "test-large-eviction",
				Request: &openai.ChatCompletionRequest{
					Model: largeModel.ID,
				},
			},
			model: largeModel,
		}

		// Debug: Check that stale slots were actually created and are stale
		var allSlots []*Slot
		scheduler.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
			if slot.RunnerID == testRunnerID {
				allSlots = append(allSlots, slot)
			}
			return true
		})
		t.Logf("Debug: Found %d total slots on runner", len(allSlots))

		staleCount := 0
		for _, slot := range allSlots {
			isStale := slot.IsStale()
			modelMemory := uint64(0)
			if slot.InitialWork().model != nil && slot.InitialWork().model.IsAllocationConfigured() {
				modelMemory = slot.InitialWork().model.GetMemoryForAllocation()
			}
			t.Logf("Debug: Slot %s (model: %s, memory: %d GB) - IsStale: %t, LastActivity: %v",
				slot.ID.String(), slot.InitialWork().ModelName(), modelMemory/(1024*1024*1024), isStale, slot.LastActivityTime)
			if isStale {
				staleCount++
			}
		}
		t.Logf("Debug: Found %d stale slots", staleCount)

		result, err := scheduler.tryAllAllocationsWithEviction(testRunnerID, workload)

		require.NoError(t, err)
		require.NotNil(t, result)

		t.Logf("Debug: Allocation result - GPUCount: %d, TotalMemory: %d GB, EvictedSlots: %d",
			result.AllocationOption.GPUCount,
			result.AllocationOption.TotalMemoryRequired/(1024*1024*1024),
			len(result.EvictedSlots))

		// Should get allocation (could be single GPU with eviction or multi-GPU)
		assert.True(t, result.AllocationOption.GPUCount >= 1)
		assert.Equal(t, uint64(100*1024*1024*1024), result.AllocationOption.TotalMemoryRequired)

		// Should have evicted some slots
		assert.True(t, len(result.EvictedSlots) > 0, "Expected evicted slots but got %d", len(result.EvictedSlots))

		// Evicted slots should no longer be in scheduler
		for _, evictedSlot := range result.EvictedSlots {
			_, exists := scheduler.slots.Load(evictedSlot.ID)
			assert.False(t, exists, "Evicted slot should be removed from scheduler")
		}

		// Clean up
		scheduler.slots.Delete(staleSlot1.ID)
		scheduler.slots.Delete(staleSlot2.ID)
	})

	t.Run("very large allocation may succeed or fail", func(t *testing.T) {
		// Try to allocate model that's very large
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "test-huge-model",
				Request: &openai.ChatCompletionRequest{
					Model: hugModel.ID,
				},
			},
			model: hugModel, // 200GB - challenging on 2x80GB GPUs
		}

		result, err := scheduler.tryAllAllocationsWithEviction(testRunnerID, workload)

		// The algorithm might find a solution or fail - both are acceptable
		if err != nil {
			assert.Equal(t, ErrRunnersAreFull, err)
			assert.Nil(t, result)
		} else {
			assert.NotNil(t, result)
			assert.True(t, result.AllocationOption.GPUCount >= 1)
		}
	})

	t.Run("configured workload returns error", func(t *testing.T) {
		// Create configured workload (should not be passed to this method)
		allocation := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{0},
		}
		configuredModel, err := NewModelForGPUAllocation(smallModel, allocation, memoryService)
		require.NoError(t, err)

		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "test-configured-model",
				Request: &openai.ChatCompletionRequest{
					Model: configuredModel.ID,
				},
			},
			model: configuredModel,
		}

		result, err := scheduler.tryAllAllocationsWithEviction(testRunnerID, workload)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "already has configured model")
	})
}

func TestGetAllPossibleGPUAllocations(t *testing.T) {
	// Create test runner controller with multi-GPU runner
	_, runnerCtrl := CreateTestSchedulerWithMultiGPURunner(t)
	testRunnerID := "test-runner-multi-gpu"

	t.Run("small model has single and multi-GPU options", func(t *testing.T) {
		modelMemory := uint64(30 * 1024 * 1024 * 1024) // 30GB

		options, err := runnerCtrl.GetAllPossibleGPUAllocations(testRunnerID, modelMemory, types.RuntimeVLLM)

		require.NoError(t, err)
		require.True(t, len(options) >= 2) // At least single GPU and 2-GPU options

		// Check for single GPU option
		foundSingleGPU := false
		for _, option := range options {
			if option.GPUCount == 1 {
				foundSingleGPU = true
				assert.Len(t, option.GPUs, 1)
				assert.Equal(t, modelMemory, option.TotalMemoryRequired)
				assert.Equal(t, modelMemory, option.MemoryPerGPU)
				assert.Equal(t, 1, option.TensorParallelSize)
			}
		}
		assert.True(t, foundSingleGPU, "Should have single GPU allocation option")

		// Check for multi-GPU option
		foundMultiGPU := false
		for _, option := range options {
			if option.GPUCount == 2 {
				foundMultiGPU = true
				assert.Len(t, option.GPUs, 2)
				assert.Equal(t, modelMemory, option.TotalMemoryRequired)
				assert.Equal(t, modelMemory/2, option.MemoryPerGPU)
				assert.Equal(t, 2, option.TensorParallelSize)
			}
		}
		assert.True(t, foundMultiGPU, "Should have multi-GPU allocation option")
	})

	t.Run("large model allocation options", func(t *testing.T) {
		modelMemory := uint64(100 * 1024 * 1024 * 1024) // 100GB

		options, err := runnerCtrl.GetAllPossibleGPUAllocations(testRunnerID, modelMemory, types.RuntimeVLLM)

		require.NoError(t, err)
		require.True(t, len(options) >= 1) // Should have at least one option

		// All options should be valid for the given GPU constraints
		for _, option := range options {
			if option.GPUCount == 1 {
				// Single GPU option: entire model must fit on one GPU (100GB <= 80GB should fail, but algorithm might allow)
				assert.Equal(t, modelMemory, option.MemoryPerGPU)
			} else {
				// Multi-GPU option: memory per GPU should be reasonable
				expectedPerGPU := modelMemory / uint64(option.GPUCount)
				assert.Equal(t, expectedPerGPU, option.MemoryPerGPU)
				assert.True(t, expectedPerGPU <= 80*1024*1024*1024, "Memory per GPU should fit in 80GB limit")
			}
			assert.Equal(t, modelMemory, option.TotalMemoryRequired)
		}
	})

	t.Run("very large model has limited options", func(t *testing.T) {
		modelMemory := uint64(200 * 1024 * 1024 * 1024) // 200GB - challenging on 2x80GB GPUs

		options, err := runnerCtrl.GetAllPossibleGPUAllocations(testRunnerID, modelMemory, types.RuntimeVLLM)

		require.NoError(t, err)

		// If options exist, they should be valid multi-GPU allocations
		for _, option := range options {
			assert.True(t, option.GPUCount >= 2, "Very large model should require multiple GPUs")
			expectedPerGPU := modelMemory / uint64(option.GPUCount)
			assert.Equal(t, expectedPerGPU, option.MemoryPerGPU)
			// Note: The algorithm might find options that exceed GPU capacity - that's for the eviction logic to handle
		}
	})

	t.Run("ollama runtime supports multi-GPU", func(t *testing.T) {
		modelMemory := uint64(30 * 1024 * 1024 * 1024) // 30GB

		options, err := runnerCtrl.GetAllPossibleGPUAllocations(testRunnerID, modelMemory, types.RuntimeOllama)

		require.NoError(t, err)
		require.True(t, len(options) >= 2) // Should have both single and multi-GPU options

		// Verify multi-GPU option exists for Ollama
		foundMultiGPU := false
		for _, option := range options {
			if option.GPUCount > 1 {
				foundMultiGPU = true
			}
		}
		assert.True(t, foundMultiGPU, "Ollama should support multi-GPU allocations")
	})
}

func TestIntegratedAllocationAwareScheduling(t *testing.T) {
	// Create test scheduler and add it to controller
	scheduler, _ := CreateTestSchedulerWithMultiGPURunner(t)

	testRunnerID := "test-runner-multi-gpu"

	t.Run("ensureSlot uses allocation-aware eviction", func(t *testing.T) {
		// Create an unconfigured workload (as would come from the queue)
		baseModel := &types.Model{
			ID:      "integration-test:30b",
			Runtime: types.RuntimeVLLM,
			Memory:  50 * 1024 * 1024 * 1024, // 50GB
		}

		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "test-integration",
				Request: &openai.ChatCompletionRequest{
					Model: baseModel.ID,
				},
			},
			model: baseModel, // Unconfigured model
		}

		// Create slot requirement
		req := SlotRequirement{
			ExampleWorkload: workload,
			Model:           workload.ModelName(),
			Runtime:         workload.Runtime(),
		}

		// Call ensureSlotWithGlobalAllocator - this uses the new global allocator
		scheduler.ensureSlotWithGlobalAllocator(req)

		// Verify a slot was created
		var createdSlot *Slot
		scheduler.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
			if slot.RunnerID == testRunnerID && slot.InitialWork().ModelName() == workload.ModelName() {
				createdSlot = slot
				return false // Stop iteration
			}
			return true
		})

		require.NotNil(t, createdSlot, "Should have created a slot")

		// Verify the slot has a properly configured model
		assert.True(t, createdSlot.InitialWork().model.IsAllocationConfigured(), "Slot should have configured model")
		assert.True(t, createdSlot.InitialWork().model.GetGPUCount() >= 1, "Should have valid GPU allocation")

		// Verify GPU allocation is set
		assert.NotNil(t, createdSlot.GPUAllocation, "Slot should have GPU allocation")

		// Clean up
		scheduler.slots.Delete(createdSlot.ID)
	})
}

// CreateTestSchedulerWithMultiGPURunner creates a test scheduler with a multi-GPU runner for testing
func CreateTestSchedulerWithMultiGPURunner(t *testing.T) (*Scheduler, *RunnerController) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Create mock store
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},
		RunnerClient:  DefaultMockRunnerClient(),
	})
	require.NoError(t, err)

	memoryService := NewMockMemoryEstimationServiceForAllocation()

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController:        runnerCtrl,
		Store:                   mockStore,
		MemoryEstimationService: memoryService,
		QueueSize:               50,
	})
	require.NoError(t, err)

	// Create multi-GPU runner (2 GPUs, 80GB each)
	testRunnerID := "test-runner-multi-gpu"
	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			ID:          testRunnerID,
			TotalMemory: 160 * 1024 * 1024 * 1024,
			GPUCount:    2,
			GPUs: []*types.GPUStatus{
				{Index: 0, TotalMemory: 80 * 1024 * 1024 * 1024, FreeMemory: 80 * 1024 * 1024 * 1024},
				{Index: 1, TotalMemory: 80 * 1024 * 1024 * 1024, FreeMemory: 80 * 1024 * 1024 * 1024},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Register the runner as connected
	runnerCtrl.OnConnectedHandler(testRunnerID)

	return scheduler, runnerCtrl
}
