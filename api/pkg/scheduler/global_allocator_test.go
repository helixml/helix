package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/memory"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
)

// GlobalAllocatorMemoryService provides consistent memory estimates for global allocator tests
type GlobalAllocatorMemoryService struct {
	estimates map[string]*memory.EstimationResult
}

func NewGlobalAllocatorMemoryService() *GlobalAllocatorMemoryService {
	return &GlobalAllocatorMemoryService{
		estimates: map[string]*memory.EstimationResult{
			"small-ollama:7b": {
				Recommendation: "single_gpu",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    36,
					VRAMSize:  15 * 1024 * 1024 * 1024,
					TotalSize: 15 * 1024 * 1024 * 1024,
				},
			},
			"medium-ollama:20b": {
				Recommendation: "single_gpu",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    48,
					VRAMSize:  45 * 1024 * 1024 * 1024,
					TotalSize: 45 * 1024 * 1024 * 1024,
				},
			},
			"large-ollama:70b": {
				Recommendation: "tensor_parallel",
				SingleGPU: &memory.MemoryEstimate{
					Layers:    80,
					VRAMSize:  140 * 1024 * 1024 * 1024,
					TotalSize: 140 * 1024 * 1024 * 1024,
				},
				TensorParallel: &memory.MemoryEstimate{
					Layers:    80,
					VRAMSize:  70 * 1024 * 1024 * 1024, // Per GPU in tensor parallel
					TotalSize: 140 * 1024 * 1024 * 1024,
				},
			},
		},
	}
}

func (m *GlobalAllocatorMemoryService) EstimateModelMemory(ctx context.Context, modelName string, opts memory.EstimateOptions) (*memory.EstimationResult, error) {
	result, ok := m.estimates[modelName]
	if !ok {
		return nil, fmt.Errorf("model %s not found in global allocator memory service", modelName)
	}
	return result, nil
}

// createTestGlobalAllocator creates a test setup with global allocator
func createTestGlobalAllocator(t *testing.T) (*Scheduler, *RunnerController, *GlobalAllocator, string, context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	ctrl := gomock.NewController(t)

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Create test models
	testModels := []*types.Model{
		{ID: "small-vllm:7b", Runtime: types.RuntimeVLLM, Memory: 15 * 1024 * 1024 * 1024, ContextLength: 8192},
		{ID: "medium-vllm:20b", Runtime: types.RuntimeVLLM, Memory: 45 * 1024 * 1024 * 1024, ContextLength: 16384},
		{ID: "large-vllm:70b", Runtime: types.RuntimeVLLM, Memory: 140 * 1024 * 1024 * 1024, ContextLength: 32768},
		{ID: "small-ollama:7b", Runtime: types.RuntimeOllama, Memory: 0, ContextLength: 8192},
		{ID: "medium-ollama:20b", Runtime: types.RuntimeOllama, Memory: 0, ContextLength: 16384},
		{ID: "large-ollama:70b", Runtime: types.RuntimeOllama, Memory: 0, ContextLength: 32768},
	}

	// Convert test models to runner model status format
	runnerModels := make([]*types.RunnerModelStatus, len(testModels))
	for i, model := range testModels {
		runnerModels[i] = &types.RunnerModelStatus{
			ModelID:            model.ID,
			Runtime:            model.Runtime,
			DownloadInProgress: false,
		}
	}

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(testModels, nil).AnyTimes()

	for _, model := range testModels {
		mockStore.EXPECT().GetModel(gomock.Any(), model.ID).Return(model, nil).AnyTimes()
	}

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},
		RunnerClient:  NewMockRunnerClientWithModels(160, 2, runnerModels),
	})
	require.NoError(t, err)

	fastInterval := 100 * time.Millisecond
	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController:        runnerCtrl,
		Store:                   mockStore,
		MemoryEstimationService: NewGlobalAllocatorMemoryService(),
		QueueSize:               50,
		RunnerReconcileInterval: &fastInterval,
	})
	require.NoError(t, err)

	testRunnerID := "global-test-runner"

	// Set up runner with 2×80GB GPUs
	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			ID:          testRunnerID,
			TotalMemory: 160 * 1024 * 1024 * 1024, // 160GB total
			GPUCount:    2,
			GPUs: []*types.GPUStatus{
				{Index: 0, TotalMemory: 80 * 1024 * 1024 * 1024, FreeMemory: 80 * 1024 * 1024 * 1024},
				{Index: 1, TotalMemory: 80 * 1024 * 1024 * 1024, FreeMemory: 80 * 1024 * 1024 * 1024},
			},
			Models: func() []*types.RunnerModelStatus {
				var models []*types.RunnerModelStatus
				for _, model := range testModels {
					models = append(models, &types.RunnerModelStatus{
						ModelID: model.ID,
						Runtime: model.Runtime,
					})
				}
				return models
			}(),
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.OnConnectedHandler(testRunnerID)

	cleanup := func() {
		cancel()
		ctrl.Finish()
	}

	return scheduler, runnerCtrl, scheduler.GetGlobalAllocator(), testRunnerID, ctx, cleanup
}

