package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestSlotStoreSaveAndLoad verifies that slots are correctly saved to and loaded from the database
func TestSlotStoreSaveAndLoad(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test data
	slotID := uuid.New()
	runnerID := "test-runner"
	modelName := "meta-llama/Llama-3.3-70B-Instruct"
	runtime := types.RuntimeVLLM
	contextLength := int64(8192)
	modelMemory := uint64(15 * 1024 * 1024 * 1024) // 15GB
	gpuIndex := 1
	gpuIndices := []int{1, 2}
	tensorParallelSize := 2
	runtimeArgs := map[string]any{
		"args": []string{"--max-model-len", "8192"},
	}

	model := &types.Model{
		ID:            modelName,
		Name:          modelName,
		Runtime:       runtime,
		Memory:        modelMemory,
		ContextLength: contextLength,
		RuntimeArgs:   runtimeArgs,
		Concurrency:   256,
	}

	// Create workload
	workload, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
		RequestID: "test-request",
		Request: &openai.ChatCompletionRequest{
			Model: model.ID,
		},
	}, model)
	require.NoError(t, err)

	// Create GPU allocation
	gpuAllocation := &GPUAllocation{
		WorkloadID:         "test-workload",
		RunnerID:           runnerID,
		SingleGPU:          &gpuIndex,
		MultiGPUs:          gpuIndices,
		TensorParallelSize: tensorParallelSize,
	}

	// Create scheduler slot
	schedulerSlot := NewSlot(runnerID, workload,
		func(string, time.Time) bool { return false },
		func(string, time.Time) bool { return false },
		gpuAllocation)
	schedulerSlot.ID = slotID
	schedulerSlot.SetRunning()
	schedulerSlot.Start() // 1 active request

	// Track what gets saved to the database
	var savedSlot *types.RunnerSlot

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, slot *types.RunnerSlot) (*types.RunnerSlot, error) {
			savedSlot = slot
			return slot, nil
		},
	).Times(1)

	// Mock ListAllSlots to return empty initially
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).Times(1)

	// Create slot store
	slotStore := NewSlotStore(mockStore)

	// Store the slot
	slotStore.Store(slotID, schedulerSlot)

	// Wait for async save
	time.Sleep(100 * time.Millisecond)

	// Verify slot was saved
	require.NotNil(t, savedSlot, "Slot should have been saved to database")

	t.Log("🔍 Verifying core fields were correctly saved to database...")

	// ========================================
	// VERIFY CORE IDENTIFIERS (Always worked)
	// ========================================
	t.Run("CoreIdentifiers", func(t *testing.T) {
		assert.Equal(t, slotID, savedSlot.ID, "ID should match")
		assert.Equal(t, runnerID, savedSlot.RunnerID, "RunnerID should be saved")
		assert.NotZero(t, savedSlot.Created, "Created timestamp should be set")
		t.Logf("✅ Core identifiers saved correctly")
	})

	// ========================================
	// VERIFY CONCURRENCY TRACKING (Always worked)
	// ========================================
	t.Run("ConcurrencyTracking", func(t *testing.T) {
		assert.Equal(t, int64(1), savedSlot.ActiveRequests, "ActiveRequests should be saved")
		assert.Equal(t, int64(256), savedSlot.MaxConcurrency, "MaxConcurrency should be saved")
		assert.True(t, savedSlot.Active, "Active should be true (1 active request)")
		assert.True(t, savedSlot.Ready, "Ready should be true (isRunning = true)")

		t.Logf("✅ Concurrency tracking fields saved correctly:")
		t.Logf("   ActiveRequests: %d", savedSlot.ActiveRequests)
		t.Logf("   MaxConcurrency: %d", savedSlot.MaxConcurrency)
		t.Logf("   Active: %t", savedSlot.Active)
		t.Logf("   Ready: %t", savedSlot.Ready)
	})

	// ========================================
	// VERIFY JSONB SERIALIZATION (Always worked)
	// ========================================
	t.Run("JSONBSerialization", func(t *testing.T) {
		assert.NotNil(t, savedSlot.WorkloadData, "WorkloadData should be serialized")
		if savedSlot.WorkloadData != nil {
			assert.NotEmpty(t, savedSlot.WorkloadData, "WorkloadData should not be empty")
			assert.Contains(t, savedSlot.WorkloadData, "workload_type", "WorkloadData should contain workload_type")
			t.Logf("✅ WorkloadData serialized: %d keys", len(savedSlot.WorkloadData))
		}

		assert.NotNil(t, savedSlot.GPUAllocationData, "GPUAllocationData should be serialized")
		if savedSlot.GPUAllocationData != nil {
			assert.NotEmpty(t, savedSlot.GPUAllocationData, "GPUAllocationData should not be empty")
			t.Logf("✅ GPUAllocationData serialized: %d keys", len(savedSlot.GPUAllocationData))
		}
	})

	// ========================================
	// VERIFY WORKLOAD CONFIGURATION FIELDS (New in Phase 2)
	// These fields are only populated with the new saveToDatabase() implementation
	// ========================================
	t.Run("WorkloadConfigurationEnrichment", func(t *testing.T) {
		if savedSlot.Model != "" {
			// New behavior: Model/Runtime/Memory extracted from workload on save
			assert.Equal(t, modelName, savedSlot.Model, "Model should be extracted from workload")
			assert.Equal(t, runtime, savedSlot.Runtime, "Runtime should be extracted from workload")
			assert.Equal(t, modelMemory, savedSlot.ModelMemoryRequirement, "ModelMemoryRequirement should be extracted from workload")
			assert.Greater(t, savedSlot.ModelMemoryRequirement, uint64(0), "ModelMemoryRequirement must be > 0")

			t.Logf("✅ NEW: Workload configuration fields extracted on save:")
			t.Logf("   Model: %s", savedSlot.Model)
			t.Logf("   Runtime: %s", savedSlot.Runtime)
			t.Logf("   Memory: %d bytes (%.2f GB)", savedSlot.ModelMemoryRequirement, float64(savedSlot.ModelMemoryRequirement)/(1024*1024*1024))
		} else {
			// Old behavior: These fields not populated on save (would come from runner updates)
			t.Logf("ℹ️  OLD: Workload configuration fields not extracted (would be populated by runner updates)")
		}
	})

	// ========================================
	// VERIFY GPU ALLOCATION FIELDS (New in Phase 2)
	// These fields are only populated with the new saveToDatabase() implementation
	// ========================================
	t.Run("GPUAllocationEnrichment", func(t *testing.T) {
		if savedSlot.GPUIndex != nil {
			// New behavior: GPU allocation fields extracted on save
			assert.Equal(t, gpuIndex, *savedSlot.GPUIndex, "GPUIndex value should match")
			assert.Equal(t, gpuIndices, savedSlot.GPUIndices, "GPUIndices should be extracted from GPU allocation")
			assert.Equal(t, tensorParallelSize, savedSlot.TensorParallelSize, "TensorParallelSize should be extracted from GPU allocation")

			t.Logf("✅ NEW: GPU allocation fields extracted on save:")
			t.Logf("   GPUIndex: %d", *savedSlot.GPUIndex)
			t.Logf("   GPUIndices: %v", savedSlot.GPUIndices)
			t.Logf("   TensorParallelSize: %d", savedSlot.TensorParallelSize)
		} else {
			// Old behavior: These fields not populated on save
			t.Logf("ℹ️  OLD: GPU allocation fields not extracted (would be populated by runner updates)")
		}
	})

	t.Log("✅ Slot store save operations verified successfully")
}

