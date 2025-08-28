package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
)

func TestTensorParallelismDiagnosis(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	testModels := []*types.Model{
		{ID: "small-model", Runtime: types.RuntimeVLLM, Memory: 30 * 1024 * 1024 * 1024}, // 30GB - should fit on single 50GB GPU
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

	// Create runner with 2×50GB GPUs
	testRunnerID := "tp-diagnosis-test"
	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			ID:          testRunnerID,
			TotalMemory: 100 * 1024 * 1024 * 1024, // 100GB total
			GPUCount:    2,
			GPUs: []*types.GPUStatus{
				{Index: 0, TotalMemory: 50 * 1024 * 1024 * 1024, FreeMemory: 50 * 1024 * 1024 * 1024}, // 50GB
				{Index: 1, TotalMemory: 50 * 1024 * 1024 * 1024, FreeMemory: 50 * 1024 * 1024 * 1024}, // 50GB
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.OnConnectedHandler(testRunnerID)

	t.Run("diagnose why 30GB model might use tensor parallelism", func(t *testing.T) {
		modelMemory := uint64(30 * 1024 * 1024 * 1024) // 30GB

		// Step 1: Test the original GetOptimalGPUAllocation method
		t.Logf("=== Testing GetOptimalGPUAllocation (original method) ===")
		singleGPU, multiGPUs, tensorParallelSize := runnerCtrl.GetOptimalGPUAllocation(testRunnerID, modelMemory, types.RuntimeVLLM)

		t.Logf("Original allocation result:")
		t.Logf("  SingleGPU: %v", singleGPU)
		t.Logf("  MultiGPUs: %v", multiGPUs)
		t.Logf("  TensorParallelSize: %d", tensorParallelSize)

		if singleGPU != nil {
			t.Logf("✅ Original method correctly chooses single GPU: %d", *singleGPU)
		} else {
			t.Logf("⚠️ Original method unexpectedly chose multi-GPU for 30GB model")
		}

		// Step 2: Test our new GetAllPossibleGPUAllocations method
		t.Logf("\n=== Testing GetAllPossibleGPUAllocations (new method) ===")
		options, err := runnerCtrl.GetAllPossibleGPUAllocations(testRunnerID, modelMemory, types.RuntimeVLLM)
		require.NoError(t, err)

		t.Logf("New method allocation options (%d total):", len(options))
		for i, option := range options {
			t.Logf("  Option %d: %d GPUs, GPUs=%v, MemoryPerGPU=%d GB, TotalMemory=%d GB",
				i+1, option.GPUCount, option.GPUs,
				option.MemoryPerGPU/(1024*1024*1024),
				option.TotalMemoryRequired/(1024*1024*1024))
		}

		// The first option should be single GPU
		if len(options) > 0 {
			firstOption := options[0]
			if firstOption.GPUCount == 1 {
				t.Logf("✅ New method correctly prioritizes single GPU allocation")
			} else {
				t.Logf("⚠️ New method unexpectedly prioritizes %d-GPU allocation", firstOption.GPUCount)
			}
		}

		// Step 3: Test the full allocation-aware eviction
		t.Logf("\n=== Testing tryAllAllocationsWithEviction (full pipeline) ===")
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "tp-diagnosis",
				Request: &openai.ChatCompletionRequest{
					Model: "small-model",
				},
			},
			model: testModels[0], // 30GB model
		}

		result, err := scheduler.tryAllAllocationsWithEviction(testRunnerID, workload)
		if err != nil {
			t.Logf("❌ Full pipeline failed: %v", err)
		} else {
			require.NotNil(t, result)
			t.Logf("Full pipeline result:")
			t.Logf("  GPUCount: %d", result.AllocationOption.GPUCount)
			t.Logf("  GPUs: %v", result.AllocationOption.GPUs)
			t.Logf("  MemoryPerGPU: %d GB", result.AllocationOption.MemoryPerGPU/(1024*1024*1024))
			t.Logf("  TotalMemoryRequired: %d GB", result.AllocationOption.TotalMemoryRequired/(1024*1024*1024))

			if result.AllocationOption.GPUCount == 1 {
				t.Logf("✅ Full pipeline correctly chooses single GPU")
			} else {
				t.Logf("⚠️ Full pipeline unexpectedly chooses %d-GPU allocation", result.AllocationOption.GPUCount)
				t.Logf("   This might indicate a bug in our allocation ordering logic")
			}
		}

		// Step 4: Test actual scheduling through ensureSlot
		t.Logf("\n=== Testing actual ensureSlot scheduling ===")
		req := SlotRequirement{
			ExampleWorkload: workload,
			Model:           workload.ModelName(),
			Runtime:         workload.Runtime(),
		}

		scheduler.ensureSlot(req)
		time.Sleep(100 * time.Millisecond)

		// Check what actually got created
		var createdSlot *Slot
		scheduler.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
			if slot.RunnerID == testRunnerID {
				createdSlot = slot
				return false
			}
			return true
		})

		if createdSlot == nil {
			t.Logf("❌ No slot was created by ensureSlot")
		} else {
			t.Logf("✅ Slot created by ensureSlot:")
			if createdSlot.GPUAllocation.SingleGPU != nil {
				t.Logf("  Single GPU: %d", *createdSlot.GPUAllocation.SingleGPU)
			} else if len(createdSlot.GPUAllocation.MultiGPUs) > 0 {
				t.Logf("  Multi-GPU: %v", createdSlot.GPUAllocation.MultiGPUs)
				t.Logf("  TensorParallelSize: %d", createdSlot.GPUAllocation.TensorParallelSize)
			}

			// Check final memory allocation
			allocatedMemoryPerGPU, err := runnerCtrl.calculateAllocatedMemoryPerGPU(testRunnerID)
			require.NoError(t, err)

			t.Logf("Final GPU memory allocation:")
			for gpuIdx, allocated := range allocatedMemoryPerGPU {
				t.Logf("  GPU %d: %d GB allocated", gpuIdx, allocated/(1024*1024*1024))
			}
		}
	})
}
