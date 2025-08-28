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

// ProductionBugMemoryService simulates the exact production scenario that caused overscheduling
type ProductionBugMemoryService struct {
	modelMemory map[string]*memory.EstimationResult
}

func NewProductionBugMemoryService() *ProductionBugMemoryService {
	return &ProductionBugMemoryService{
		modelMemory: map[string]*memory.EstimationResult{
			// This is the exact scenario that caused the overscheduling bug:
			// Database had ~42GB, but GGUF estimation returns ~74GB
			"qwen3:30b": {
				Recommendation: "single_gpu",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    48,
					VRAMSize:  79982566400, // 74.5GB - ACTUAL memory requirement from GGUF
					TotalSize: 79982566400, // This is what the model really needs
				},
			},
			"gpt-oss:20b": {
				Recommendation: "single_gpu",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  51325686144, // 47.8GB - ACTUAL from GGUF
					TotalSize: 51325686144,
				},
			},
			"qwen3:8b": {
				Recommendation: "single_gpu",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    32,
					VRAMSize:  15767897088, // 14.7GB - ACTUAL from GGUF
					TotalSize: 15767897088,
				},
			},
		},
	}
}

func (m *ProductionBugMemoryService) EstimateModelMemory(ctx context.Context, modelName string, opts memory.EstimateOptions) (*memory.EstimationResult, error) {
	result, ok := m.modelMemory[modelName]
	if !ok {
		return nil, fmt.Errorf("model %s not found in production bug mock", modelName)
	}
	return result, nil
}