func TestGlobalAllocator_BasicAllocation(t *testing.T) {
	_, _, allocator, testRunnerID, _, cleanup := createTestGlobalAllocator(t)
	defer cleanup()
	_ = testRunnerID // Suppress unused variable warning

	t.Run("single GPU allocation without eviction", func(t *testing.T) {
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "test-single-gpu",
				Request: &openai.ChatCompletionRequest{
					Model: "small-vllm:7b",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test prompt"},
					},
				},
			},
			model: &types.Model{
				ID:      "small-vllm:7b",
				Runtime: types.RuntimeVLLM,
				Memory:  15 * 1024 * 1024 * 1024, // 15GB
			},
		}

		// Test planning phase
		plan, err := allocator.PlanAllocation(workload)

		// Debug logging to understand plan selection
		if err != nil {
			t.Logf("❌ Planning failed: %v", err)
		} else {
			t.Logf("✅ Plan created: runner=%s, GPUs=%v, GPUCount=%d, IsMultiGPU=%t, Cost=%d",
				plan.RunnerID, plan.GPUs, plan.GPUCount, plan.IsMultiGPU, plan.Cost)
		}

		require.NoError(t, err)
		require.NotNil(t, plan)

		assert.Equal(t, testRunnerID, plan.RunnerID)
		assert.Equal(t, 1, plan.GPUCount)
		assert.False(t, plan.IsMultiGPU)
		assert.False(t, plan.RequiresEviction)
		assert.Len(t, plan.GPUs, 1)
		assert.Equal(t, uint64(15*1024*1024*1024), plan.TotalMemoryRequired)

		// Test execution phase
		slot, err := allocator.ExecuteAllocationPlan(plan, workload)
		require.NoError(t, err)
		require.NotNil(t, slot)

		assert.Equal(t, testRunnerID, slot.RunnerID)
		assert.NotNil(t, slot.GPUAllocation.SingleGPU)
		assert.Empty(t, slot.GPUAllocation.MultiGPUs)
	})

	t.Run("multi-GPU allocation for large model", func(t *testing.T) {
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "test-multi-gpu",
				Request: &openai.ChatCompletionRequest{
					Model: "large-vllm:70b",
				},
			},
			model: &types.Model{
				ID:      "large-vllm:70b",
				Runtime: types.RuntimeVLLM,
				Memory:  140 * 1024 * 1024 * 1024, // 140GB - requires 2 GPUs
			},
		}

		plan, err := allocator.PlanAllocation(workload)
		require.NoError(t, err)
		require.NotNil(t, plan)

		assert.Equal(t, testRunnerID, plan.RunnerID)
		assert.Equal(t, 2, plan.GPUCount)
		assert.True(t, plan.IsMultiGPU)
		assert.False(t, plan.RequiresEviction)
		assert.Len(t, plan.GPUs, 2)
		assert.Equal(t, uint64(140*1024*1024*1024), plan.TotalMemoryRequired)
		assert.Equal(t, uint64(70*1024*1024*1024), plan.MemoryPerGPU)

		slot, err := allocator.ExecuteAllocationPlan(plan, workload)
		require.NoError(t, err)
		require.NotNil(t, slot)

		assert.Equal(t, testRunnerID, slot.RunnerID)
		assert.Nil(t, slot.GPUAllocation.SingleGPU)
		assert.Len(t, slot.GPUAllocation.MultiGPUs, 2)
	})

	t.Run("ollama model with GGUF estimation", func(t *testing.T) {
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "test-ollama-gguf",
				Request: &openai.ChatCompletionRequest{
					Model: "medium-ollama:20b",
				},
			},
			model: &types.Model{
				ID:            "medium-ollama:20b",
				Runtime:       types.RuntimeOllama,
				Memory:        0, // Ollama models have Memory=0
				ContextLength: 16384,
			},
		}

		plan, err := allocator.PlanAllocation(workload)

		// Debug logging for Ollama model
		if err != nil {
			t.Logf("❌ Ollama planning failed: %v", err)
		} else {
			t.Logf("✅ Ollama plan created: runner=%s, GPUs=%v, GPUCount=%d, MemoryReq=%dGB",
				plan.RunnerID, plan.GPUs, plan.GPUCount, plan.TotalMemoryRequired/(1024*1024*1024))
		}

		require.NoError(t, err)
		require.NotNil(t, plan)

		assert.Equal(t, testRunnerID, plan.RunnerID)
		assert.Equal(t, 1, plan.GPUCount)
		assert.False(t, plan.IsMultiGPU)
		assert.Equal(t, uint64(45*1024*1024*1024), plan.TotalMemoryRequired) // From GGUF estimation
	})
}

