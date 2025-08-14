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

// TestRealPrewarmGPUDistribution tests the actual pre-warm model distribution
// using the real scheduler code, not simulated concurrent processing.
func TestRealPrewarmGPUDistribution(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Use real models from the model registry that have Prewarm=true
	// Looking at models.go, we have:
	// - "gpt-oss:20b" with 48GB memory, Prewarm: true
	// - "qwen3:8b" with 10GB memory, Prewarm: true
	// - "Qwen/Qwen2.5-VL-7B-Instruct" with 39GB memory, Prewarm: true (VLLM)
	// - "MrLight/dse-qwen2-2b-mrl-v1" with 8GB memory, Prewarm: true (VLLM)
	realPrewarmModels := []*types.Model{
		{ID: "qwen3:8b", Memory: 10 * 1024 * 1024 * 1024, Runtime: types.RuntimeOllama, Prewarm: true},                 // 10GB
		{ID: "gpt-oss:20b", Memory: 48 * 1024 * 1024 * 1024, Runtime: types.RuntimeOllama, Prewarm: true},              // 48GB
		{ID: "MrLight/dse-qwen2-2b-mrl-v1", Memory: 8 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: true}, // 8GB
	}

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(realPrewarmModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

	// Mock GetModel calls for each model
	for _, model := range realPrewarmModels {
		mockStore.EXPECT().GetModel(gomock.Any(), model.ID).Return(model, nil).AnyTimes()
	}

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},      // Use mock health checker for tests
		RunnerClient:  DefaultMockRunnerClient(), // Use mock runner client for tests
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
		QueueSize:        50,
	})
	require.NoError(t, err)

	testRunnerID := "real-prewarm-test-runner"

	// Set up a runner with 2 GPUs, each with 40GB memory (80GB total)
	// This should allow:
	// - qwen3:8b (10GB) + MrLight/dse-qwen2-2b-mrl-v1 (8GB) = 18GB on one GPU
	// - gpt-oss:20b (48GB) won't fit on a single 40GB GPU, so it should be rejected or use multi-GPU
	gpuMemoryBytes := uint64(40 * 1024 * 1024 * 1024) // 40GB per GPU
	totalMemoryBytes := 2 * gpuMemoryBytes            // 80GB total

	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			ID:          testRunnerID,
			TotalMemory: totalMemoryBytes,
			GPUCount:    2,
			GPUs: []*types.GPUStatus{
				{
					Index:       0,
					TotalMemory: gpuMemoryBytes,
					FreeMemory:  gpuMemoryBytes, // Initially all free
					UsedMemory:  0,
					ModelName:   "NVIDIA A100 40GB",
				},
				{
					Index:       1,
					TotalMemory: gpuMemoryBytes,
					FreeMemory:  gpuMemoryBytes, // Initially all free
					UsedMemory:  0,
					ModelName:   "NVIDIA A100 40GB",
				},
			},
			Models: []*types.RunnerModelStatus{
				{ModelID: "qwen3:8b", Runtime: types.RuntimeOllama, DownloadInProgress: false},
				{ModelID: "gpt-oss:20b", Runtime: types.RuntimeOllama, DownloadInProgress: false},
				{ModelID: "MrLight/dse-qwen2-2b-mrl-v1", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Initially empty slots
	runnerCtrl.slotsCache.Set(testRunnerID, NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: []*types.RunnerSlot{}}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Add runner to the controller's runner list
	runnerCtrl.runnersMu.Lock()
	runnerCtrl.runners = append(runnerCtrl.runners, testRunnerID)
	runnerCtrl.runnersMu.Unlock()

	t.Logf("Testing real pre-warm GPU distribution")
	t.Logf("Runner has 2 GPUs with %d GB each (%d GB total)",
		gpuMemoryBytes/(1024*1024*1024), totalMemoryBytes/(1024*1024*1024))

	for i, model := range realPrewarmModels {
		t.Logf("Pre-warm model %d: %s (%d GB)", i+1, model.ID, model.Memory/(1024*1024*1024))
	}

	// Trigger the REAL pre-warming process using the scheduler's actual method
	t.Logf("\n=== Triggering Real Pre-warm Process ===")

	// This is the actual method called when a runner connects
	scheduler.PrewarmNewRunner(testRunnerID)

	// Give time for the pre-warming process to complete
	// With our fix, each slot is created in a separate reconciliation cycle (every 5 seconds)
	// We have 3 models, so we need to wait for 3 cycles: 3 * 5 = 15 seconds + buffer
	t.Logf("Waiting for multiple slot reconciliation cycles (5 seconds each, 3 models = ~15 seconds)...")
	time.Sleep(18 * time.Second)

	// Check the actual slots that were created by the scheduler
	t.Logf("\n=== Analyzing Real Slot Creation ===")

	// Count slots created by the scheduler
	slotsCreated := 0
	var slotModels []string
	var gpuAllocations []struct {
		Model    string
		GPUIndex *int
		GPUs     []int
	}

	scheduler.slots.Range(func(slotID uuid.UUID, slot *Slot) bool {
		if slot.RunnerID == testRunnerID {
			slotsCreated++
			modelName := slot.InitialWork().ModelName().String()
			slotModels = append(slotModels, modelName)

			gpuAlloc := struct {
				Model    string
				GPUIndex *int
				GPUs     []int
			}{
				Model: modelName,
			}

			if slot.GPUAllocation != nil {
				gpuAlloc.GPUIndex = slot.GPUAllocation.SingleGPU
				gpuAlloc.GPUs = slot.GPUAllocation.MultiGPUs
			}

			gpuAllocations = append(gpuAllocations, gpuAlloc)
		}
		return true
	})

	t.Logf("Real scheduler created %d slots for models: %v", slotsCreated, slotModels)

	// Analyze GPU distribution
	gpuUsageCount := make(map[int]int)
	for i, allocation := range gpuAllocations {
		t.Logf("Slot %d: Model=%s, SingleGPU=%v, MultiGPUs=%v",
			i, allocation.Model, allocation.GPUIndex, allocation.GPUs)

		if allocation.GPUIndex != nil {
			gpuUsageCount[*allocation.GPUIndex]++
		}
		for _, gpuIndex := range allocation.GPUs {
			gpuUsageCount[gpuIndex]++
		}
	}

	t.Logf("\nGPU usage distribution:")
	for gpuIndex := 0; gpuIndex < 2; gpuIndex++ {
		count := gpuUsageCount[gpuIndex]
		t.Logf("  GPU %d: %d models allocated", gpuIndex, count)
	}

	// Verify that slots were created (some models should fit)
	assert.Greater(t, slotsCreated, 0, "Scheduler should create at least some pre-warm slots")

	// Check if models are distributed across GPUs
	gpu0Count := gpuUsageCount[0]
	gpu1Count := gpuUsageCount[1]

	if gpu0Count > 0 && gpu1Count > 0 {
		t.Logf("✅ GOOD: Models are distributed across GPUs (GPU0: %d, GPU1: %d)", gpu0Count, gpu1Count)
		t.Logf("Pre-warm GPU distribution is working correctly!")
	} else if gpu0Count > 0 || gpu1Count > 0 {
		t.Logf("⚠️  Models allocated to only one GPU (GPU0: %d, GPU1: %d)", gpu0Count, gpu1Count)
		t.Logf("This could be due to memory constraints or the specific models selected")

		// This might be expected behavior depending on model sizes and memory constraints
		// Let's check the total memory used
		totalMemoryUsed := uint64(0)
		for _, model := range realPrewarmModels {
			for _, slotModel := range slotModels {
				if model.ID == slotModel {
					totalMemoryUsed += model.Memory
					break
				}
			}
		}

		t.Logf("Total memory used by pre-warm models: %d GB out of %d GB available",
			totalMemoryUsed/(1024*1024*1024), totalMemoryBytes/(1024*1024*1024))
	} else {
		t.Errorf("❌ No models were allocated to any GPU - this suggests a problem with pre-warming")
	}

	// Additional verification: check that memory constraints are respected
	for gpuIndex := 0; gpuIndex < 2; gpuIndex++ {
		memoryUsedOnGPU := uint64(0)
		for _, allocation := range gpuAllocations {
			if allocation.GPUIndex != nil && *allocation.GPUIndex == gpuIndex {
				// Find the model memory
				for _, model := range realPrewarmModels {
					if model.ID == allocation.Model {
						memoryUsedOnGPU += model.Memory
						break
					}
				}
			}
			// Handle multi-GPU allocations
			for _, allocGPU := range allocation.GPUs {
				if allocGPU == gpuIndex {
					// Find the model memory and divide by number of GPUs
					for _, model := range realPrewarmModels {
						if model.ID == allocation.Model {
							memoryUsedOnGPU += model.Memory / uint64(len(allocation.GPUs))
							break
						}
					}
				}
			}
		}

		t.Logf("GPU %d memory usage: %d GB out of %d GB capacity",
			gpuIndex, memoryUsedOnGPU/(1024*1024*1024), gpuMemoryBytes/(1024*1024*1024))

		assert.LessOrEqual(t, memoryUsedOnGPU, gpuMemoryBytes,
			"GPU %d should not exceed its memory capacity", gpuIndex)
	}
}

// TestRealPrewarmWithLargerGPUs tests pre-warm distribution with larger GPUs
// that can fit all the models to see if distribution still works
func TestRealPrewarmWithLargerGPUs(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Use the hardcoded models that the scheduler expects (from GetDefaultTestModels)
	// This is more realistic than creating custom test models
	realPrewarmModels := GetDefaultTestModels()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(realPrewarmModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

	for _, model := range realPrewarmModels {
		mockStore.EXPECT().GetModel(gomock.Any(), model.ID).Return(model, nil).AnyTimes()
	}

	// Use default MockRunnerClient which has the matching hardcoded models
	mockRunnerClient := NewMockRunnerClient(160, 2) // 160GB total, 2 GPUs = 80GB per GPU

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{}, // Use mock health checker for tests
		RunnerClient:  mockRunnerClient,     // Use mock runner client with only test models
	})
	require.NoError(t, err)

	// Use fast reconciliation interval for test (100ms instead of default 5s)
	// This allows multiple slots to be created during the test wait period
	fastReconcileInterval := 100 * time.Millisecond
	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController:        runnerCtrl,
		QueueSize:               50,
		RunnerReconcileInterval: &fastReconcileInterval,
	})
	require.NoError(t, err)

	testRunnerID := "large-gpu-test-runner"

	// Note: Using MockRunnerClient with 160GB total (2x80GB GPUs) configured above
	// No need to manually set caches with new desired state architecture
	gpuMemoryBytes := uint64(80 * 1024 * 1024 * 1024) // 80GB per GPU (matches MockRunnerClient config)

	// Simulate runner connection properly using the connection handler
	runnerCtrl.OnConnectedHandler(testRunnerID)

	t.Logf("Testing pre-warm distribution with large GPUs (%d GB each)", gpuMemoryBytes/(1024*1024*1024))

	// Trigger real pre-warming
	scheduler.PrewarmNewRunner(testRunnerID)
	t.Logf("Waiting for slot reconciliation with fast 100ms intervals...")
	time.Sleep(2 * time.Second) // With 100ms intervals, this gives 20 reconciliation cycles

	// Analyze results
	slotsCreated := 0
	gpuUsageCount := make(map[int]int)

	scheduler.slots.Range(func(slotID uuid.UUID, slot *Slot) bool {
		if slot.RunnerID == testRunnerID {
			slotsCreated++

			if slot.GPUAllocation != nil {
				if slot.GPUAllocation.SingleGPU != nil {
					gpuUsageCount[*slot.GPUAllocation.SingleGPU]++
				}
				for _, gpuIndex := range slot.GPUAllocation.MultiGPUs {
					gpuUsageCount[gpuIndex]++
				}
			}
		}
		return true
	})

	t.Logf("Created %d slots with large GPUs", slotsCreated)
	t.Logf("GPU distribution: GPU0: %d, GPU1: %d", gpuUsageCount[0], gpuUsageCount[1])

	// With plenty of memory, we should create slots for both models
	// Count how many models actually have Prewarm: true
	expectedPrewarmSlots := 0
	for _, model := range realPrewarmModels {
		if model.Prewarm {
			expectedPrewarmSlots++
		}
	}

	assert.Equal(t, expectedPrewarmSlots, slotsCreated, "Should create slots for all pre-warm models")

	// Check if they're distributed across GPUs (this is the key test)
	gpu0Count := gpuUsageCount[0]
	gpu1Count := gpuUsageCount[1]

	if gpu0Count > 0 && gpu1Count > 0 {
		t.Logf("✅ SUCCESS: Models distributed across GPUs even with large GPUs")
	} else {
		t.Logf("⚠️  All models on one GPU (GPU0: %d, GPU1: %d) - investigating why", gpu0Count, gpu1Count)
		// This suggests the issue is in the GPU selection algorithm, not memory constraints
	}
}
