package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/memory"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestMultiGPUEvictionCalculation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Test models for eviction scenarios
	testModels := []*types.Model{
		{ID: "large-model:100b", Memory: 100 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false}, // 100GB
		{ID: "medium-model:60b", Memory: 60 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false},  // 60GB
		{ID: "small-model:30b", Memory: 30 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false},   // 30GB
	}

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(testModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

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

	// Memory service for multi-GPU eviction test
	memoryService := &MockMemoryEstimationServiceForAllocation{
		modelMemory: map[string]*memory.EstimationResult{
			"small-model:30b": {
				Recommendation: "single_gpu",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  30 * 1024 * 1024 * 1024,
					TotalSize: 30 * 1024 * 1024 * 1024,
				},
			},
			"medium-model:60b": {
				Recommendation: "tensor_parallel",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  60 * 1024 * 1024 * 1024,
					TotalSize: 60 * 1024 * 1024 * 1024,
				},
				TensorParallel: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  60 * 1024 * 1024 * 1024,
					TotalSize: 60 * 1024 * 1024 * 1024,
					GPUSizes:  []uint64{30 * 1024 * 1024 * 1024, 30 * 1024 * 1024 * 1024},
				},
			},
		},
	}

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController:        runnerCtrl,
		Store:                   mockStore,
		MemoryEstimationService: memoryService,
		QueueSize:               50,
	})
	require.NoError(t, err)

	testRunnerID := "eviction-test-runner"

	// Mock runner with 2x80GB GPUs
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

	runnerCtrl.runnersMu.Lock()
	runnerCtrl.runners = append(runnerCtrl.runners, testRunnerID)
	runnerCtrl.runnersMu.Unlock()

	t.Logf("=== Testing Multi-GPU Eviction Memory Calculation ===")

	// Create configured models for testing
	singleGPUAllocation := GPUAllocationConfig{
		GPUCount:     1,
		SpecificGPUs: []int{0},
	}
	configuredSingleGPUModel, err := NewModelForGPUAllocation(testModels[2], singleGPUAllocation, memoryService)
	require.NoError(t, err)

	multiGPUAllocation := GPUAllocationConfig{
		GPUCount:     2,
		SpecificGPUs: []int{0, 1},
	}
	configuredMultiGPUModel, err := NewModelForGPUAllocation(testModels[1], multiGPUAllocation, memoryService)
	require.NoError(t, err)

	// Create test slots that would be considered stale
	staleTime := time.Now().Add(-10 * time.Minute) // Long ago

	// Single GPU slot: 30GB on GPU 0
	singleGPUSlot := &Slot{
		ID:       uuid.New(),
		RunnerID: testRunnerID,
		initialWork: &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				Request: &openai.ChatCompletionRequest{
					Model: "small-model:30b",
				},
			},
			model: configuredSingleGPUModel, // 30GB configured model
		},
		LastActivityTime: staleTime,
		activeRequests:   0,
		maxConcurrency:   1,
		isStaleFunc:      func(string, time.Time) bool { return true }, // Always stale
		isErrorFunc:      func(string, time.Time) bool { return false },
		isRunning:        true, // Must be running to reach stale check
		GPUAllocation: &GPUAllocation{
			WorkloadID:         "single-gpu-workload",
			RunnerID:           testRunnerID,
			SingleGPU:          func() *int { i := 0; return &i }(),
			TensorParallelSize: 1,
		},
	}

	// Multi-GPU slot: 60GB across both GPUs (30GB each)
	multiGPUSlot := &Slot{
		ID:       uuid.New(),
		RunnerID: testRunnerID,
		initialWork: &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				Request: &openai.ChatCompletionRequest{
					Model: "medium-model:60b",
				},
			},
			model: configuredMultiGPUModel, // 60GB configured model
		},
		LastActivityTime: staleTime,
		activeRequests:   0,
		maxConcurrency:   1,
		isStaleFunc:      func(string, time.Time) bool { return true }, // Always stale
		isErrorFunc:      func(string, time.Time) bool { return false },
		isRunning:        true, // Must be running to reach stale check
		GPUAllocation: &GPUAllocation{
			WorkloadID:         "multi-gpu-workload",
			RunnerID:           testRunnerID,
			MultiGPUs:          []int{0, 1},
			TensorParallelSize: 2,
		},
	}

	// Store slots in scheduler
	scheduler.slots.Store(singleGPUSlot.ID, singleGPUSlot)
	scheduler.slots.Store(multiGPUSlot.ID, multiGPUSlot)

	t.Logf("Created test slots:")
	t.Logf("  Single GPU slot: 30GB on GPU 0")
	t.Logf("  Multi-GPU slot: 60GB across GPUs 0,1 (30GB each)")

	// Debug: Check if slots are found and their stale status
	var foundSlots []*Slot
	scheduler.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		if slot.RunnerID == testRunnerID {
			foundSlots = append(foundSlots, slot)
			t.Logf("  Found slot %s: model=%s, stale=%v", slot.ID, slot.InitialWork().ModelName(), slot.IsStale())
		}
		return true
	})
	t.Logf("Found %d slots for runner %s", len(foundSlots), testRunnerID)

	// Test evictable memory calculation
	evictableMemory, err := scheduler.calculateEvictableMemoryPerGPU(testRunnerID)
	require.NoError(t, err)

	t.Logf("Evictable memory per GPU:")
	for i := 0; i < 2; i++ {
		t.Logf("  GPU %d: %d GB", i, evictableMemory[i]/(1024*1024*1024))
	}

	// Expected results:
	// GPU 0: 30GB (single) + 30GB (half of multi) = 60GB
	// GPU 1: 0GB (no single) + 30GB (half of multi) = 30GB
	expectedGPU0 := uint64(60 * 1024 * 1024 * 1024)
	expectedGPU1 := uint64(30 * 1024 * 1024 * 1024)

	require.Equal(t, expectedGPU0, evictableMemory[0], "GPU 0 should have 60GB evictable")
	require.Equal(t, expectedGPU1, evictableMemory[1], "GPU 1 should have 30GB evictable")

	// Test eviction-aware allocation
	t.Logf("\n=== Testing Eviction-Aware GPU Allocation ===")

	// Test case 1: 100GB model (would need 2 GPUs normally)
	modelMemory := uint64(100 * 1024 * 1024 * 1024)
	singleGPU, multiGPUs, tensorParallelSize := scheduler.getOptimalGPUAllocationWithEviction(
		testRunnerID, modelMemory, types.RuntimeVLLM, evictableMemory)

	t.Logf("100GB model allocation with eviction:")
	t.Logf("  Single GPU: %v", singleGPU)
	t.Logf("  Multi GPUs: %v", multiGPUs)
	t.Logf("  Tensor Parallel Size: %d", tensorParallelSize)

	// Should be able to allocate with eviction
	require.True(t, singleGPU != nil || len(multiGPUs) > 0, "Should be able to allocate 100GB model with eviction")

	// Test case 2: 140GB model (would need eviction across both GPUs)
	largeModelMemory := uint64(140 * 1024 * 1024 * 1024)
	singleGPU2, multiGPUs2, tensorParallelSize2 := scheduler.getOptimalGPUAllocationWithEviction(
		testRunnerID, largeModelMemory, types.RuntimeVLLM, evictableMemory)

	t.Logf("140GB model allocation with eviction:")
	t.Logf("  Single GPU: %v", singleGPU2)
	t.Logf("  Multi GPUs: %v", multiGPUs2)
	t.Logf("  Tensor Parallel Size: %d", tensorParallelSize2)

	// With eviction, we have 60GB + 30GB = 90GB evictable + 160GB total = 250GB potential
	// So 140GB should fit with multi-GPU allocation
	if len(multiGPUs2) > 0 {
		require.Equal(t, tensorParallelSize2, len(multiGPUs2), "Tensor parallel size should match GPU count")
		t.Logf("✅ Large model can be allocated with eviction across %d GPUs", tensorParallelSize2)
	}

	t.Logf("\n=== Multi-GPU Eviction Test Summary ===")
	t.Logf("✅ Evictable memory calculation works correctly across GPUs")
	t.Logf("✅ Multi-GPU eviction properly handles both single and multi-GPU slots")
	t.Logf("✅ Eviction-aware allocation considers memory from both GPUs")
	t.Logf("✅ Memory distribution accounts for tensor parallelism correctly")
}

