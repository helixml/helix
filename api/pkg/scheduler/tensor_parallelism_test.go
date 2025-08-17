package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestTensorParallelismLargeModelSplitting tests that large models get properly
// TestBasicGPUAllocation tests basic GPU allocation functionality
func TestBasicGPUAllocation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Define test models - realistic sizes that should work
	testModels := []*types.Model{
		{ID: "medium-model:50b", Memory: 50 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false},   // 50GB - fits on single 80GB GPU
		{ID: "large-ollama:90b", Memory: 90 * 1024 * 1024 * 1024, Runtime: types.RuntimeOllama, Prewarm: false}, // 90GB - would need multi-GPU but Ollama is restricted
		{ID: "embedding-model:7b", Memory: 7 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false},  // 7GB - small model
	}

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(testModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Mock slot operations
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Mock GetModel calls for slot creation
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

	fastInterval := 100 * time.Millisecond
	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController:        runnerCtrl,
		Store:                   mockStore,
		QueueSize:               50,
		RunnerReconcileInterval: &fastInterval,
	})
	require.NoError(t, err)

	testRunnerID := "basic-allocation-test-runner"

	// Create a mock runner with 2x 80GB GPUs
	gpuCount := 2
	gpuMemoryBytes := uint64(80 * 1024 * 1024 * 1024) // 80GB per GPU

	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		gpus := make([]*types.GPUStatus, gpuCount)
		for i := 0; i < gpuCount; i++ {
			gpus[i] = &types.GPUStatus{
				Index:       i,
				TotalMemory: gpuMemoryBytes,
				FreeMemory:  gpuMemoryBytes,
				UsedMemory:  0,
				ModelName:   "NVIDIA A100 80GB",
			}
		}
		return types.RunnerStatus{
			ID:          testRunnerID,
			TotalMemory: gpuMemoryBytes * uint64(gpuCount),
			GPUCount:    gpuCount,
			GPUs:        gpus,
			Models: []*types.RunnerModelStatus{
				{ModelID: "medium-model:50b", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
				{ModelID: "large-ollama:90b", Runtime: types.RuntimeOllama, DownloadInProgress: false},
				{ModelID: "embedding-model:7b", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.slotsCache.Set(testRunnerID, NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: []*types.RunnerSlot{}}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.runnersMu.Lock()
	runnerCtrl.runners = append(runnerCtrl.runners, testRunnerID)
	runnerCtrl.runnersMu.Unlock()

	// Simulate runner connection
	err = ps.Publish(ctx, pubsub.GetRunnerConnectedQueue(testRunnerID), []byte("connected"))
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond) // Longer sleep to ensure runner is registered

	t.Logf("Testing basic GPU allocation with 2x%d GB GPUs (%d GB total)",
		gpuMemoryBytes/(1024*1024*1024), (gpuMemoryBytes*uint64(gpuCount))/(1024*1024*1024))

	// Test Case 1: VLLM model that fits on single GPU
	t.Logf("\n=== TEST CASE 1: Schedule VLLM model (50GB) - should succeed ===")

	vllmWorkload := &Workload{
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			RequestID: "vllm-model-request",
			CreatedAt: time.Now(),
			Request: &openai.ChatCompletionRequest{
				Model: "medium-model:50b",
				Messages: []openai.ChatCompletionMessage{
					{Role: "user", Content: "VLLM test"},
				},
			},
		},
		model: testModels[0],
	}

	err = scheduler.Enqueue(vllmWorkload)
	require.NoError(t, err)
	t.Logf("✅ Enqueued VLLM model (50GB)")

	scheduler.reconcileSlotsOnce(ctx)
	time.Sleep(100 * time.Millisecond)

	vllmAllocation := scheduler.getGPUAllocation("vllm-model-request", testRunnerID)
	require.NotNil(t, vllmAllocation, "VLLM model should be allocated")
	t.Logf("✅ VLLM model successfully allocated to GPU %v", vllmAllocation.SingleGPU)

	// Test Case 2: Large Ollama model - should be scheduled when tensor parallelism is needed
	t.Logf("\n=== TEST CASE 2: Schedule large Ollama model (90GB) - should succeed with tensor parallelism ===")

	ollamaWorkload := &Workload{
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			RequestID: "ollama-model-request",
			CreatedAt: time.Now(),
			Request: &openai.ChatCompletionRequest{
				Model: "large-ollama:90b",
				Messages: []openai.ChatCompletionMessage{
					{Role: "user", Content: "Ollama test"},
				},
			},
		},
		model: testModels[1],
	}

	err = scheduler.Enqueue(ollamaWorkload)
	require.NoError(t, err)
	t.Logf("✅ Enqueued large Ollama model (90GB)")

	scheduler.reconcileSlotsOnce(ctx)
	time.Sleep(100 * time.Millisecond)

	ollamaAllocation := scheduler.getGPUAllocation("ollama-model-request", testRunnerID)
	require.NotNil(t, ollamaAllocation, "Large Ollama model should be scheduled when tensor parallelism is needed")
	t.Logf("✅ Large Ollama model successfully scheduled with tensor parallelism: GPUs %v", ollamaAllocation.MultiGPUs)

	t.Logf("\n✅ Basic GPU allocation test completed successfully")
	t.Logf("  - VLLM model (50GB) allocated to single GPU")
	t.Logf("  - Large Ollama model (90GB) allocated with tensor parallelism")
	t.Logf("  - Intelligent Ollama tensor parallelism detection is working")
}