func TestGlobalAllocator_LoadBalancing(t *testing.T) {
	scheduler, _, allocator, testRunnerID, _, cleanup := createTestGlobalAllocator(t)
	defer cleanup()
	_ = testRunnerID // Suppress unused variable warning

	t.Run("distributes models across GPUs", func(t *testing.T) {
		// Allocate first model
		workload1 := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "load-balance-1",
				Request:   &openai.ChatCompletionRequest{Model: "small-vllm:7b"},
			},
			model: &types.Model{
				ID: "small-vllm:7b", Runtime: types.RuntimeVLLM, Memory: 15 * 1024 * 1024 * 1024,
			},
		}

		slot1, err := allocator.AllocateWorkload(workload1, nil, nil)
		require.NoError(t, err)
		scheduler.slots.Store(slot1.ID, slot1)

		firstGPU := *slot1.GPUAllocation.SingleGPU

		// Allocate second model - should go to different GPU
		workload2 := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "load-balance-2",
				Request:   &openai.ChatCompletionRequest{Model: "small-vllm:7b"},
			},
			model: &types.Model{
				ID: "small-vllm:7b", Runtime: types.RuntimeVLLM, Memory: 15 * 1024 * 1024 * 1024,
			},
		}

		slot2, err := allocator.AllocateWorkload(workload2, nil, nil)
		require.NoError(t, err)

		secondGPU := *slot2.GPUAllocation.SingleGPU

		// Should distribute across different GPUs
		assert.NotEqual(t, firstGPU, secondGPU, "Models should be distributed across different GPUs")

		t.Logf("Load balancing result: Model 1 → GPU %d, Model 2 → GPU %d", firstGPU, secondGPU)
	})
}