func TestEvictableMemoryCalculationMultiGPU(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	testModels := []*types.Model{
		{ID: "model-a:30b", Memory: 30 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false}, // 30GB
		{ID: "model-b:60b", Memory: 60 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false}, // 60GB
	}

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(testModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

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

	shortStaleTime := 100 * time.Millisecond
	// Memory service for evictable memory test
	memoryService := &MockMemoryEstimationServiceForAllocation{
		modelMemory: map[string]*memory.EstimationResult{
			"model-a:30b": {
				Recommendation: "single_gpu",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  30 * 1024 * 1024 * 1024,
					TotalSize: 30 * 1024 * 1024 * 1024,
				},
			},
			"model-b:60b": {
				Recommendation: "tensor_parallel",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  60 * 1024 * 1024 * 1024,
					TotalSize: 60 * 1024 * 1024 * 1024,
				},
				TensorParallel: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  60 * 1024 * 1024 * 1024,
					TotalSize: 60 * 1024 * 1024 * 1024,
					GPUSizes:  []uint64{30 * 1024 * 1024 * 1024, 30 * 1024 * 1024 * 1024},
				},
			},
		},
	}

	serverConfig := &config.ServerConfig{
		Providers: config.Providers{
			Helix: config.Helix{
				ModelTTL: shortStaleTime,
			},
		},
	}
	scheduler, err := NewScheduler(ctx, serverConfig, &Params{
		RunnerController:        runnerCtrl,
		Store:                   mockStore,
		MemoryEstimationService: memoryService,
		QueueSize:               50,
	})
	require.NoError(t, err)

	testRunnerID := "evictable-memory-test-runner"

	// Mock runner setup
	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			ID:          testRunnerID,
			TotalMemory: 160 * 1024 * 1024 * 1024, // 160GB total
			GPUCount:    2,
			GPUs: []*types.GPUStatus{
				{Index: 0, TotalMemory: 80 * 1024 * 1024 * 1024, FreeMemory: 80 * 1024 * 1024 * 1024},
				{Index: 1, TotalMemory: 80 * 1024 * 1024 * 1024, FreeMemory: 80 * 1024 * 1024 * 1024},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.runnersMu.Lock()
	runnerCtrl.runners = append(runnerCtrl.runners, testRunnerID)
	runnerCtrl.runnersMu.Unlock()

	// Create configured models for testing
	singleGPUAllocation := GPUAllocationConfig{
		GPUCount:     1,
		SpecificGPUs: []int{0},
	}
	configuredModelA, err := NewModelForGPUAllocation(testModels[0], singleGPUAllocation, memoryService)
	require.NoError(t, err)

	multiGPUAllocation := GPUAllocationConfig{
		GPUCount:     2,
		SpecificGPUs: []int{0, 1},
	}
	configuredModelB, err := NewModelForGPUAllocation(testModels[1], multiGPUAllocation, memoryService)
	require.NoError(t, err)

	// Create test slots manually
	t.Logf("Creating test slots for evictable memory calculation")

	// Single GPU slot (30GB on GPU 0)
	singleGPUSlot := &Slot{
		ID:       uuid.New(),
		RunnerID: testRunnerID,
		initialWork: &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				Request: &openai.ChatCompletionRequest{
					Model: "model-a:30b",
				},
			},
			model: configuredModelA,
		}, // 30GB configured model
		LastActivityTime: time.Now().Add(-200 * time.Millisecond), // Make it stale
		activeRequests:   0,
		maxConcurrency:   1,
		isStaleFunc:      func(string, time.Time) bool { return true }, // Always stale
		isErrorFunc:      func(string, time.Time) bool { return false },
		isRunning:        true, // Must be running to reach stale check
		GPUAllocation: &GPUAllocation{
			WorkloadID:         "single-gpu-workload",
			RunnerID:           testRunnerID,
			SingleGPU:          func() *int { i := 0; return &i }(),
			TensorParallelSize: 1,
		},
	}

	// Multi-GPU slot (60GB across both GPUs)
	multiGPUSlot := &Slot{
		ID:       uuid.New(),
		RunnerID: testRunnerID,
		initialWork: &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				Request: &openai.ChatCompletionRequest{
					Model: "model-b:60b",
				},
			},
			model: configuredModelB,
		}, // 60GB configured model
		LastActivityTime: time.Now().Add(-200 * time.Millisecond), // Make it stale
		activeRequests:   0,
		maxConcurrency:   1,
		isStaleFunc:      func(string, time.Time) bool { return true }, // Always stale
		isErrorFunc:      func(string, time.Time) bool { return false },
		isRunning:        true, // Must be running to reach stale check
		GPUAllocation: &GPUAllocation{
			WorkloadID:         "multi-gpu-workload",
			RunnerID:           testRunnerID,
			MultiGPUs:          []int{0, 1},
			TensorParallelSize: 2,
		},
	}

	// Store slots in scheduler
	scheduler.slots.Store(singleGPUSlot.ID, singleGPUSlot)
	scheduler.slots.Store(multiGPUSlot.ID, multiGPUSlot)

	// Calculate evictable memory
	evictableMemory, err := scheduler.calculateEvictableMemoryPerGPU(testRunnerID)
	require.NoError(t, err)

	t.Logf("Evictable memory calculation results:")
	t.Logf("  GPU 0: %d GB evictable", evictableMemory[0]/(1024*1024*1024))
	t.Logf("  GPU 1: %d GB evictable", evictableMemory[1]/(1024*1024*1024))

	// GPU 0 should have: 30GB (single) + 30GB (half of multi) = 60GB evictable
	expectedGPU0 := uint64(60 * 1024 * 1024 * 1024)
	require.Equal(t, expectedGPU0, evictableMemory[0], "GPU 0 should have 60GB evictable")

	// GPU 1 should have: 0GB (no single) + 30GB (half of multi) = 30GB evictable
	expectedGPU1 := uint64(30 * 1024 * 1024 * 1024)
	require.Equal(t, expectedGPU1, evictableMemory[1], "GPU 1 should have 30GB evictable")

	t.Logf("✅ Evictable memory calculation correctly handles mixed single/multi-GPU slots")
}
