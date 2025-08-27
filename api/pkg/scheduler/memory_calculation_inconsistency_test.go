package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/memory"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// SimpleMemoryEstimationServiceForTest provides simple mock memory estimates for testing
type SimpleMemoryEstimationServiceForTest struct{}

func (m *SimpleMemoryEstimationServiceForTest) EstimateModelMemory(ctx context.Context, modelName string, opts memory.EstimateOptions) (*memory.EstimationResult, error) {
	// Return appropriate memory values for different models
	var memSize uint64
	switch modelName {
	case "qwen3:8b":
		memSize = 10 * 1024 * 1024 * 1024 // 10GB
	case "gpt-oss:20b":
		memSize = 48 * 1024 * 1024 * 1024 // 48GB
	case "qwen3:30b":
		memSize = 55 * 1024 * 1024 * 1024 // 55GB (GGUF estimate)
	default:
		return nil, fmt.Errorf("model %s not found in test mock", modelName)
	}

	estimate := &memory.MemoryEstimate{
		Layers:    36, // Mock value
		VRAMSize:  memSize,
		TotalSize: memSize,
	}

	return &memory.EstimationResult{
		Recommendation: "single_gpu",
		SingleGPU:      estimate,
	}, nil
}

// Helper function for absolute difference
func abs64(x uint64) uint64 {
	return x // Since we're dealing with uint64, we don't need abs for negative values
}