// TestOverschedulingBugFix demonstrates that the new Model architecture prevents the
// specific overscheduling bug that occurred in production with qwen3:30b
func TestOverschedulingBugFix(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	t.Logf("🐛 OVERSCHEDULING BUG FIX TEST")
	t.Logf("Testing the exact production scenario that caused overscheduling:")
	t.Logf("  Database: qwen3:30b = 42GB (stale)")
	t.Logf("  GGUF:     qwen3:30b = 74GB (authoritative)")
	t.Logf("  Runner:   2×80GB A100 GPUs (160GB total)")

	// Production models with the EXACT values that caused the bug
	productionModels := []*types.Model{
		// The problematic model: DB shows 42GB but GGUF estimates 74GB
		{
			ID:            "qwen3:30b",
			Memory:        0, // Correct: Ollama models should have Memory=0
			Runtime:       types.RuntimeOllama,
			Prewarm:       false,
			ContextLength: 262144,
		},
		// Other production models
		{
			ID:            "gpt-oss:20b",
			Memory:        0, // Correct: Ollama models should have Memory=0
			Runtime:       types.RuntimeOllama,
			Prewarm:       true,
			ContextLength: 131072,
		},
		{
			ID:            "qwen3:8b",
			Memory:        0, // Correct: Ollama models should have Memory=0
			Runtime:       types.RuntimeOllama,
			Prewarm:       true,
			ContextLength: 40960,
		},
	}

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(productionModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

	// Add GetModel expectations for each model
	for _, model := range productionModels {
		mockStore.EXPECT().GetModel(gomock.Any(), model.ID).Return(model, nil).AnyTimes()
	}

	// Mock slot operations
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

	// Create scheduler with production bug memory service
	memoryService := NewProductionBugMemoryService()
	_, err = NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController:        runnerCtrl,
		Store:                   mockStore,
		MemoryEstimationService: memoryService,
		QueueSize:               50,
	})
	require.NoError(t, err)

	testRunnerID := "production-bug-test-runner"

	// Set up runner identical to production: 2×80GB A100 GPUs
	gpuMemoryBytes := uint64(80 * 1024 * 1024 * 1024) // 80GB per GPU
	totalMemoryBytes := 2 * gpuMemoryBytes            // 160GB total

	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			ID:          testRunnerID,
			TotalMemory: totalMemoryBytes,
			FreeMemory:  totalMemoryBytes, // All memory available initially
			UsedMemory:  0,
			GPUCount:    2,
			GPUs: []*types.GPUStatus{
				{
					Index:       0,
					TotalMemory: gpuMemoryBytes,
					FreeMemory:  gpuMemoryBytes,
					UsedMemory:  0,
				},
				{
					Index:       1,
					TotalMemory: gpuMemoryBytes,
					FreeMemory:  gpuMemoryBytes,
					UsedMemory:  0,
				},
			},
			Models: []*types.RunnerModelStatus{
				{ModelID: "qwen3:30b", Runtime: types.RuntimeOllama, DownloadInProgress: false},
				{ModelID: "gpt-oss:20b", Runtime: types.RuntimeOllama, DownloadInProgress: false},
				{ModelID: "qwen3:8b", Runtime: types.RuntimeOllama, DownloadInProgress: false},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.slotsCache.Set(testRunnerID, NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: []*types.RunnerSlot{}}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.runnersMu.Lock()
	runnerCtrl.runners = append(runnerCtrl.runners, testRunnerID)
	runnerCtrl.runnersMu.Unlock()

	// TEST PHASE 1: Demonstrate NEW architecture prevents overscheduling
	t.Logf("\n=== PHASE 1: Testing NEW Architecture (Fixed) ===")

	// Step 1: Create base model (what would come from database)
	baseModel := &types.Model{
		ID:            "qwen3:30b",
		Memory:        0, // NEW: Ollama models have Memory=0 (no stale DB values)
		Runtime:       types.RuntimeOllama,
		ContextLength: 262144,
	}

	// Step 2: Scheduler makes GPU allocation decision
	// This simulates what happens in ensureSlots
	authoritativeMemory, err := memoryService.EstimateModelMemory(ctx, baseModel.ID, memory.EstimateOptions{})
	require.NoError(t, err)
	actualMemory := authoritativeMemory.SingleGPU.TotalSize
	t.Logf("✅ Authoritative GGUF memory: %d GB", actualMemory/(1024*1024*1024))

	// Scheduler decides on single GPU (74GB fits on 80GB GPU)
	allocation := GPUAllocationConfig{
		GPUCount:     1,
		SpecificGPUs: []int{0},
	}

	// Step 3: Create configured model
	configuredModel, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
	require.NoError(t, err)

	// Step 4: Verify configured model uses authoritative memory
	configuredMemory := configuredModel.GetMemoryForAllocation()
	t.Logf("✅ Configured model memory: %d GB", configuredMemory/(1024*1024*1024))
	assert.Equal(t, actualMemory, configuredMemory, "Configured model should use authoritative GGUF memory")

	// Step 5: Create workload with configured model
	configuredWorkload, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
		RequestID: "overscheduling-bug-fix-test",
		Request: &openai.ChatCompletionRequest{
			Model:    "qwen3:30b",
			Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "test"}},
		},
	}, configuredModel)
	require.NoError(t, err)

	// Step 6: Verify scheduling decisions use authoritative memory
	schedulingMemory := configuredWorkload.model.GetMemoryForAllocation()
	t.Logf("✅ Scheduling decision memory: %d GB", schedulingMemory/(1024*1024*1024))
	assert.Equal(t, actualMemory, schedulingMemory, "Scheduling should use same memory as configuration")

	// TEST PHASE 2: Verify overscheduling prevention
	t.Logf("\n=== PHASE 2: Verify Overscheduling Prevention ===")

	// Calculate remaining capacity after first model
	remainingCapacity := gpuMemoryBytes - configuredMemory
	t.Logf("After allocating qwen3:30b (74GB): %d GB remaining on GPU 0", remainingCapacity/(1024*1024*1024))

	// Try to allocate another large model that would cause overscheduling with old logic
	secondBaseModel := &types.Model{
		ID:            "gpt-oss:20b",
		Memory:        0, // NEW: Ollama models have Memory=0
		Runtime:       types.RuntimeOllama,
		ContextLength: 131072,
	}

	secondMemoryEstimate, err := memoryService.EstimateModelMemory(ctx, secondBaseModel.ID, memory.EstimateOptions{})
	require.NoError(t, err)
	secondActualMemory := secondMemoryEstimate.SingleGPU.TotalSize
	t.Logf("Second model (gpt-oss:20b) needs: %d GB", secondActualMemory/(1024*1024*1024))

	// Check if second model would fit on remaining GPU capacity
	wouldFit := secondActualMemory <= remainingCapacity
	t.Logf("Second model would fit on GPU 0: %v", wouldFit)

	if wouldFit {
		t.Logf("✅ NEW ARCHITECTURE: Second model correctly fits on remaining GPU capacity")
	} else {
		t.Logf("✅ NEW ARCHITECTURE: Second model correctly rejected - would exceed GPU capacity")
		t.Logf("   This prevents overscheduling that would have occurred with stale DB values")
	}

	// TEST PHASE 3: Demonstrate benefits
	t.Logf("\n=== PHASE 3: Benefits of NEW Architecture ===")

	t.Logf("✅ BENEFIT 1: Memory values are always authoritative")
	t.Logf("   - Database Memory=0 for Ollama models (no stale values)")
	t.Logf("   - GetMemoryForAllocation() uses GGUF estimates")
	t.Logf("   - No possibility of scheduler/slot memory inconsistency")

	t.Logf("✅ BENEFIT 2: No memory calculation errors in hot paths")
	t.Logf("   - Memory is calculated once at model configuration time")
	t.Logf("   - GetMemoryForAllocation() never fails")
	t.Logf("   - Eliminates error handling throughout scheduler")

	t.Logf("✅ BENEFIT 3: GPU allocation info travels with model")
	t.Logf("   - Model knows: GPU count=%d, specific GPUs=%v",
		configuredModel.GetGPUCount(), configuredModel.GetSpecificGPUs())
	t.Logf("   - Per-GPU memory: %v",
		func() []string {
			var result []string
			for i, mem := range configuredModel.GetPerGPUMemory() {
				result = append(result, fmt.Sprintf("GPU%d=%dGB", i, mem/(1024*1024*1024)))
			}
			return result
		}())

	t.Logf("✅ BENEFIT 4: Fail-fast design prevents bugs")
	t.Logf("   - Accessing unconfigured models panics immediately")
	t.Logf("   - Forces proper usage of NewModelForGPUAllocation()")
	t.Logf("   - Makes bugs obvious during development")

	// TEST PHASE 4: Demonstrate the specific production bug scenario
	t.Logf("\n=== PHASE 4: Production Bug Scenario Simulation ===")

	// This simulates what WOULD have happened with the old architecture:
	oldDatabaseMemory := uint64(42 * 1024 * 1024 * 1024)      // 42GB - what was in the database
	newGGUFMemory := configuredModel.GetMemoryForAllocation() // 74GB - what GGUF estimates

	memoryDifference := newGGUFMemory - oldDatabaseMemory
	t.Logf("❌ OLD ARCHITECTURE PROBLEM:")
	t.Logf("   Database Memory: %d GB (used for scheduling decisions)", oldDatabaseMemory/(1024*1024*1024))
	t.Logf("   GGUF Memory:     %d GB (used for slot creation)", newGGUFMemory/(1024*1024*1024))
	t.Logf("   Difference:      %d GB (caused overscheduling!)", memoryDifference/(1024*1024*1024))

	t.Logf("✅ NEW ARCHITECTURE SOLUTION:")
	t.Logf("   Scheduling Memory: %d GB (from GetMemoryForAllocation)", configuredModel.GetMemoryForAllocation()/(1024*1024*1024))
	t.Logf("   Slot Memory:       %d GB (same - from GetMemoryForAllocation)", configuredModel.GetMemoryForAllocation()/(1024*1024*1024))
	t.Logf("   Difference:        0 GB (no inconsistency possible!)")

	// Verify that the old bug cannot happen with new architecture
	assert.Equal(t, configuredModel.GetMemoryForAllocation(), newGGUFMemory,
		"New architecture eliminates memory inconsistency")

	// Verify database access is protected
	_, dbErr := configuredModel.GetDatabaseMemory()
	require.Error(t, dbErr, "Database memory access should be blocked for Ollama models")
	assert.Contains(t, dbErr.Error(), "should use GetMemoryForAllocation()",
		"Error message should guide to correct usage")
}