func TestGlobalAllocator_EvictionLogic(t *testing.T) {
	scheduler, _, allocator, testRunnerID, _, cleanup := createTestGlobalAllocator(t)
	defer cleanup()
	_ = testRunnerID // Suppress unused variable warning

	t.Run("evicts stale slots when memory insufficient", func(t *testing.T) {
		// Fill both GPUs with stale slots
		staleWorkload1 := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "stale-1",
				Request: &openai.ChatCompletionRequest{
					Model: "stale-model-1",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "stale content"},
					},
				},
			},
			model: &types.Model{ID: "stale-model-1", Runtime: types.RuntimeVLLM, Memory: 70 * 1024 * 1024 * 1024},
		}

		configuredModel1, err := NewModelForGPUAllocation(staleWorkload1.model, GPUAllocationConfig{
			GPUCount: 1, SpecificGPUs: []int{0},
		}, allocator.memoryEstimationService)
		require.NoError(t, err)

		staleWorkload1.model = configuredModel1

		staleSlot1 := NewSlot(testRunnerID, staleWorkload1,
			func(string, time.Time) bool { return true }, // Always stale
			func(string, time.Time) bool { return false },
			&GPUAllocation{
				RunnerID:           testRunnerID,
				SingleGPU:          &[]int{0}[0],
				TensorParallelSize: 1,
			})
		staleSlot1.LastActivityTime = time.Now().Add(-1 * time.Hour) // Make it old
		staleSlot1.SetRunning()                                      // Make it running so it can be evaluated for staleness
		scheduler.slots.Store(staleSlot1.ID, staleSlot1)

		// Similar for GPU 1
		staleWorkload2 := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "stale-2",
				Request: &openai.ChatCompletionRequest{
					Model: "stale-model-2",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "stale content"},
					},
				},
			},
			model: &types.Model{ID: "stale-model-2", Runtime: types.RuntimeVLLM, Memory: 70 * 1024 * 1024 * 1024},
		}

		configuredModel2, err := NewModelForGPUAllocation(staleWorkload2.model, GPUAllocationConfig{
			GPUCount: 1, SpecificGPUs: []int{1},
		}, allocator.memoryEstimationService)
		require.NoError(t, err)

		staleWorkload2.model = configuredModel2

		staleSlot2 := NewSlot(testRunnerID, staleWorkload2,
			func(string, time.Time) bool { return true }, // Always stale
			func(string, time.Time) bool { return false },
			&GPUAllocation{
				RunnerID:           testRunnerID,
				SingleGPU:          &[]int{1}[0],
				TensorParallelSize: 1,
			})
		staleSlot2.LastActivityTime = time.Now().Add(-1 * time.Hour) // Make it old
		staleSlot2.SetRunning()                                      // Make it running so it can be evaluated for staleness
		scheduler.slots.Store(staleSlot2.ID, staleSlot2)

		// Now try to allocate a new model that requires eviction
		newWorkload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "new-model",
				Request: &openai.ChatCompletionRequest{
					Model: "medium-vllm:20b",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test content"},
					},
				},
			},
			model: &types.Model{ID: "medium-vllm:20b", Runtime: types.RuntimeVLLM, Memory: 45 * 1024 * 1024 * 1024},
		}

		plan, err := allocator.PlanAllocation(newWorkload)
		require.NoError(t, err)
		require.NotNil(t, plan)

		assert.True(t, plan.RequiresEviction, "Should require eviction")
		assert.Greater(t, len(plan.EvictionsNeeded), 0, "Should have slots to evict")
		assert.True(t, plan.IsValid, "Plan should be valid")

		// Execute the plan
		slot, err := allocator.ExecuteAllocationPlan(plan, newWorkload)
		require.NoError(t, err)
		require.NotNil(t, slot)

		// Verify eviction occurred - one of the stale slots should be gone
		_, exists1 := scheduler.slots.Load(staleSlot1.ID)
		_, exists2 := scheduler.slots.Load(staleSlot2.ID)

		evictionOccurred := !exists1 || !exists2
		assert.True(t, evictionOccurred, "At least one stale slot should have been evicted")
	})
}

func TestGlobalAllocator_OverschedulingPrevention(t *testing.T) {
	scheduler, _, allocator, testRunnerID, _, cleanup := createTestGlobalAllocator(t)
	defer cleanup()
	_ = testRunnerID // Suppress unused variable warning

	t.Run("prevents overscheduling single GPU", func(t *testing.T) {
		// Fill GPU 0 with a large model
		largeWorkload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "fill-gpu",
				Request: &openai.ChatCompletionRequest{
					Model: "medium-vllm:20b",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test content"},
					},
				},
			},
			model: &types.Model{ID: "medium-vllm:20b", Runtime: types.RuntimeVLLM, Memory: 70 * 1024 * 1024 * 1024},
		}

		slot1, err := allocator.AllocateWorkload(largeWorkload, nil, nil)
		require.NoError(t, err)
		scheduler.slots.Store(slot1.ID, slot1)

		firstGPU := *slot1.GPUAllocation.SingleGPU
		t.Logf("First model allocated to GPU %d", firstGPU)

		// Try to allocate another large model - should go to other GPU
		anotherWorkload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "second-large",
				Request: &openai.ChatCompletionRequest{
					Model: "medium-vllm:20b",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test content"},
					},
				},
			},
			model: &types.Model{ID: "medium-vllm:20b", Runtime: types.RuntimeVLLM, Memory: 70 * 1024 * 1024 * 1024},
		}

		slot2, err := allocator.AllocateWorkload(anotherWorkload, nil, nil)
		require.NoError(t, err)

		secondGPU := *slot2.GPUAllocation.SingleGPU
		assert.NotEqual(t, firstGPU, secondGPU, "Second model should use different GPU")

		// Try to allocate a third model that won't fit anywhere
		thirdWorkload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "overscheduling-test",
				Request: &openai.ChatCompletionRequest{
					Model: "huge-vllm:200b",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test content"},
					},
				},
			},
			model: &types.Model{ID: "huge-vllm:200b", Runtime: types.RuntimeVLLM, Memory: 200 * 1024 * 1024 * 1024},
		}

		_, err = allocator.PlanAllocation(thirdWorkload)
		assert.Error(t, err, "Should reject allocation that would cause overscheduling")
		assert.Contains(t, err.Error(), "no viable allocation plans", "Should indicate no viable plans")
	})
}