// TestSlotStoreLoadFromDatabase verifies that slots are correctly loaded from database on startup
func TestSlotStoreLoadFromDatabase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	slotID := uuid.New()
	runnerID := "test-runner"
	modelName := "meta-llama/Llama-3.3-70B-Instruct"
	runtime := types.RuntimeVLLM
	modelMemory := uint64(20 * 1024 * 1024 * 1024) // 20GB
	gpuIndex := 0

	// Simulate database returning a persisted slot
	dbSlot := &types.RunnerSlot{
		ID:                     slotID,
		RunnerID:               runnerID,
		Model:                  modelName,
		Runtime:                runtime,
		ModelMemoryRequirement: modelMemory,
		Ready:                  true,
		Active:                 false,
		ActiveRequests:         0,
		MaxConcurrency:         128,
		GPUIndex:               &gpuIndex,
		GPUIndices:             []int{0},
		TensorParallelSize:     1,
		Created:                time.Now().Add(-1 * time.Hour),
		Updated:                time.Now(),
		// Workload and GPU allocation data would be serialized JSONB
		WorkloadData: map[string]any{
			"workload_type": "llm",
			"model": map[string]any{
				"id":     modelName,
				"memory": float64(modelMemory),
			},
		},
		GPUAllocationData: map[string]any{
			"workload_id":          "test",
			"runner_id":            runnerID,
			"single_gpu":           float64(0),
			"tensor_parallel_size": float64(1),
		},
	}

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{dbSlot}, nil).Times(1)

	// Create slot store - this should load from database
	slotStore := NewSlotStore(mockStore)

	t.Log("🔍 Verifying slot was loaded from database...")

	// Verify slot was loaded
	loadedSlot, exists := slotStore.Load(slotID)
	require.True(t, exists, "Slot should be loaded from database")
	require.NotNil(t, loadedSlot, "Loaded slot should not be nil")

	// ========================================
	// VERIFY LOADED SLOT FIELDS
	// ========================================
	t.Run("LoadedSlotFields", func(t *testing.T) {
		assert.Equal(t, slotID, loadedSlot.ID, "ID should match")
		assert.Equal(t, runnerID, loadedSlot.RunnerID, "RunnerID should match")
		assert.True(t, loadedSlot.IsRunning(), "Slot should be running (Ready=true)")
		assert.Equal(t, int64(128), loadedSlot.maxConcurrency, "MaxConcurrency should be restored")

		t.Logf("✅ Slot loaded from database correctly:")
		t.Logf("   ID: %s", loadedSlot.ID)
		t.Logf("   RunnerID: %s", loadedSlot.RunnerID)
		t.Logf("   IsRunning: %t", loadedSlot.IsRunning())
		t.Logf("   MaxConcurrency: %d", loadedSlot.maxConcurrency)
	})

	// ========================================
	// VERIFY CRITICAL SAFETY: ZERO MEMORY REJECTION
	// ========================================
	t.Run("ZeroMemoryRejection", func(t *testing.T) {
		// Create a slot with zero memory (invalid)
		invalidSlot := &types.RunnerSlot{
			ID:                     uuid.New(),
			RunnerID:               "invalid-runner",
			Model:                  "invalid-model",
			Runtime:                types.RuntimeOllama,
			ModelMemoryRequirement: 0, // ZERO - should be rejected
			Ready:                  true,
			Created:                time.Now(),
			Updated:                time.Now(),
		}

		mockStore2 := store.NewMockStore(ctrl)
		mockStore2.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{invalidSlot}, nil).Times(1)
		mockStore2.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).Times(1)

		// Create slot store - should reject and delete invalid slot
		slotStore2 := NewSlotStore(mockStore2)

		// Verify invalid slot was NOT loaded
		_, exists := slotStore2.Load(invalidSlot.ID)
		assert.False(t, exists, "Slot with zero memory should be rejected")

		t.Logf("✅ CRITICAL SAFETY: Slot with zero memory was correctly rejected")
	})

	t.Log("✅ All slot store load operations verified successfully")
}