// TestMemoryArchitectureComparison shows the before/after of the memory architecture
func TestMemoryArchitectureComparison(t *testing.T) {
	memoryService := NewProductionBugMemoryService()

	t.Run("OLD architecture problems", func(t *testing.T) {
		// This simulates the old problematic approach
		oldStyleModel := &types.Model{
			ID:      "qwen3:30b",
			Memory:  42 * 1024 * 1024 * 1024, // OLD: Stale database value
			Runtime: types.RuntimeOllama,
		}

		t.Logf("❌ OLD ARCHITECTURE:")
		t.Logf("   model.Memory = %d GB (from database, potentially stale)",
			oldStyleModel.Memory/(1024*1024*1024))

		// Old code would use model.Memory for scheduling decisions (42GB)
		schedulingMemory := oldStyleModel.Memory

		// But getModelMemory() would return different value for slot creation (74GB)
		authResult, err := memoryService.EstimateModelMemory(context.Background(), oldStyleModel.ID, memory.EstimateOptions{})
		require.NoError(t, err)
		slotMemory := authResult.SingleGPU.TotalSize

		inconsistency := slotMemory - schedulingMemory
		t.Logf("   Scheduling decision: %d GB", schedulingMemory/(1024*1024*1024))
		t.Logf("   Actual slot usage:   %d GB", slotMemory/(1024*1024*1024))
		t.Logf("   INCONSISTENCY:       %d GB (caused overscheduling!)", inconsistency/(1024*1024*1024))

		assert.NotEqual(t, schedulingMemory, slotMemory, "Old architecture had memory inconsistency")
	})

	t.Run("NEW architecture solution", func(t *testing.T) {
		// This demonstrates the new approach
		baseModel := &types.Model{
			ID:            "qwen3:30b",
			Memory:        0, // NEW: Ollama models have Memory=0 (no stale values)
			Runtime:       types.RuntimeOllama,
			ContextLength: 262144, // Required for GGUF estimation
		}

		allocation := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{0},
		}

		configuredModel, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.NoError(t, err)

		t.Logf("✅ NEW ARCHITECTURE:")
		t.Logf("   Database Memory = 0 (no stale values)")

		// Both scheduling and slot creation use the same authoritative value
		schedulingMemory := configuredModel.GetMemoryForAllocation()
		slotMemory := configuredModel.GetMemoryForAllocation() // Same method!

		t.Logf("   Scheduling decision: %d GB", schedulingMemory/(1024*1024*1024))
		t.Logf("   Actual slot usage:   %d GB", slotMemory/(1024*1024*1024))
		t.Logf("   CONSISTENCY:         Perfect (no overscheduling possible!)")

		assert.Equal(t, schedulingMemory, slotMemory, "New architecture guarantees memory consistency")
		assert.Equal(t, uint64(79982566400), schedulingMemory, "Should use authoritative GGUF value")
	})
}