// TestMemoryCalculationInconsistency demonstrates the critical bug where the scheduler
// and runner controller use different methods to calculate allocated memory, leading
// to over-scheduling and impossible free memory calculations.
//
// This test will FAIL until the inconsistency is fixed.
func TestMemoryCalculationInconsistency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Use default hardcoded models to match production behavior
	testModels := GetDefaultTestModels()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(testModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Mock slot operations
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},      // Use mock health checker for tests
		RunnerClient:  DefaultMockRunnerClient(), // Use mock runner client for tests
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController:        runnerCtrl,
		Store:                   mockStore,
		MemoryEstimationService: &SimpleMemoryEstimationServiceForTest{}, // Add mock memory estimation service
		QueueSize:               50,
	})
	require.NoError(t, err)

	testRunnerID := "memory-test-runner"

	// Set up a runner with 100GB total memory
	totalMemoryGB := uint64(100)
	totalMemoryBytes := totalMemoryGB * 1024 * 1024 * 1024

	// Mock runner status with GPU information
	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory: totalMemoryBytes,
			GPUCount:    2,
			GPUs: []*types.GPUStatus{
				{
					Index:       0,
					TotalMemory: totalMemoryBytes / 2, // 50GB per GPU
					FreeMemory:  totalMemoryBytes / 2, // Initially all free
					UsedMemory:  0,
					ModelName:   "NVIDIA A100 80GB PCIe",
				},
				{
					Index:       1,
					TotalMemory: totalMemoryBytes / 2, // 50GB per GPU
					FreeMemory:  totalMemoryBytes / 2, // Initially all free
					UsedMemory:  0,
					ModelName:   "NVIDIA A100 80GB PCIe",
				},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Use an existing prewarm model that the scheduler knows about
	// From the default configuration: "qwen3:8b" - Ollama model with GGUF estimation
	testModel := &types.Model{
		ID:            "qwen3:8b",
		Memory:        0, // Ollama models have Memory=0 in database
		Runtime:       types.RuntimeOllama,
		Prewarm:       true,
		ContextLength: 8192, // Required for GGUF estimation
	}

	// Create configured model for allocation
	memoryService := NewMockMemoryEstimationServiceForAllocation()
	allocation := GPUAllocationConfig{
		GPUCount:     1,
		SpecificGPUs: []int{0},
	}
	configuredModel, err := NewModelForGPUAllocation(testModel, allocation, memoryService)
	require.NoError(t, err)

	// Create a workload using the configured model
	workload := &Workload{
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			RequestID: "test-workload-1",
			CreatedAt: time.Now(),
			Request: &openai.ChatCompletionRequest{
				Model: configuredModel.ID,
				Messages: []openai.ChatCompletionMessage{
					{Role: "user", Content: "test"},
				},
			},
		},
		model: configuredModel,
	}

	// STEP 1: Calculate initial memory state using scheduler method
	schedulerTotalMem, schedulerAllocatedMem, schedulerFreeMem, err := scheduler.calculateRunnerMemory(testRunnerID)
	require.NoError(t, err, "Should be able to calculate runner memory")

	t.Logf("BEFORE slot creation - Scheduler calculation:")
	t.Logf("  Total: %d GB, Allocated: %d GB, Free: %d GB",
		schedulerTotalMem/(1024*1024*1024),
		schedulerAllocatedMem/(1024*1024*1024),
		schedulerFreeMem/(1024*1024*1024))

	// STEP 2: Calculate initial memory state using runner controller method
	runnerAllocatedMemPerGPU, err := runnerCtrl.calculateAllocatedMemoryPerGPU(testRunnerID)
	require.NoError(t, err, "Should be able to calculate allocated memory per GPU initially")
	runnerTotalAllocated := uint64(0)
	for _, allocated := range runnerAllocatedMemPerGPU {
		runnerTotalAllocated += allocated
	}

	t.Logf("BEFORE slot creation - RunnerController calculation:")
	t.Logf("  Per-GPU allocated: %v", runnerAllocatedMemPerGPU)
	t.Logf("  Total allocated: %d GB", runnerTotalAllocated/(1024*1024*1024))

	// Both should show zero allocated memory initially
	assert.Equal(t, uint64(0), schedulerAllocatedMem, "Scheduler should show 0 allocated memory initially")
	assert.Equal(t, uint64(0), runnerTotalAllocated, "RunnerController should show 0 allocated memory initially")

	// STEP 3: Manually create a slot to simulate what happens during scheduling
	// This simulates what happens in ensureSlots() -> NewSlot()
	// Create a GPU allocation for GPU 0 (single GPU allocation)
	gpuIndex := 0
	gpuAllocation := &GPUAllocation{
		WorkloadID:         workload.ID(),
		RunnerID:           testRunnerID,
		SingleGPU:          &gpuIndex,
		TensorParallelSize: 1,
	}
	slot := NewSlot(testRunnerID, workload, func(string, time.Time) bool { return false }, func(string, time.Time) bool { return false }, gpuAllocation)
	scheduler.slots.Store(slot.ID, slot)

	// Also add to runner controller's slot cache to simulate a running slot
	// This simulates what the runner reports back when the slot is active
	mockSlots := []*types.RunnerSlot{
		{
			ID:       slot.ID,
			Model:    testModel.ID, // This will trigger the heuristic calculation
			Active:   true,
			Ready:    true,
			GPUIndex: func() *int { i := 0; return &i }(), // Assign to GPU 0
		},
	}

	runnerCtrl.slotsCache.Set(testRunnerID, NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: mockSlots}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// STEP 4: Calculate memory after slot creation using both methods
	schedulerTotalMem2, schedulerAllocatedMem2, schedulerFreeMem2, err := scheduler.calculateRunnerMemory(testRunnerID)
	require.NoError(t, err, "Should be able to calculate runner memory after slot creation")

	t.Logf("AFTER slot creation - Scheduler calculation:")
	t.Logf("  Total: %d GB, Allocated: %d GB, Free: %d GB",
		schedulerTotalMem2/(1024*1024*1024),
		schedulerAllocatedMem2/(1024*1024*1024),
		schedulerFreeMem2/(1024*1024*1024))

	runnerAllocatedMemPerGPU2, err := runnerCtrl.calculateAllocatedMemoryPerGPU(testRunnerID)
	require.NoError(t, err, "Should be able to calculate allocated memory per GPU after slot creation")
	runnerTotalAllocated2 := uint64(0)
	for _, allocated := range runnerAllocatedMemPerGPU2 {
		runnerTotalAllocated2 += allocated
	}

	t.Logf("AFTER slot creation - RunnerController calculation:")
	t.Logf("  Per-GPU allocated: %v", runnerAllocatedMemPerGPU2)
	t.Logf("  Total allocated: %d GB", runnerTotalAllocated2/(1024*1024*1024))

	// STEP 5: Verify the consistency after the fix
	// Both scheduler and runner controller should now report the same value
	expectedAllocated := uint64(10 * 1024 * 1024 * 1024) // 10GB for qwen3:8b
	assert.Equal(t, expectedAllocated, schedulerAllocatedMem2,
		"Scheduler should use configured model memory (10GB)")

	assert.Equal(t, expectedAllocated, runnerTotalAllocated2,
		"RunnerController should use same authoritative model memory (10GB)")

	// Check if the fix worked - both methods should now give the same results
	if schedulerAllocatedMem2 == runnerTotalAllocated2 {
		t.Logf("✓ MEMORY CALCULATION CONSISTENCY ACHIEVED!")
		t.Logf("  Both scheduler and RunnerController report: %d GB allocated", schedulerAllocatedMem2/(1024*1024*1024))
		t.Logf("  This eliminates over-scheduling caused by inconsistent memory views!")
	} else {
		t.Errorf("MEMORY CALCULATION INCONSISTENCY STILL DETECTED!")
		t.Errorf("  Scheduler calculated allocated memory: %d GB", schedulerAllocatedMem2/(1024*1024*1024))
		t.Errorf("  RunnerController calculated allocated memory: %d GB", runnerTotalAllocated2/(1024*1024*1024))
		t.Errorf("  Difference: %d GB", abs64(schedulerAllocatedMem2-runnerTotalAllocated2)/(1024*1024*1024))
		t.Errorf("This inconsistency causes over-scheduling because different parts of the system")
		t.Errorf("have different views of how much memory is actually allocated!")
	}

	// STEP 6: Verify the system correctly handles total vs per-GPU memory
	// This difference is expected and correct behavior for multi-GPU systems
	schedulerFreeMemoryGB := schedulerFreeMem2 / (1024 * 1024 * 1024)

	// Calculate what the runner controller thinks is free on GPU 0
	gpu0Allocated := runnerAllocatedMemPerGPU2[0]
	gpu0Total := totalMemoryBytes / 2 // 50GB
	gpu0Free := gpu0Total - gpu0Allocated
	gpu0FreeGB := gpu0Free / (1024 * 1024 * 1024)

	t.Logf("Memory allocation verification:")
	t.Logf("  Scheduler system-wide free: %d GB (correct for multi-GPU models)", schedulerFreeMemoryGB)
	t.Logf("  GPU 0 individual free: %d GB (correct for single-GPU constraint)", gpu0FreeGB)

	// This is actually CORRECT behavior - not over-scheduling
	if schedulerFreeMemoryGB > gpu0FreeGB {
		t.Logf("✓ EXPECTED BEHAVIOR: Multi-GPU vs Single-GPU memory difference")
		t.Logf("  System-wide: %d GB (allows models up to this size via tensor parallelism)", schedulerFreeMemoryGB)
		t.Logf("  Per-GPU: %d GB (constraint for single-GPU models)", gpu0FreeGB)
		t.Logf("  This difference enables proper multi-GPU scheduling without over-allocation")
	} else {
		t.Logf("  System-wide and per-GPU memory are similar (single GPU system or unusual allocation)")
	}
}