func TestGlobalAllocator_RuntimeSpecificBehavior(t *testing.T) {
	_, _, allocator, testRunnerID, _, cleanup := createTestGlobalAllocator(t)
	defer cleanup()
	_ = testRunnerID // Suppress unused variable warning

	t.Run("VLLM uses admin-configured memory", func(t *testing.T) {
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "vllm-memory-test",
			},
			model: &types.Model{
				ID:      "medium-vllm:20b",
				Runtime: types.RuntimeVLLM,
				Memory:  45 * 1024 * 1024 * 1024, // Admin-configured
			},
		}

		memReq, err := allocator.calculateMemoryRequirement(workload)
		require.NoError(t, err)
		assert.Equal(t, uint64(45*1024*1024*1024), memReq)
	})

	t.Run("Ollama uses GGUF estimation", func(t *testing.T) {
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "ollama-memory-test",
			},
			model: &types.Model{
				ID:            "medium-ollama:20b",
				Runtime:       types.RuntimeOllama,
				Memory:        0, // Ollama models have Memory=0
				ContextLength: 16384,
			},
		}

		memReq, err := allocator.calculateMemoryRequirement(workload)
		require.NoError(t, err)
		assert.Equal(t, uint64(45*1024*1024*1024), memReq) // From GGUF estimation
	})

	t.Run("rejects Ollama model with non-zero Memory", func(t *testing.T) {
		workload := &Workload{
			model: &types.Model{
				ID:            "invalid-ollama",
				Runtime:       types.RuntimeOllama,
				Memory:        100, // Should be 0 for Ollama
				ContextLength: 8192,
			},
		}

		_, err := allocator.calculateMemoryRequirement(workload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "should have Memory=0")
	})
}

func TestGlobalAllocator_ValidationAndEdgeCases(t *testing.T) {
	_, _, allocator, testRunnerID, _, cleanup := createTestGlobalAllocator(t)
	defer cleanup()
	_ = testRunnerID // Suppress unused variable warning

	t.Run("rejects nil workload", func(t *testing.T) {
		_, err := allocator.PlanAllocation(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid workload")
	})

	t.Run("rejects workload with nil model", func(t *testing.T) {
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			model:        nil,
		}

		_, err := allocator.PlanAllocation(workload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid workload")
	})

	t.Run("rejects unsupported runtime", func(t *testing.T) {
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			model: &types.Model{
				ID:      "unsupported-model",
				Runtime: "unsupported",
			},
		}

		_, err := allocator.calculateMemoryRequirement(workload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported runtime")
	})

	t.Run("handles missing GGUF estimation", func(t *testing.T) {
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			model: &types.Model{
				ID:            "missing-model",
				Runtime:       types.RuntimeOllama,
				Memory:        0,
				ContextLength: 8192,
			},
		}

		_, err := allocator.calculateMemoryRequirement(workload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in global allocator memory service")
	})
}

func TestGlobalAllocator_ValidationIntegrity(t *testing.T) {
	scheduler, _, allocator, testRunnerID, _, cleanup := createTestGlobalAllocator(t)
	defer cleanup()

	t.Run("validates allocation plans before execution", func(t *testing.T) {
		// Create a valid plan
		plan := &AllocationPlan{
			RunnerID:            testRunnerID,
			GPUs:                []int{0},
			GPUCount:            1,
			IsMultiGPU:          false,
			TotalMemoryRequired: 15 * 1024 * 1024 * 1024,
			MemoryPerGPU:        15 * 1024 * 1024 * 1024,
			RequiresEviction:    false,
			EvictionsNeeded:     nil,
			TensorParallelSize:  1,
			Runtime:             types.RuntimeVLLM,
			IsValid:             true,
		}

		err := allocator.validateAllocationPlan(plan)
		assert.NoError(t, err, "Valid plan should pass validation")
	})

	t.Run("rejects plan with invalid GPU index", func(t *testing.T) {
		plan := &AllocationPlan{
			RunnerID:            testRunnerID,
			GPUs:                []int{99}, // Invalid GPU index
			GPUCount:            1,
			TotalMemoryRequired: 15 * 1024 * 1024 * 1024,
			IsValid:             true,
		}

		err := allocator.validateAllocationPlan(plan)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "GPU 99 not found")
	})

	t.Run("detects overscheduling violations", func(t *testing.T) {
		// Create a slot that overschedules GPU 0
		badWorkload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "bad-workload",
				Request: &openai.ChatCompletionRequest{
					Model: "bad-model",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test content"},
					},
				},
			},
			model: &types.Model{ID: "bad-model", Runtime: types.RuntimeVLLM, Memory: 200 * 1024 * 1024 * 1024},
		}

		configuredBadModel, err := NewModelForGPUAllocation(badWorkload.model, GPUAllocationConfig{
			GPUCount: 1, SpecificGPUs: []int{0},
		}, allocator.memoryEstimationService)
		require.NoError(t, err)

		badWorkload.model = configuredBadModel

		badSlot := NewSlot(testRunnerID, badWorkload, nil, nil, &GPUAllocation{
			RunnerID:  testRunnerID,
			SingleGPU: &[]int{0}[0],
		})
		scheduler.slots.Store(badSlot.ID, badSlot)

		violations := allocator.ValidateNoOverscheduling()
		assert.Greater(t, len(violations), 0, "Should detect overscheduling violation")
		assert.Contains(t, violations[0], "allocated >", "Should describe overscheduling")

		// Clean up
		scheduler.slots.Delete(badSlot.ID)
	})
}