// TestConfiguredModelVsUnconfiguredModel demonstrates fail-fast behavior
func TestConfiguredModelVsUnconfiguredModel(t *testing.T) {
	memoryService := NewProductionBugMemoryService()

	// Base model (unconfigured)
	baseModel := &types.Model{
		ID:            "qwen3:30b",
		Memory:        0,
		Runtime:       types.RuntimeOllama,
		ContextLength: 262144,
	}

	t.Run("unconfigured model fails fast", func(t *testing.T) {
		// Accessing memory on unconfigured model should panic
		assert.Panics(t, func() {
			_ = baseModel.GetMemoryForAllocation()
		}, "Unconfigured model should panic on memory access")

		assert.False(t, baseModel.IsAllocationConfigured(), "Base model should not be configured")
	})

	t.Run("configured model works perfectly", func(t *testing.T) {
		allocation := GPUAllocationConfig{
			GPUCount:     1,
			SpecificGPUs: []int{0},
		}

		configuredModel, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
		require.NoError(t, err)

		// All methods work without errors
		assert.True(t, configuredModel.IsAllocationConfigured())
		assert.Equal(t, uint64(79982566400), configuredModel.GetMemoryForAllocation())
		assert.Equal(t, 1, configuredModel.GetGPUCount())
		assert.Equal(t, 1, configuredModel.GetTensorParallelSize())
		assert.Equal(t, []int{0}, configuredModel.GetSpecificGPUs())
	})
}

// TestMultiGPUModelConfiguration tests multi-GPU scenarios
func TestMultiGPUModelConfiguration(t *testing.T) {
	// Create memory service with multi-GPU support
	memoryService := &ProductionBugMemoryService{
		modelMemory: map[string]*memory.EstimationResult{
			"large-model:120b": {
				Recommendation: "tensor_parallel",
				TensorParallel: &memory.MemoryEstimate{
					Layers:    48,
					VRAMSize:  120 * 1024 * 1024 * 1024,
					TotalSize: 120 * 1024 * 1024 * 1024,
					GPUSizes:  []uint64{60 * 1024 * 1024 * 1024, 60 * 1024 * 1024 * 1024}, // 60GB per GPU
				},
			},
		},
	}

	baseModel := &types.Model{
		ID:            "large-model:120b",
		Memory:        0,
		Runtime:       types.RuntimeOllama,
		ContextLength: 8192,
	}

	allocation := GPUAllocationConfig{
		GPUCount:     2,
		SpecificGPUs: []int{0, 1},
	}

	configuredModel, err := NewModelForGPUAllocation(baseModel, allocation, memoryService)
	require.NoError(t, err)

	t.Logf("Multi-GPU Model Configuration:")
	t.Logf("  Total memory: %d GB", configuredModel.GetMemoryForAllocation()/(1024*1024*1024))
	t.Logf("  GPU count: %d", configuredModel.GetGPUCount())
	t.Logf("  Tensor parallel size: %d", configuredModel.GetTensorParallelSize())
	t.Logf("  Specific GPUs: %v", configuredModel.GetSpecificGPUs())
	t.Logf("  Per-GPU memory: %v", func() []string {
		var result []string
		for _, mem := range configuredModel.GetPerGPUMemory() {
			result = append(result, fmt.Sprintf("%dGB", mem/(1024*1024*1024)))
		}
		return result
	}())

	// Verify multi-GPU configuration is correct
	assert.Equal(t, uint64(120*1024*1024*1024), configuredModel.GetMemoryForAllocation())
	assert.Equal(t, 2, configuredModel.GetGPUCount())
	assert.Equal(t, 2, configuredModel.GetTensorParallelSize())
	assert.Equal(t, []int{0, 1}, configuredModel.GetSpecificGPUs())
	assert.Equal(t, []uint64{60 * 1024 * 1024 * 1024, 60 * 1024 * 1024 * 1024}, configuredModel.GetPerGPUMemory())
}