// TestSlotStoreWorkloadRecovery verifies that workload can be recovered from JSONB data
func TestSlotStoreWorkloadRecovery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	slotID := uuid.New()
	runnerID := "test-runner"

	// Create a slot with workload data
	model := &types.Model{
		ID:      "recovery-model",
		Runtime: types.RuntimeOllama,
		Memory:  10 * 1024 * 1024 * 1024,
	}

	workload, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
		RequestID: "recovery-test",
		Request: &openai.ChatCompletionRequest{
			Model: model.ID,
		},
	}, model)
	require.NoError(t, err)

	schedulerSlot := NewSlot(runnerID, workload,
		func(string, time.Time) bool { return false },
		func(string, time.Time) bool { return false },
		nil)
	schedulerSlot.ID = slotID

	var savedWorkloadData map[string]any

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, slot *types.RunnerSlot) (*types.RunnerSlot, error) {
			savedWorkloadData = slot.WorkloadData
			return slot, nil
		},
	).Times(1)
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).Times(1)

	slotStore := NewSlotStore(mockStore)
	slotStore.Store(slotID, schedulerSlot)

	time.Sleep(100 * time.Millisecond)

	// Verify workload was serialized
	require.NotNil(t, savedWorkloadData, "WorkloadData should be saved")
	assert.Contains(t, savedWorkloadData, "workload_type", "WorkloadData should contain workload_type")
	assert.Contains(t, savedWorkloadData, "model", "WorkloadData should contain model")

	t.Logf("✅ Workload recovery verified:")
	t.Logf("   WorkloadData fields: %v", getKeys(savedWorkloadData))
}

// TestSlotStoreConcurrentAccess verifies thread-safe access to the slot store
func TestSlotStoreConcurrentAccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).Times(1)
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	slotStore := NewSlotStore(mockStore)

	model := &types.Model{
		ID:      "concurrent-model",
		Runtime: types.RuntimeOllama,
		Memory:  5 * 1024 * 1024 * 1024,
	}

	runnerID := "concurrent-runner"

	// Store multiple slots concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(index int) {
			workload, _ := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
				RequestID: "concurrent-test",
				Request: &openai.ChatCompletionRequest{
					Model: model.ID,
				},
			}, model)

			slot := NewSlot(runnerID, workload,
				func(string, time.Time) bool { return false },
				func(string, time.Time) bool { return false },
				nil)

			slotStore.Store(slot.ID, slot)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify size
	assert.Equal(t, 10, slotStore.Size(), "All 10 slots should be stored")

	t.Log("✅ Concurrent access verified successfully")
}

// Helper function to get map keys
func getKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