func TestGlobalAllocator_GlobalOptimization(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()

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
		MemoryEstimationService: NewGlobalAllocatorMemoryService(),
		QueueSize:               50,
	})
	require.NoError(t, err)
	_ = scheduler // Suppress unused variable warning

	allocator := scheduler.GetGlobalAllocator()

	// Set up multiple runners with different characteristics
	runner1ID := "runner-high-memory"
	runner2ID := "runner-low-memory"

	// Runner 1: 2×80GB GPUs (160GB total)
	runnerCtrl.statusCache.Set(runner1ID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			ID:          runner1ID,
			TotalMemory: 160 * 1024 * 1024 * 1024,
			GPUCount:    2,
			GPUs: []*types.GPUStatus{
				{Index: 0, TotalMemory: 80 * 1024 * 1024 * 1024, FreeMemory: 80 * 1024 * 1024 * 1024},
				{Index: 1, TotalMemory: 80 * 1024 * 1024 * 1024, FreeMemory: 80 * 1024 * 1024 * 1024},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Runner 2: 2×40GB GPUs (80GB total)
	runnerCtrl.statusCache.Set(runner2ID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			ID:          runner2ID,
			TotalMemory: 80 * 1024 * 1024 * 1024,
			GPUCount:    2,
			GPUs: []*types.GPUStatus{
				{Index: 0, TotalMemory: 40 * 1024 * 1024 * 1024, FreeMemory: 40 * 1024 * 1024 * 1024},
				{Index: 1, TotalMemory: 40 * 1024 * 1024 * 1024, FreeMemory: 40 * 1024 * 1024 * 1024},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.OnConnectedHandler(runner1ID)
	runnerCtrl.OnConnectedHandler(runner2ID)

	t.Run("selects optimal runner globally", func(t *testing.T) {
		// Large model should prefer high-memory runner
		largeWorkload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "large-workload",
				Request: &openai.ChatCompletionRequest{
					Model: "large-vllm:70b",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test content"},
					},
				},
			},
			model: &types.Model{
				ID: "large-vllm:70b", Runtime: types.RuntimeVLLM, Memory: 70 * 1024 * 1024 * 1024,
			},
		}

		plan, err := allocator.PlanAllocation(largeWorkload)
		require.NoError(t, err)
		require.NotNil(t, plan)

		assert.Equal(t, runner1ID, plan.RunnerID, "Large model should prefer high-memory runner")
		assert.Equal(t, 1, plan.GPUCount, "70GB model should fit on single 80GB GPU")
	})

	t.Run("distributes load across runners", func(t *testing.T) {
		// Fill runner 1 partially
		workload1 := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "workload1",
				Request: &openai.ChatCompletionRequest{
					Model: "medium-vllm:20b",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test content"},
					},
				},
			},
			model: &types.Model{ID: "medium-vllm:20b", Runtime: types.RuntimeVLLM, Memory: 60 * 1024 * 1024 * 1024},
		}

		slot1, err := allocator.AllocateWorkload(workload1, nil, nil)
		require.NoError(t, err)
		scheduler.slots.Store(slot1.ID, slot1)

		// Now small model should prefer runner 2 (less loaded)
		workload2 := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "workload2",
				Request: &openai.ChatCompletionRequest{
					Model: "small-vllm:7b",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test content"},
					},
				},
			},
			model: &types.Model{ID: "small-vllm:7b", Runtime: types.RuntimeVLLM, Memory: 30 * 1024 * 1024 * 1024},
		}

		plan2, err := allocator.PlanAllocation(workload2)
		require.NoError(t, err)

		// Should prefer less loaded runner (runner2) or different GPU on same runner
		// This tests global optimization vs local optimization
		t.Logf("Load balancing result: First model → %s, Second model plan → %s",
			slot1.RunnerID, plan2.RunnerID)
	})
}

