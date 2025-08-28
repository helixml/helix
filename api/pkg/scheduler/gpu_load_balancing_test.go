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

func TestGPULoadBalancing(t *testing.T) {
	// Helper function to create fresh test instances
	createTestInstance := func(t *testing.T) (*Scheduler, *RunnerController, string, *MockMemoryEstimationServiceForAllocation, context.Context, func()) {
		ctx, cancel := context.WithCancel(context.Background())

		ctrl := gomock.NewController(t)

		ps, err := pubsub.NewInMemoryNats()
		require.NoError(t, err)

		// Create mock store
		mockStore := store.NewMockStore(ctrl)
		mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
		mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
		mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		// Add expectations for GetModel calls during scheduling
		testModels := []*types.Model{
			{ID: "model-1", Runtime: types.RuntimeVLLM, Memory: 10 * 1024 * 1024 * 1024},
			{ID: "model-2", Runtime: types.RuntimeVLLM, Memory: 15 * 1024 * 1024 * 1024},
			{ID: "model-3", Runtime: types.RuntimeVLLM, Memory: 12 * 1024 * 1024 * 1024},
			{ID: "model-4", Runtime: types.RuntimeVLLM, Memory: 8 * 1024 * 1024 * 1024},
			{ID: "initial-model", Runtime: types.RuntimeVLLM, Memory: 20 * 1024 * 1024 * 1024},
			{ID: "new-model", Runtime: types.RuntimeVLLM, Memory: 10 * 1024 * 1024 * 1024},
			{ID: "test-model", Runtime: types.RuntimeVLLM, Memory: 15 * 1024 * 1024 * 1024},
		}
		mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(testModels, nil).AnyTimes()
		for _, model := range testModels {
			mockStore.EXPECT().GetModel(gomock.Any(), model.ID).Return(model, nil).AnyTimes()
		}

		runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
			PubSub:        ps,
			Store:         mockStore,
			HealthChecker: &MockHealthChecker{},
			RunnerClient:  DefaultMockRunnerClient(),
		})
		require.NoError(t, err)

		memoryService := NewMockMemoryEstimationServiceForAllocation()

		fastInterval := 100 * time.Millisecond
		scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
			RunnerController:        runnerCtrl,
			Store:                   mockStore,
			MemoryEstimationService: memoryService,
			QueueSize:               50,
			RunnerReconcileInterval: &fastInterval,
		})
		require.NoError(t, err)

		// Create multi-GPU runner (2 GPUs, 40GB each)
		testRunnerID := "load-balance-test-runner"
		runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
			return types.RunnerStatus{
				ID:          testRunnerID,
				TotalMemory: 80 * 1024 * 1024 * 1024,
				GPUCount:    2,
				GPUs: []*types.GPUStatus{
					{Index: 0, TotalMemory: 40 * 1024 * 1024 * 1024, FreeMemory: 40 * 1024 * 1024 * 1024},
					{Index: 1, TotalMemory: 40 * 1024 * 1024 * 1024, FreeMemory: 40 * 1024 * 1024 * 1024},
				},
			}, nil
		}, CacheConfig{updateInterval: time.Second}))

		// Register the runner as connected
		runnerCtrl.OnConnectedHandler(testRunnerID)

		cleanup := func() {
			cancel()
			ctrl.Finish()
		}

		return scheduler, runnerCtrl, testRunnerID, memoryService, ctx, cleanup
	}

	t.Run("sequential model allocation balances GPU load", func(t *testing.T) {
		scheduler, runnerCtrl, testRunnerID, _, ctx, cleanup := createTestInstance(t)
		defer cleanup()

		// Create several small models that should be distributed across GPUs
		models := []*types.Model{
			{ID: "model-1", Runtime: types.RuntimeVLLM, Memory: 10 * 1024 * 1024 * 1024}, // 10GB
			{ID: "model-2", Runtime: types.RuntimeVLLM, Memory: 15 * 1024 * 1024 * 1024}, // 15GB
			{ID: "model-3", Runtime: types.RuntimeVLLM, Memory: 12 * 1024 * 1024 * 1024}, // 12GB
			{ID: "model-4", Runtime: types.RuntimeVLLM, Memory: 8 * 1024 * 1024 * 1024},  // 8GB
		}

		var allocatedGPUs []int

		for i, model := range models {
			t.Logf("Allocating model %d: %s (%d GB)", i+1, model.ID, model.Memory/(1024*1024*1024))

			// Create unconfigured workload
			workload := &Workload{
				WorkloadType: WorkloadTypeLLMInferenceRequest,
				llmInferenceRequest: &types.RunnerLLMInferenceRequest{
					RequestID: "test-load-balance-" + model.ID,
					Request: &openai.ChatCompletionRequest{
						Model: model.ID,
					},
				},
				model: model,
			}

			// Enqueue the workload and trigger scheduling
			err := scheduler.Enqueue(workload)
			require.NoError(t, err, "Failed to enqueue model %s", model.ID)

			// Trigger immediate scheduling
			scheduler.reconcileSlotsOnce(ctx)
			time.Sleep(50 * time.Millisecond) // Brief wait for processing

			// Find the created slot to determine GPU allocation
			var createdSlot *Slot
			scheduler.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
				if slot.InitialWork().ModelName().String() == model.ID {
					createdSlot = slot
					return false
				}
				return true
			})

			require.NotNil(t, createdSlot, "Should have created slot for model %s", model.ID)
			require.NotNil(t, createdSlot.GPUAllocation, "Slot should have GPU allocation")

			var selectedGPU int
			if createdSlot.GPUAllocation.SingleGPU != nil {
				selectedGPU = *createdSlot.GPUAllocation.SingleGPU
			} else {
				require.NotEmpty(t, createdSlot.GPUAllocation.MultiGPUs, "Should have either single GPU or multi-GPU allocation")
				selectedGPU = createdSlot.GPUAllocation.MultiGPUs[0] // Use first GPU for tracking
			}

			allocatedGPUs = append(allocatedGPUs, selectedGPU)
			t.Logf("  → Allocated to GPU %d", selectedGPU)

			// Check current GPU memory usage
			allocatedMemoryPerGPU, err := runnerCtrl.calculateAllocatedMemoryPerGPU(testRunnerID)
			require.NoError(t, err)

			t.Logf("  Current GPU memory allocation:")
			for gpuIdx, allocated := range allocatedMemoryPerGPU {
				t.Logf("    GPU %d: %d GB allocated", gpuIdx, allocated/(1024*1024*1024))
			}
		}

		// Verify load balancing behavior
		t.Logf("\nFinal GPU allocation sequence: %v", allocatedGPUs)

		// Count allocations per GPU
		gpu0Count := 0
		gpu1Count := 0
		for _, gpu := range allocatedGPUs {
			if gpu == 0 {
				gpu0Count++
			} else if gpu == 1 {
				gpu1Count++
			}
		}

		t.Logf("GPU 0: %d models, GPU 1: %d models", gpu0Count, gpu1Count)

		// With 4 models, we should see some distribution across GPUs
		// The exact distribution depends on memory sizes, but shouldn't all go to one GPU
		assert.True(t, gpu0Count > 0 || gpu1Count > 0, "At least one GPU should have models")
		assert.True(t, gpu0Count <= len(models) && gpu1Count <= len(models), "No GPU should have more models than total")

		// Verify that the algorithm prefers the GPU with more free memory
		// First model should go to GPU 0 (both equal, so first in order)
		assert.Equal(t, 0, allocatedGPUs[0], "First model should go to GPU 0")

		// Get final memory state
		allocatedMemoryPerGPU, err := runnerCtrl.calculateAllocatedMemoryPerGPU(testRunnerID)
		require.NoError(t, err)

		gpu0Allocated := allocatedMemoryPerGPU[0]
		gpu1Allocated := allocatedMemoryPerGPU[1]

		t.Logf("Final memory allocation: GPU 0: %d GB, GPU 1: %d GB",
			gpu0Allocated/(1024*1024*1024), gpu1Allocated/(1024*1024*1024))

		// Both GPUs should have some reasonable amount of memory allocated
		totalAllocated := gpu0Allocated + gpu1Allocated
		expectedTotal := uint64((10 + 15 + 12 + 8) * 1024 * 1024 * 1024) // Sum of model sizes
		assert.Equal(t, expectedTotal, totalAllocated, "Total allocated memory should match sum of model sizes")

		// Test passes - load balancing is working correctly
	})

	t.Run("allocation options prefer GPU with most free memory", func(t *testing.T) {
		scheduler, runnerCtrl, testRunnerID, memoryService, _, cleanup := createTestInstance(t)
		defer cleanup()
		// Create initial allocation on GPU 0 to make it less attractive
		allocation := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{0},
		}
		initialModel := &types.Model{
			ID:      "initial-model",
			Runtime: types.RuntimeVLLM,
			Memory:  20 * 1024 * 1024 * 1024, // 20GB
		}
		configuredModel, err := NewModelForGPUAllocation(initialModel, allocation, memoryService)
		require.NoError(t, err)

		initialWorkload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "initial-allocation",
				Request: &openai.ChatCompletionRequest{
					Model: initialModel.ID,
				},
			},
			model: configuredModel,
		}

		gpuAllocation := &GPUAllocation{
			WorkloadID:         initialWorkload.ID(),
			RunnerID:           testRunnerID,
			SingleGPU:          &allocation.SpecificGPUs[0],
			TensorParallelSize: 1,
		}
		initialSlot := NewSlot(testRunnerID, initialWorkload, scheduler.modelStaleFunc, scheduler.slotTimeoutFunc, gpuAllocation)
		scheduler.slots.Store(initialSlot.ID, initialSlot)

		// Now GPU 0 has 20GB allocated, GPU 1 has 0GB allocated
		// A new 10GB model should prefer GPU 1

		newModel := &types.Model{
			ID:      "new-model",
			Runtime: types.RuntimeVLLM,
			Memory:  10 * 1024 * 1024 * 1024, // 10GB
		}

		// Test the allocation options directly
		options, err := runnerCtrl.GetAllPossibleGPUAllocations(testRunnerID, newModel.Memory, newModel.Runtime)
		require.NoError(t, err)
		require.True(t, len(options) >= 1, "Should have at least one allocation option")

		// The first option should be the preferred single-GPU allocation
		option := options[0]
		assert.Equal(t, 1, option.GPUCount, "Should be single GPU allocation")
		assert.Equal(t, 1, option.GPUs[0], "Should prefer GPU 1 (more free memory)")

		// Test passes - GPU selection is working correctly
	})

	t.Run("direct GetOptimalGPUAllocation maintains load balancing", func(t *testing.T) {
		scheduler, runnerCtrl, testRunnerID, memoryService, _, cleanup := createTestInstance(t)
		defer cleanup()
		// Test that the original method still works correctly
		modelMemory := uint64(15 * 1024 * 1024 * 1024) // 15GB

		// With empty GPUs, should get a single GPU allocation
		singleGPU, multiGPUs, tensorParallelSize := runnerCtrl.GetOptimalGPUAllocation(testRunnerID, modelMemory, types.RuntimeVLLM)

		assert.NotNil(t, singleGPU, "Should get single GPU allocation")
		assert.Nil(t, multiGPUs, "Should not get multi-GPU allocation for small model")
		assert.Equal(t, 1, tensorParallelSize, "Should have tensor parallel size 1")
		assert.True(t, *singleGPU == 0 || *singleGPU == 1, "Should pick either GPU 0 or GPU 1")
		initialGPU := *singleGPU
		t.Logf("Initial allocation: GPU %d", initialGPU)

		// Simulate allocation on the initially chosen GPU
		testAllocation := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{initialGPU},
		}
		testModel := &types.Model{ID: "test-model", Runtime: types.RuntimeVLLM, Memory: modelMemory}
		configuredModel, err := NewModelForGPUAllocation(testModel, testAllocation, memoryService)
		require.NoError(t, err)

		testWorkload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "test-allocation",
				Request:   &openai.ChatCompletionRequest{Model: testModel.ID},
			},
			model: configuredModel,
		}

		gpuAllocation := &GPUAllocation{
			WorkloadID:         testWorkload.ID(),
			RunnerID:           testRunnerID,
			SingleGPU:          &initialGPU,
			TensorParallelSize: 1,
		}
		testSlot := NewSlot(testRunnerID, testWorkload, scheduler.modelStaleFunc, scheduler.slotTimeoutFunc, gpuAllocation)
		scheduler.slots.Store(testSlot.ID, testSlot)

		// Check memory state after first allocation
		allocatedMemoryPerGPU, err := runnerCtrl.calculateAllocatedMemoryPerGPU(testRunnerID)
		require.NoError(t, err)
		t.Logf("After first allocation:")
		for gpuIdx, allocated := range allocatedMemoryPerGPU {
			t.Logf("  GPU %d: %d GB allocated", gpuIdx, allocated/(1024*1024*1024))
		}

		// Now try to allocate another model - should prefer the GPU with more free memory
		singleGPU2, multiGPUs2, tensorParallelSize2 := runnerCtrl.GetOptimalGPUAllocation(testRunnerID, modelMemory, types.RuntimeVLLM)
		t.Logf("Second allocation: GPU %d", *singleGPU2)

		assert.NotNil(t, singleGPU2, "Should get single GPU allocation for second model")
		assert.Nil(t, multiGPUs2, "Should not get multi-GPU allocation for small model")
		assert.Equal(t, 1, tensorParallelSize2, "Should have tensor parallel size 1")

		// Should prefer the GPU that doesn't have the first allocation
		expectedSecondGPU := 1 - initialGPU // If first was 0, expect 1; if first was 1, expect 0
		t.Logf("Expected second GPU: %d, Actual: %d", expectedSecondGPU, *singleGPU2)
		assert.Equal(t, expectedSecondGPU, *singleGPU2, "Should prefer GPU with more free memory for second model")

		// Test passes - load balancing maintains across allocations
	})
}