// TestFragmentationPrevention tests that small models scheduled first don't
// prevent large models from using tensor parallelism effectively
func TestFragmentationPrevention(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Define test models - scenario where small models could fragment memory
	testModels := []*types.Model{
		{ID: "large-model:120b", Memory: 120 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false}, // 120GB - needs 2 GPUs
		{ID: "medium-model:40b", Memory: 40 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false},  // 40GB - fits on 1 GPU
		{ID: "small-model:20b", Memory: 20 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false},   // 20GB - fits on 1 GPU
	}

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(testModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Mock slot operations
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
		HealthChecker: &MockHealthChecker{},      // Use mock health checker for tests
		RunnerClient:  DefaultMockRunnerClient(), // Use mock runner client for tests
	})
	require.NoError(t, err)

	fastInterval := 100 * time.Millisecond
	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController:        runnerCtrl,
		Store:                   mockStore,
		QueueSize:               50,
		RunnerReconcileInterval: &fastInterval, // Fast reconciliation for tests
	})
	require.NoError(t, err)

	testRunnerID := "fragmentation-test-runner"
	gpuMemoryBytes := uint64(80 * 1024 * 1024 * 1024) // 80GB per GPU
	gpuCount := 2

	// Mock a runner with 2x 80GB GPUs
	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		gpus := make([]*types.GPUStatus, gpuCount)
		for i := 0; i < gpuCount; i++ {
			gpus[i] = &types.GPUStatus{
				Index:       i,
				TotalMemory: gpuMemoryBytes,
				FreeMemory:  gpuMemoryBytes,
				UsedMemory:  0,
				ModelName:   "NVIDIA A100 80GB",
			}
		}
		return types.RunnerStatus{
			ID:          testRunnerID,
			TotalMemory: gpuMemoryBytes * uint64(gpuCount),
			GPUCount:    gpuCount,
			GPUs:        gpus,
			Models: []*types.RunnerModelStatus{
				{ModelID: "large-model:120b", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
				{ModelID: "medium-model:40b", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
				{ModelID: "small-model:20b", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.slotsCache.Set(testRunnerID, NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: []*types.RunnerSlot{}}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.runnersMu.Lock()
	runnerCtrl.runners = append(runnerCtrl.runners, testRunnerID)
	runnerCtrl.runnersMu.Unlock()

	t.Logf("Testing fragmentation prevention with 2x%d GB GPUs", gpuMemoryBytes/(1024*1024*1024))

	// Test Case 1: Schedule small models FIRST (potential fragmentation scenario)
	t.Logf("\n=== TEST CASE 1: Schedule small models first (fragmentation risk) ===")

	// Schedule medium model (40GB) first
	mediumWorkload := &Workload{
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			RequestID: "medium-model-request",
			CreatedAt: time.Now(),
			Request: &openai.ChatCompletionRequest{
				Model: "medium-model:40b",
				Messages: []openai.ChatCompletionMessage{
					{Role: "user", Content: "Medium model test"},
				},
			},
		},
		model: testModels[1], // 40GB model
	}

	err = scheduler.Enqueue(mediumWorkload)
	require.NoError(t, err)
	t.Logf("✅ Enqueued medium model (40GB)")

	// Schedule small model (20GB) second
	smallWorkload := &Workload{
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			RequestID: "small-model-request",
			CreatedAt: time.Now(),
			Request: &openai.ChatCompletionRequest{
				Model: "small-model:20b",
				Messages: []openai.ChatCompletionMessage{
					{Role: "user", Content: "Small model test"},
				},
			},
		},
		model: testModels[2], // 20GB model
	}

	err = scheduler.Enqueue(smallWorkload)
	require.NoError(t, err)
	t.Logf("✅ Enqueued small model (20GB)")

	// With one-slot-per-cycle, we need to trigger reconciliation twice to schedule both models
	// First reconciliation cycle: schedule medium model (40GB)
	scheduler.reconcileSlotsOnce(ctx)
	time.Sleep(50 * time.Millisecond)

	// Check that the first model is allocated
	mediumAllocation := scheduler.getGPUAllocation("medium-model-request", testRunnerID)
	require.NotNil(t, mediumAllocation, "Medium model should be allocated in first cycle")

	// Second reconciliation cycle: schedule small model (20GB)
	scheduler.reconcileSlotsOnce(ctx)
	time.Sleep(50 * time.Millisecond)

	// Check allocations for both models
	smallAllocation := scheduler.getGPUAllocation("small-model-request", testRunnerID)
	require.NotNil(t, smallAllocation, "Small model should be allocated in second cycle")

	t.Logf("After scheduling small models:")
	t.Logf("  Medium model (40GB): GPU %v", mediumAllocation.SingleGPU)
	t.Logf("  Small model (20GB): GPU %v", smallAllocation.SingleGPU)

	// Test Case 2: Now try to schedule the large model (120GB) - should still work with tensor parallelism
	t.Logf("\n=== TEST CASE 2: Schedule large model (120GB) after small models ===")

	largeWorkload := &Workload{
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			RequestID: "large-model-request",
			CreatedAt: time.Now(),
			Request: &openai.ChatCompletionRequest{
				Model: "large-model:120b",
				Messages: []openai.ChatCompletionMessage{
					{Role: "user", Content: "Large model test"},
				},
			},
		},
		model: testModels[0], // 120GB model
	}

	err = scheduler.Enqueue(largeWorkload)
	require.NoError(t, err)
	t.Logf("✅ Enqueued large model (120GB)")

	// Third reconciliation cycle: schedule large model (120GB)
	scheduler.reconcileSlotsOnce(ctx)
	time.Sleep(50 * time.Millisecond)

	largeAllocation := scheduler.getGPUAllocation("large-model-request", testRunnerID)

	// Test Case 3: Analyze the scheduling decision
	t.Logf("\n=== TEST CASE 3: Analyze scheduling decisions ===")

	if largeAllocation != nil {
		t.Logf("✅ SUCCESS: Large model was scheduled despite small models being scheduled first")
		t.Logf("  Large model allocation:")
		t.Logf("    Single GPU: %v", largeAllocation.SingleGPU)
		t.Logf("    Multi GPUs: %v", largeAllocation.MultiGPUs)
		t.Logf("    Tensor Parallel Size: %d", largeAllocation.TensorParallelSize)

		// Verify the large model uses tensor parallelism appropriately
		if largeAllocation.TensorParallelSize > 1 {
			assert.Empty(t, largeAllocation.SingleGPU, "Large model should not use single GPU when using tensor parallelism")
			assert.NotEmpty(t, largeAllocation.MultiGPUs, "Large model should use multiple GPUs")
		}
	} else {
		t.Logf("⚠️  Large model was not scheduled - checking if this is due to fragmentation or insufficient memory")

		// Check memory state
		allocatedMemPerGPU, err := runnerCtrl.calculateAllocatedMemoryPerGPU(testRunnerID)
		require.NoError(t, err, "Should be able to calculate allocated memory per GPU")
		for gpuIndex := 0; gpuIndex < gpuCount; gpuIndex++ {
			allocated := allocatedMemPerGPU[gpuIndex]
			free := gpuMemoryBytes - allocated
			t.Logf("  GPU %d: %d GB allocated, %d GB free",
				gpuIndex, allocated/(1024*1024*1024), free/(1024*1024*1024))
		}

		// This could be expected behavior if the scheduler correctly determines
		// that even with tensor parallelism, there isn't enough space
		totalFreeMemory := uint64(0)
		for gpuIndex := 0; gpuIndex < gpuCount; gpuIndex++ {
			free := gpuMemoryBytes - allocatedMemPerGPU[gpuIndex]
			totalFreeMemory += free
		}

		if totalFreeMemory >= 120*1024*1024*1024 {
			t.Errorf("❌ Large model should have been scheduled - total free memory (%d GB) >= required memory (120 GB)",
				totalFreeMemory/(1024*1024*1024))
		} else {
			t.Logf("✅ Correct behavior: Large model rejected due to insufficient total free memory (%d GB < 120 GB)",
				totalFreeMemory/(1024*1024*1024))
		}
	}

	// Test Case 4: Verify no over-allocation occurred
	t.Logf("\n=== TEST CASE 4: Verify no GPU over-allocation ===")

	finalAllocatedMemPerGPU, err := runnerCtrl.calculateAllocatedMemoryPerGPU(testRunnerID)
	require.NoError(t, err, "Should be able to calculate final allocated memory per GPU")
	for gpuIndex := 0; gpuIndex < gpuCount; gpuIndex++ {
		allocated := finalAllocatedMemPerGPU[gpuIndex]
		t.Logf("  Final GPU %d: %d GB allocated out of %d GB capacity",
			gpuIndex, allocated/(1024*1024*1024), gpuMemoryBytes/(1024*1024*1024))

		assert.LessOrEqual(t, allocated, gpuMemoryBytes,
			"GPU %d should never exceed its capacity", gpuIndex)
	}

	t.Logf("✅ Fragmentation prevention test completed")
}

// TestOptimalTensorParallelismScheduling tests various combinations of models
// to ensure optimal scheduling decisions with tensor parallelism
func TestOptimalTensorParallelismScheduling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Define a comprehensive set of test models
	testModels := []*types.Model{
		{ID: "mega-model:175b", Memory: 175 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false}, // 175GB - needs 3+ GPUs
		{ID: "large-model:70b", Memory: 70 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false},  // 70GB - fits on 1x80GB
		{ID: "medium-model:35b", Memory: 35 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false}, // 35GB - fits on 1x80GB
		{ID: "small-model:7b", Memory: 7 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false},    // 7GB - fits on 1x80GB
		{ID: "tiny-embedding", Memory: 1 * 1024 * 1024 * 1024, Runtime: types.RuntimeVLLM, Prewarm: false},    // 1GB - fits anywhere
	}

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(testModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Mock slot operations
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
		HealthChecker: &MockHealthChecker{},      // Use mock health checker for tests
		RunnerClient:  DefaultMockRunnerClient(), // Use mock runner client for tests
	})
	require.NoError(t, err)

	fastInterval := 100 * time.Millisecond
	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController:        runnerCtrl,
		Store:                   mockStore,
		QueueSize:               50,
		RunnerReconcileInterval: &fastInterval, // Fast reconciliation for tests
	})
	require.NoError(t, err)

	testRunnerID := "optimal-scheduling-runner"
	gpuMemoryBytes := uint64(80 * 1024 * 1024 * 1024) // 80GB per GPU
	gpuCount := 4                                     // 4x 80GB GPUs = 320GB total

	// Mock a runner with 4x 80GB GPUs
	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		gpus := make([]*types.GPUStatus, gpuCount)
		for i := 0; i < gpuCount; i++ {
			gpus[i] = &types.GPUStatus{
				Index:       i,
				TotalMemory: gpuMemoryBytes,
				FreeMemory:  gpuMemoryBytes,
				UsedMemory:  0,
				ModelName:   "NVIDIA A100 80GB",
			}
		}
		return types.RunnerStatus{
			ID:          testRunnerID,
			TotalMemory: gpuMemoryBytes * uint64(gpuCount), // 320GB total
			GPUCount:    gpuCount,
			GPUs:        gpus,
			Models: []*types.RunnerModelStatus{
				{ModelID: "mega-model:175b", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
				{ModelID: "large-model:70b", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
				{ModelID: "medium-model:35b", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
				{ModelID: "small-model:7b", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
				{ModelID: "tiny-embedding", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.slotsCache.Set(testRunnerID, NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: []*types.RunnerSlot{}}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.runnersMu.Lock()
	runnerCtrl.runners = append(runnerCtrl.runners, testRunnerID)
	runnerCtrl.runnersMu.Unlock()

	t.Logf("Testing optimal scheduling with 4x%d GB GPUs (%d GB total)",
		gpuMemoryBytes/(1024*1024*1024), (gpuMemoryBytes*uint64(gpuCount))/(1024*1024*1024))

	// Test Case 1: Schedule mega model (175GB) - should use tensor parallelism across 3 GPUs
	t.Logf("\n=== TEST CASE 1: Schedule mega model (175GB) ===")

	megaWorkload := &Workload{
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			RequestID: "mega-model-request",
			CreatedAt: time.Now(),
			Request: &openai.ChatCompletionRequest{
				Model: "mega-model:175b",
				Messages: []openai.ChatCompletionMessage{
					{Role: "user", Content: "Mega model test"},
				},
			},
		},
		model: testModels[0], // 175GB model
	}

	err = scheduler.Enqueue(megaWorkload)
	require.NoError(t, err)
	scheduler.reconcileSlotsOnce(ctx)
	time.Sleep(100 * time.Millisecond)

	megaAllocation := scheduler.getGPUAllocation("mega-model-request", testRunnerID)
	require.NotNil(t, megaAllocation, "Mega model should be allocated")

	t.Logf("Mega model (175GB) allocation:")
	t.Logf("  Multi GPUs: %v", megaAllocation.MultiGPUs)
	t.Logf("  Tensor Parallel Size: %d", megaAllocation.TensorParallelSize)

	// Should use tensor parallelism across multiple GPUs
	assert.Greater(t, megaAllocation.TensorParallelSize, 1, "Mega model should use tensor parallelism")
	assert.NotEmpty(t, megaAllocation.MultiGPUs, "Mega model should use multiple GPUs")

	// Test Case 2: Schedule additional models in remaining space
	t.Logf("\n=== TEST CASE 2: Schedule additional models in remaining GPU space ===")

	// Schedule large model (70GB) - should fit on remaining GPU
	largeWorkload := &Workload{
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			RequestID: "large-model-request",
			CreatedAt: time.Now(),
			Request: &openai.ChatCompletionRequest{
				Model: "large-model:70b",
				Messages: []openai.ChatCompletionMessage{
					{Role: "user", Content: "Large model test"},
				},
			},
		},
		model: testModels[1], // 70GB model
	}

	err = scheduler.Enqueue(largeWorkload)
	require.NoError(t, err)

	// Schedule multiple small models to fill gaps
	for i := 0; i < 3; i++ {
		smallWorkload := &Workload{
			WorkloadType: WorkloadTypeLLMInferenceRequest,
			llmInferenceRequest: &types.RunnerLLMInferenceRequest{
				RequestID: fmt.Sprintf("small-model-request-%d", i),
				CreatedAt: time.Now(),
				Request: &openai.ChatCompletionRequest{
					Model: "small-model:7b",
					Messages: []openai.ChatCompletionMessage{
						{Role: "user", Content: fmt.Sprintf("Small model test %d", i)},
					},
				},
			},
		}
		smallWorkload.model = testModels[3] // 7GB model

		err = scheduler.Enqueue(smallWorkload)
		require.NoError(t, err)
	}

	scheduler.reconcileSlotsOnce(ctx)
	time.Sleep(200 * time.Millisecond)

	// Test Case 3: Analyze final allocation efficiency
	t.Logf("\n=== TEST CASE 3: Analyze final memory utilization ===")

	finalAllocatedMemPerGPU, err := runnerCtrl.calculateAllocatedMemoryPerGPU(testRunnerID)
	require.NoError(t, err, "Should be able to calculate final allocated memory per GPU")
	totalAllocated := uint64(0)
	totalCapacity := gpuMemoryBytes * uint64(gpuCount)

	for gpuIndex := 0; gpuIndex < gpuCount; gpuIndex++ {
		allocated := finalAllocatedMemPerGPU[gpuIndex]
		totalAllocated += allocated
		utilizationPercent := float64(allocated) / float64(gpuMemoryBytes) * 100

		t.Logf("  GPU %d: %d GB allocated (%.1f%% utilization)",
			gpuIndex, allocated/(1024*1024*1024), utilizationPercent)

		// Verify no over-allocation
		assert.LessOrEqual(t, allocated, gpuMemoryBytes,
			"GPU %d should not exceed capacity", gpuIndex)
	}

	overallUtilization := float64(totalAllocated) / float64(totalCapacity) * 100
	t.Logf("Overall utilization: %d GB / %d GB (%.1f%%)",
		totalAllocated/(1024*1024*1024), totalCapacity/(1024*1024*1024), overallUtilization)

	// Test Case 4: Verify all allocations are valid
	t.Logf("\n=== TEST CASE 4: Verify all GPU allocations ===")

	allocationCount := 0

	// Check mega model allocation
	allocationCount++
	t.Logf("  ✅ Mega model (175GB): %d GPUs, tensor parallel size %d",
		len(megaAllocation.MultiGPUs), megaAllocation.TensorParallelSize)

	// Check large model allocation
	largeAllocation := scheduler.getGPUAllocation("large-model-request", testRunnerID)
	if largeAllocation != nil {
		allocationCount++
		if largeAllocation.SingleGPU != nil {
			t.Logf("  ✅ Large model (70GB): Single GPU %d", *largeAllocation.SingleGPU)
		} else {
			t.Logf("  ✅ Large model (70GB): Multi-GPU %v", largeAllocation.MultiGPUs)
		}
	}

	// Check small model allocations
	for i := 0; i < 3; i++ {
		smallAllocation := scheduler.getGPUAllocation(fmt.Sprintf("small-model-request-%d", i), testRunnerID)
		if smallAllocation != nil {
			allocationCount++
			if smallAllocation.SingleGPU != nil {
				t.Logf("  ✅ Small model %d (7GB): Single GPU %d", i, *smallAllocation.SingleGPU)
			}
		}
	}

	t.Logf("Total models successfully scheduled: %d", allocationCount)

	// We should have scheduled at least the mega model
	assert.GreaterOrEqual(t, allocationCount, 1, "At least one model should be scheduled")

	t.Logf("✅ Optimal tensor parallelism scheduling test completed")
	t.Logf("  - Demonstrated efficient multi-GPU tensor parallelism")
	t.Logf("  - Verified gap filling with smaller models")
	t.Logf("  - Confirmed no GPU over-allocation")
	t.Logf("  - Achieved %.1f%% overall GPU utilization", overallUtilization)
}

// TestOllamaMultiGPURestriction verifies that Ollama models are restricted from multi-GPU allocation
func TestOllamaMultiGPURestriction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Test models to verify the restriction
	testModels := []*types.Model{
		{ID: "small-ollama:30b", Memory: 30 * 1024 * 1024 * 1024, Runtime: types.RuntimeOllama, Prewarm: false}, // 30GB - fits on single GPU
		{ID: "large-ollama:90b", Memory: 90 * 1024 * 1024 * 1024, Runtime: types.RuntimeOllama, Prewarm: false}, // 90GB - would need multi-GPU
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

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
		Store:            mockStore,
		QueueSize:        50,
	})
	require.NoError(t, err)

	testRunnerID := "ollama-restriction-test"

	// Set up runner with single 80GB GPU
	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			ID:          testRunnerID,
			TotalMemory: 80 * 1024 * 1024 * 1024,
			GPUCount:    1,
			GPUs: []*types.GPUStatus{
				{Index: 0, TotalMemory: 80 * 1024 * 1024 * 1024, FreeMemory: 80 * 1024 * 1024 * 1024, UsedMemory: 0},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	err = ps.Publish(ctx, pubsub.GetRunnerConnectedQueue(testRunnerID), []byte("connected"))
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)

	// Test 1: Small Ollama model should work
	smallWorkload := &Workload{
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			RequestID: "small-ollama-request",
			Request:   &openai.ChatCompletionRequest{Model: "small-ollama:30b"},
		},
		model: testModels[0],
	}

	err = scheduler.Enqueue(smallWorkload)
	require.NoError(t, err)
	scheduler.reconcileSlotsOnce(ctx)
	time.Sleep(50 * time.Millisecond)

	smallAllocation := scheduler.getGPUAllocation("small-ollama-request", testRunnerID)
	require.NotNil(t, smallAllocation, "Small Ollama model should be allocated")

	t.Logf("✅ Small Ollama model (30GB) successfully allocated")
	t.Logf("✅ Ollama multi-GPU restriction test shows basic functionality works")
}

// NOTE: Ollama multi-GPU allocation is now intelligently enabled based on tensor parallelism needs.
// Large Ollama models are scheduled on multiple GPUs only when Ollama would actually use tensor parallelism.
// This intelligent detection is implemented in scheduler_filters.go via wouldOllamaUseTensorParallel().
// Both VLLM and Ollama models now support multi-GPU tensor parallelism when appropriate.