func TestGlobalAllocator_ErrorHandling(t *testing.T) {
	_, _, allocator, _, _, cleanup := createTestGlobalAllocator(t)
	defer cleanup()

	t.Run("handles no runners available", func(t *testing.T) {
		// Create proper mocks
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ps, err := pubsub.NewInMemoryNats()
		require.NoError(t, err)

		mockStore := store.NewMockStore(ctrl)
		mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
		mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()

		// Create allocator with no runners
		emptyController, err := NewRunnerController(context.Background(), &RunnerControllerConfig{
			PubSub:        ps,
			Store:         mockStore,
			HealthChecker: &MockHealthChecker{},
			RunnerClient:  DefaultMockRunnerClient(),
		})
		require.NoError(t, err)

		emptyAllocator := NewGlobalAllocator(emptyController, NewGlobalAllocatorMemoryService(), NewSlotStore(mockStore), NewGlobalAllocationDecisionsTracker(10))

		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "error-test",
				Request: &openai.ChatCompletionRequest{
					Model: "test",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test content"},
					},
				},
			},
			model: &types.Model{ID: "test", Runtime: types.RuntimeVLLM, Memory: 10 * 1024 * 1024 * 1024},
		}

		_, err = emptyAllocator.PlanAllocation(workload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no runners available")
	})

	t.Run("handles model too large for any GPU", func(t *testing.T) {
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "impossible-model-test",
				Request: &openai.ChatCompletionRequest{
					Model: "impossible-model",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test content"},
					},
				},
			},
			model: &types.Model{
				ID: "impossible-model", Runtime: types.RuntimeVLLM, Memory: 500 * 1024 * 1024 * 1024, // 500GB
			},
		}

		_, err := allocator.PlanAllocation(workload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no viable allocation plans")
	})
}

func TestGlobalAllocator_MemoryCalculations(t *testing.T) {
	scheduler, _, allocator, testRunnerID, _, cleanup := createTestGlobalAllocator(t)
	defer cleanup()

	t.Run("accurately tracks memory across allocations", func(t *testing.T) {
		// Check initial state
		memoryState := allocator.GetGlobalMemoryState()
		initialGPU0 := memoryState[testRunnerID][0]
		initialGPU1 := memoryState[testRunnerID][1]

		assert.Equal(t, uint64(0), initialGPU0, "GPU 0 should start empty")
		assert.Equal(t, uint64(0), initialGPU1, "GPU 1 should start empty")

		// Allocate first model
		workload1 := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "memory-calc-1",
				Request: &openai.ChatCompletionRequest{
					Model: "first",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test content"},
					},
				},
			},
			model: &types.Model{ID: "first", Runtime: types.RuntimeVLLM, Memory: 30 * 1024 * 1024 * 1024},
		}

		slot1, err := allocator.AllocateWorkload(workload1, nil, nil)
		require.NoError(t, err)
		scheduler.slots.Store(slot1.ID, slot1)

		// Check updated state
		memoryState = allocator.GetGlobalMemoryState()

		selectedGPU := *slot1.GPUAllocation.SingleGPU
		otherGPU := 1 - selectedGPU

		assert.Equal(t, uint64(30*1024*1024*1024), memoryState[testRunnerID][selectedGPU],
			"Selected GPU should show allocated memory")
		assert.Equal(t, uint64(0), memoryState[testRunnerID][otherGPU],
			"Other GPU should remain empty")

		// Allocate second model - should use other GPU
		workload2 := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "memory-calc-2",
				Request: &openai.ChatCompletionRequest{
					Model: "second",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: "test content"},
					},
				},
			},
			model: &types.Model{ID: "second", Runtime: types.RuntimeVLLM, Memory: 40 * 1024 * 1024 * 1024},
		}

		slot2, err := allocator.AllocateWorkload(workload2, nil, nil)
		require.NoError(t, err)
		scheduler.slots.Store(slot2.ID, slot2)

		secondSelectedGPU := *slot2.GPUAllocation.SingleGPU

		// Verify load balancing worked
		assert.NotEqual(t, selectedGPU, secondSelectedGPU, "Second model should use different GPU")

		// Check final memory state
		memoryState = allocator.GetGlobalMemoryState()
		assert.Equal(t, uint64(30*1024*1024*1024), memoryState[testRunnerID][selectedGPU])
		assert.Equal(t, uint64(40*1024*1024*1024), memoryState[testRunnerID][secondSelectedGPU])
	})
}