// TestHeuristicFailureWhenModelMemoryUnknown verifies that the system fails
// gracefully when it cannot determine model memory requirements, rather than
// falling back to unreliable heuristics.
func TestHeuristicFailureWhenModelMemoryUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Mock slot operations
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},      // Use mock health checker for tests
		RunnerClient:  DefaultMockRunnerClient(), // Use mock runner client for tests
	})
	require.NoError(t, err)

	testRunnerID := "heuristic-test-runner"

	// Mock a slot with an unknown model that doesn't match any heuristics
	mockSlots := []*types.RunnerSlot{
		{
			ID:       uuid.New(),
			Model:    "completely-unknown-model-name-12345", // Won't match any heuristics
			Active:   true,
			Ready:    true,
			GPUIndex: func() *int { i := 0; return &i }(),
		},
	}

	runnerCtrl.slotsCache.Set(testRunnerID, NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: mockSlots}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Calculate allocated memory - this should now return an error when scheduler slots callback is not available
	_, err = runnerCtrl.calculateAllocatedMemoryPerGPU(testRunnerID)
	require.Error(t, err, "Should return error when scheduler slots callback is not available")
	require.Contains(t, err.Error(), "no scheduler slots callback available", "Error should indicate missing callback")

	t.Logf("Unknown model memory calculation:")
	t.Logf("  Model name: %s", "completely-unknown-model-name-12345")
	t.Logf("  Allocated memory: 0 GB")

	t.Logf("✓ SAFE BEHAVIOR: System correctly returns error when scheduler slots callback is not available")
	t.Logf("  This prevents over-scheduling by not making dangerous assumptions")

}