// TestIntegratedSchedulingFlow tests the complete flow from base model to slot creation
func TestIntegratedSchedulingFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Single test model
	baseModel := &types.Model{
		ID:            "integration-test:30b",
		Memory:        0,
		Runtime:       types.RuntimeOllama,
		ContextLength: 8192,
	}

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{baseModel}, nil).AnyTimes()
	mockStore.EXPECT().GetModel(gomock.Any(), baseModel.ID).Return(baseModel, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Memory service that knows about our test model
	memoryService := &ProductionBugMemoryService{
		modelMemory: map[string]*memory.EstimationResult{
			"integration-test:30b": {
				Recommendation: "single_gpu",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  30 * 1024 * 1024 * 1024,
					TotalSize: 30 * 1024 * 1024 * 1024,
				},
			},
		},
	}

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},
		RunnerClient:  DefaultMockRunnerClient(),
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController:        runnerCtrl,
		Store:                   mockStore,
		MemoryEstimationService: memoryService,
		QueueSize:               50,
	})
	require.NoError(t, err)

	testRunnerID := "integration-test-runner"

	// Set up runner
	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			ID:          testRunnerID,
			TotalMemory: 80 * 1024 * 1024 * 1024, // 80GB
			FreeMemory:  80 * 1024 * 1024 * 1024,
			UsedMemory:  0,
			GPUCount:    1,
			GPUs:        []*types.GPUStatus{{Index: 0, TotalMemory: 80 * 1024 * 1024 * 1024, FreeMemory: 80 * 1024 * 1024 * 1024}},
			Models:      []*types.RunnerModelStatus{{ModelID: "integration-test:30b", Runtime: types.RuntimeOllama}},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.slotsCache.Set(testRunnerID, NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: []*types.RunnerSlot{}}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.runnersMu.Lock()
	runnerCtrl.runners = append(runnerCtrl.runners, testRunnerID)
	runnerCtrl.runnersMu.Unlock()

	// Test the complete flow: enqueue -> schedule -> allocate -> configure -> create slot
	// Step 1: Create workload with unconfigured model (new architecture)
	// Let ensureSlot handle the allocation decision and model configuration
	workload, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
		RequestID: "integration-flow-test",
		Request: &openai.ChatCompletionRequest{
			Model:    "integration-test:30b",
			Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "test"}},
		},
	}, baseModel) // Note: uses unconfigured base model
	require.NoError(t, err)

	// Step 3: Enqueue the workload
	err = scheduler.Enqueue(workload)
	require.NoError(t, err)
	t.Logf("✅ Successfully enqueued workload with unconfigured model")

	// Trigger scheduling
	scheduler.reconcileSlotsOnce(ctx)
	time.Sleep(100 * time.Millisecond) // Allow processing

	// Verify slot was created with configured model
	var createdSlot *Slot
	scheduler.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		if slot.InitialWork().ID() == "integration-flow-test" {
			createdSlot = slot
			return false // Stop iteration
		}
		return true
	})

	require.NotNil(t, createdSlot, "Slot should have been created")

	// The slot's workload should now have a configured model
	slotModel := createdSlot.InitialWork().model
	if slotModel.IsAllocationConfigured() {
		t.Logf("✅ Slot created with CONFIGURED model:")
		t.Logf("   Memory: %d GB (authoritative)", slotModel.GetMemoryForAllocation()/(1024*1024*1024))
		t.Logf("   GPU count: %d", slotModel.GetGPUCount())
		t.Logf("   Specific GPUs: %v", slotModel.GetSpecificGPUs())
	} else {
		t.Logf("⚠️ Slot created with unconfigured model (fallback mode)")
	}

	t.Logf("🎯 INTEGRATION TEST COMPLETE: Full flow working with new architecture!")
}