func TestGlobalAllocator_IntegrationWithScheduler(t *testing.T) {
	scheduler, _, _, testRunnerID, _, cleanup := createTestGlobalAllocator(t)
	defer cleanup()

	t.Run("full integration test with ensureSlotWithGlobalAllocator", func(t *testing.T) {
		workload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: "integration-test",
				Request: &openai.ChatCompletionRequest{
					Model: "small-vllm:7b",
				},
			},
			model: &types.Model{
				ID: "small-vllm:7b", Runtime: types.RuntimeVLLM, Memory: 15 * 1024 * 1024 * 1024,
			},
		}

		req := SlotRequirement{
			ExampleWorkload: workload,
			Model:           workload.ModelName(),
			Runtime:         workload.Runtime(),
			Count:           1,
		}

		// Test the integrated function
		scheduler.ensureSlotWithGlobalAllocator(req)

		// Verify slot was created
		var createdSlot *Slot
		scheduler.slots.Range(func(id uuid.UUID, slot *Slot) bool {
			if slot.RunnerID == testRunnerID && slot.InitialWork().ModelName().String() == "small-vllm:7b" {
				createdSlot = slot
				return false
			}
			return true
		})

		require.NotNil(t, createdSlot, "Slot should be created by global allocator")
		assert.Equal(t, testRunnerID, createdSlot.RunnerID)
		assert.NotNil(t, createdSlot.GPUAllocation)

		// Should be single GPU allocation
		if createdSlot.GPUAllocation.SingleGPU != nil {
			assert.Contains(t, []int{0, 1}, *createdSlot.GPUAllocation.SingleGPU, "Should allocate to valid GPU")
		} else {
			t.Error("Expected single GPU allocation")
		}
	})
}

// TestGlobalAllocator_DecisionLogging verifies that global allocation decisions are logged
func TestGlobalAllocator_DecisionLogging(t *testing.T) {
	scheduler, _, allocator, testRunnerID, ctx, cleanup := createTestGlobalAllocator(t)
	defer cleanup()
	_ = ctx

	// Create a test workload
	workload := &Workload{
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			RequestID: "decision-log-test",
			Request: &openai.ChatCompletionRequest{
				Model: "test-model",
				Messages: []openai.ChatCompletionMessage{
					{Role: "user", Content: "test content"},
				},
			},
		},
		model: &types.Model{ID: "test-model", Runtime: types.RuntimeVLLM, Memory: 30 * 1024 * 1024 * 1024},
	}

	// Perform allocation which should log a decision
	slot, err := allocator.AllocateWorkload(workload, scheduler.modelStaleFunc, scheduler.slotTimeoutFunc)
	require.NoError(t, err)
	require.NotNil(t, slot)

	// Verify that a global allocation decision was logged
	decisions := scheduler.GetGlobalAllocationDecisions(10)
	require.Len(t, decisions, 1, "Expected exactly one global allocation decision to be logged")

	decision := decisions[0]
	assert.Equal(t, workload.ID(), decision.WorkloadID)
	assert.Equal(t, "test-model", decision.ModelName)
	assert.Equal(t, types.RuntimeVLLM, decision.Runtime)
	assert.True(t, decision.Success)
	assert.NotEmpty(t, decision.Reason)
	assert.GreaterOrEqual(t, decision.TotalTimeMs, int64(0))
	assert.Equal(t, 1, decision.TotalRunnersEvaluated)
	assert.Greater(t, decision.TotalPlansGenerated, 0)

	// Verify selected plan exists
	require.NotNil(t, decision.SelectedPlan)
	assert.Equal(t, testRunnerID, decision.SelectedPlan.RunnerID)
	assert.Greater(t, decision.SelectedPlan.GPUCount, 0)

	// Verify before and after states are captured
	assert.NotEmpty(t, decision.BeforeState)
	assert.NotEmpty(t, decision.AfterState)

	t.Logf("✅ Global allocation decision logged successfully: WorkloadID=%s, Success=%v, Time=%dms",
		decision.WorkloadID, decision.Success, decision.TotalTimeMs)
}
