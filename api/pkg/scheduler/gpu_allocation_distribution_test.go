package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestGPUAllocationDistribution tests that the one-slot-per-cycle fix properly distributes
// GPU allocations across multiple GPUs by checking the allocation decisions stored by the scheduler
func TestGPUAllocationDistribution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Use simple models for testing
	testModels := []*types.Model{
		{ID: "model-a", Memory: 10 * 1024 * 1024 * 1024, Runtime: types.RuntimeOllama, Prewarm: true}, // 10GB
		{ID: "model-b", Memory: 15 * 1024 * 1024 * 1024, Runtime: types.RuntimeOllama, Prewarm: true}, // 15GB
	}

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(testModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

	for _, model := range testModels {
		mockStore.EXPECT().GetModel(gomock.Any(), model.ID).Return(model, nil).AnyTimes()
	}

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},      // Use mock health checker for tests
		RunnerClient:  DefaultMockRunnerClient(), // Use mock runner client for tests
	})
	require.NoError(t, err)

	fastInterval := 100 * time.Millisecond
	_, err = NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController:        runnerCtrl,
		QueueSize:               50,
		RunnerReconcileInterval: &fastInterval, // Fast reconciliation for tests
	})
	require.NoError(t, err)

	testRunnerID := "gpu-distribution-test-runner"

	// Set up a runner with 2 GPUs, each with 30GB memory (60GB total)
	gpuMemoryBytes := uint64(30 * 1024 * 1024 * 1024) // 30GB per GPU
	totalMemoryBytes := 2 * gpuMemoryBytes            // 60GB total

	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			ID:          testRunnerID,
			TotalMemory: totalMemoryBytes,
			GPUCount:    2,
			GPUs: []*types.GPUStatus{
				{
					Index:       0,
					TotalMemory: gpuMemoryBytes,
					FreeMemory:  gpuMemoryBytes,
					UsedMemory:  0,
					ModelName:   "Test GPU 0",
				},
				{
					Index:       1,
					TotalMemory: gpuMemoryBytes,
					FreeMemory:  gpuMemoryBytes,
					UsedMemory:  0,
					ModelName:   "Test GPU 1",
				},
			},
			Models: []*types.RunnerModelStatus{
				{ModelID: "model-a", Runtime: types.RuntimeOllama, DownloadInProgress: false},
				{ModelID: "model-b", Runtime: types.RuntimeOllama, DownloadInProgress: false},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Empty slots initially
	runnerCtrl.slotsCache.Set(testRunnerID, NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: []*types.RunnerSlot{}}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Add runner to the controller's runner list
	runnerCtrl.runnersMu.Lock()
	runnerCtrl.runners = append(runnerCtrl.runners, testRunnerID)
	runnerCtrl.runnersMu.Unlock()

	t.Logf("Testing GPU allocation distribution with one-slot-per-cycle fix")
	t.Logf("Runner has 2 GPUs with %d GB each (%d GB total)",
		gpuMemoryBytes/(1024*1024*1024), totalMemoryBytes/(1024*1024*1024))

	for i, model := range testModels {
		t.Logf("Model %d: %s (%d GB)", i+1, model.ID, model.Memory/(1024*1024*1024))
	}

	// Set up minimal scheduler context with no initial slots
	SetupMinimalSchedulerContext(runnerCtrl, make(map[uuid.UUID]*Slot))

	// Manually test the GPU allocation decisions by simulating what the scheduler does
	// during slot reconciliation, but without actually creating slots

	// First allocation - should go to GPU 0 (both GPUs have equal free memory)
	t.Logf("\n=== Testing First GPU Allocation (model-a) ===")
	singleGPU1, multiGPUs1, _ := runnerCtrl.GetOptimalGPUAllocation(testRunnerID, testModels[0].Memory)

	require.NotNil(t, singleGPU1, "First model should get a single GPU allocation")
	assert.Equal(t, 0, *singleGPU1, "First model should be allocated to GPU 0 (or GPU 1, both are equal)")
	assert.Empty(t, multiGPUs1, "First model should use single GPU, not multi-GPU")

	t.Logf("✅ First allocation: model-a → GPU %d", *singleGPU1)

	// Now simulate that the first model has been allocated by creating a test slot
	// This is what would happen after the first reconciliation cycle
	testSlots := make(map[uuid.UUID]*Slot)
	firstSlot := CreateTestSlot(testRunnerID, testModels[0].ID, testModels[0].Memory, singleGPU1, nil)
	testSlots[firstSlot.ID] = firstSlot

	// Set up minimal scheduler context with the allocated slot
	SetupMinimalSchedulerContext(runnerCtrl, testSlots)

	// Second allocation - should now see the first allocation and choose the other GPU
	t.Logf("\n=== Testing Second GPU Allocation (model-b) ===")
	singleGPU2, multiGPUs2, _ := runnerCtrl.GetOptimalGPUAllocation(testRunnerID, testModels[1].Memory)

	require.NotNil(t, singleGPU2, "Second model should get a single GPU allocation")
	assert.Empty(t, multiGPUs2, "Second model should use single GPU, not multi-GPU")

	t.Logf("✅ Second allocation: model-b → GPU %d", *singleGPU2)

	// The key test: verify that the models are distributed across different GPUs
	if *singleGPU1 != *singleGPU2 {
		t.Logf("🎉 SUCCESS: Models distributed across GPUs!")
		t.Logf("   model-a (10GB) → GPU %d", *singleGPU1)
		t.Logf("   model-b (15GB) → GPU %d", *singleGPU2)
		t.Logf("✅ GPU distribution is working correctly with one-slot-per-cycle fix")
	} else {
		t.Errorf("❌ FAILED: Both models allocated to same GPU %d", *singleGPU1)
		t.Logf("This suggests the GPU allocation logic is not considering existing allocations")

		// Debug information
		allocatedMemory := runnerCtrl.calculateAllocatedMemoryPerGPU(testRunnerID)
		t.Logf("Debug - Allocated memory per GPU: %+v", allocatedMemory)
	}

	// Additional verification: check that the allocation decisions make sense
	allocatedMemoryPerGPU := runnerCtrl.calculateAllocatedMemoryPerGPU(testRunnerID)

	gpu0Memory := allocatedMemoryPerGPU[0]
	gpu1Memory := allocatedMemoryPerGPU[1]

	t.Logf("\nFinal GPU memory allocation:")
	t.Logf("  GPU 0: %d GB allocated out of %d GB", gpu0Memory/(1024*1024*1024), gpuMemoryBytes/(1024*1024*1024))
	t.Logf("  GPU 1: %d GB allocated out of %d GB", gpu1Memory/(1024*1024*1024), gpuMemoryBytes/(1024*1024*1024))

	// Verify that memory calculations are correct
	expectedGpu0Memory := uint64(0)
	expectedGpu1Memory := uint64(0)

	if *singleGPU1 == 0 {
		expectedGpu0Memory += testModels[0].Memory
	} else {
		expectedGpu1Memory += testModels[0].Memory
	}

	assert.Equal(t, expectedGpu0Memory, gpu0Memory, "GPU 0 allocated memory should match expected")
	assert.Equal(t, expectedGpu1Memory, gpu1Memory, "GPU 1 allocated memory should match expected")
}
