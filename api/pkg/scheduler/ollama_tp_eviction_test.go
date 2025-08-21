package scheduler

import (
	"context"
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

// TestOllamaTPEvictionFallback tests the scenario where:
// 1. Ollama TP prediction rejects multi-GPU allocation due to fixed overhead
// 2. Scheduler falls back to eviction-based single-GPU allocation
func TestOllamaTPEvictionFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Define test models
	testModels := []*types.Model{
		// Large Ollama model that would need eviction to fit on single GPU
		{ID: "large-ollama:70b", Memory: 70 * 1024 * 1024 * 1024, Runtime: types.RuntimeOllama, Prewarm: false}, // 70GB
		// Smaller model that will be evicted
		{ID: "small-model:7b", Memory: 7 * 1024 * 1024 * 1024, Runtime: types.RuntimeOllama, Prewarm: false}, // 7GB
	}

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(testModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Mock GetModel calls
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

	testRunnerID := "ollama-tp-eviction-test-runner"

	// Create a mock runner with 2x 40GB GPUs (total 80GB)
	// This setup ensures that:
	// - 70GB model won't fit with TP (due to 12GB overhead per GPU = 24GB total overhead)
	// - 70GB model will need eviction to fit on single GPU
	gpuCount := 2
	gpuMemoryBytes := uint64(40 * 1024 * 1024 * 1024) // 40GB per GPU

	// Create a test slot that will be evicted
	smallModelSlot := CreateTestSlot(testRunnerID, "small-model:7b", 7*1024*1024*1024,
		func() *int { i := 0; return &i }(), nil)
	// Make it stale for eviction
	smallModelSlot.LastActivityTime = time.Now().Add(-2 * time.Hour)

	// Initially, one GPU has a small model allocated (7GB)
	initialSlots := []*types.RunnerSlot{
		{
			ID:                 smallModelSlot.ID,
			RunnerID:           testRunnerID,
			Model:              "small-model:7b",
			GPUIndex:           func() *int { i := 0; return &i }(),
			TensorParallelSize: 1,
			Created:            time.Now().Add(-2 * time.Hour), // Make it stale for eviction
		},
	}

	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		gpus := make([]*types.GPUStatus, gpuCount)
		for i := 0; i < gpuCount; i++ {
			gpus[i] = &types.GPUStatus{
				Index:       i,
				TotalMemory: gpuMemoryBytes,
				FreeMemory:  gpuMemoryBytes,
				UsedMemory:  0,
				ModelName:   "NVIDIA A100 40GB",
			}
		}
		// GPU 0 has the small model allocated
		gpus[0].FreeMemory = gpuMemoryBytes - 7*1024*1024*1024 // 40GB - 7GB = 33GB free
		gpus[0].UsedMemory = 7 * 1024 * 1024 * 1024

		return types.RunnerStatus{
			ID:          testRunnerID,
			TotalMemory: gpuMemoryBytes * uint64(gpuCount),
			GPUCount:    gpuCount,
			GPUs:        gpus,
			Models: []*types.RunnerModelStatus{
				{ModelID: "large-ollama:70b", Runtime: types.RuntimeOllama, DownloadInProgress: false},
				{ModelID: "small-model:7b", Runtime: types.RuntimeOllama, DownloadInProgress: false},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.slotsCache.Set(testRunnerID, NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: initialSlots}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.runnersMu.Lock()
	runnerCtrl.runners = append(runnerCtrl.runners, testRunnerID)
	runnerCtrl.runnersMu.Unlock()

	// Add initial slot to scheduler
	scheduler.slots.Store(smallModelSlot.ID, smallModelSlot)

	// Simulate runner connection
	err = ps.Publish(ctx, pubsub.GetRunnerConnectedQueue(testRunnerID), []byte("connected"))
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	t.Logf("Testing Ollama TP → Eviction fallback scenario")
	t.Logf("Setup: 2x40GB GPUs, GPU 0 has 7GB model (stale), GPU 1 is free")
	t.Logf("Request: 70GB Ollama model")
	t.Logf("Expected: TP rejected (70GB + 24GB overhead > 80GB), eviction successful")

	// Test: Schedule large Ollama model that requires eviction
	largeWorkload := &Workload{
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			RequestID: "large-ollama-eviction-test",
			CreatedAt: time.Now(),
			Request: &openai.ChatCompletionRequest{
				Model: "large-ollama:70b",
				Messages: []openai.ChatCompletionMessage{
					{Role: "user", Content: "Large Ollama test requiring eviction"},
				},
			},
		},
		model: testModels[0],
	}

	err = scheduler.Enqueue(largeWorkload)
	require.NoError(t, err)
	t.Logf("✅ Enqueued large Ollama model (70GB)")

	// Allow scheduler to process
	scheduler.reconcileSlotsOnce(ctx)
	time.Sleep(200 * time.Millisecond) // Give more time for eviction

	// Check that the model was allocated (should succeed via eviction)
	allocation := scheduler.getGPUAllocation("large-ollama-eviction-test", testRunnerID)
	require.NotNil(t, allocation, "Large Ollama model should be allocated via eviction fallback")
	assert.NotNil(t, allocation.SingleGPU, "Should be single-GPU allocation after eviction")
	assert.Nil(t, allocation.MultiGPUs, "Should not be multi-GPU allocation")

	t.Logf("✅ Large Ollama model successfully allocated via eviction fallback to GPU %d", *allocation.SingleGPU)

	// Verify that eviction occurred by checking if the small model slot was removed
	_, smallModelSlotExists := scheduler.slots.Load(smallModelSlot.ID)

	assert.False(t, smallModelSlotExists, "Small model slot should have been evicted")
	t.Logf("✅ Confirmed eviction occurred - small model slot was removed")

	t.Logf("\n✅ Ollama TP → Eviction fallback test completed successfully")
	t.Logf("  - TP prediction correctly rejected 70GB model on 2x40GB GPUs due to overhead")
	t.Logf("  - Eviction fallback successfully allocated model on single GPU")
	t.Logf("  - Stale slot was properly evicted to make room")
}

// TestOllamaTPOverheadCalculation tests the fixed overhead calculation logic
func TestOllamaTPOverheadCalculation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()

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
		QueueSize:        10,
	})
	require.NoError(t, err)

	testRunnerID := "overhead-test-runner"

	testCases := []struct {
		name        string
		gpuSizes    []uint64 // GB
		modelSize   uint64   // GB
		expectedTP  bool
		description string
	}{
		{
			name:        "Small model on large GPUs - should use TP",
			gpuSizes:    []uint64{80, 80}, // 2x80GB
			modelSize:   100,              // 100GB
			expectedTP:  true,
			description: "100GB model on 2x80GB GPUs: 160GB - 20GB overhead = 140GB effective > 100GB",
		},
		{
			name:        "Large model on small GPUs - should reject TP",
			gpuSizes:    []uint64{40, 40}, // 2x40GB
			modelSize:   70,               // 70GB
			expectedTP:  false,
			description: "70GB model on 2x40GB GPUs: 80GB - 20GB overhead = 60GB effective < 70GB",
		},
		{
			name:        "Edge case - exactly at limit",
			gpuSizes:    []uint64{50, 50}, // 2x50GB
			modelSize:   80,               // 80GB
			expectedTP:  true,
			description: "80GB model on 2x50GB GPUs: 100GB - 20GB overhead = 80GB effective = 80GB (should fit)",
		},
		{
			name:        "Very large model - should reject",
			gpuSizes:    []uint64{80, 80, 80}, // 3x80GB
			modelSize:   220,                  // 220GB
			expectedTP:  false,
			description: "220GB model on 3x80GB GPUs: 240GB - 30GB overhead = 210GB effective < 220GB",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock runner with specified GPU configuration
			gpus := make([]*types.GPUStatus, len(tc.gpuSizes))
			var totalMemory uint64
			for i, size := range tc.gpuSizes {
				memBytes := size * 1024 * 1024 * 1024
				gpus[i] = &types.GPUStatus{
					Index:       i,
					TotalMemory: memBytes,
					FreeMemory:  memBytes,
					UsedMemory:  0,
					ModelName:   "Test GPU",
				}
				totalMemory += memBytes
			}

			runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
				return types.RunnerStatus{
					ID:          testRunnerID,
					TotalMemory: totalMemory,
					GPUCount:    len(tc.gpuSizes),
					GPUs:        gpus,
				}, nil
			}, CacheConfig{updateInterval: time.Second}))

			// Test the TP prediction
			modelMemoryBytes := tc.modelSize * 1024 * 1024 * 1024
			wouldUseTP, assignedGPUs := scheduler.wouldOllamaUseTensorParallel(testRunnerID, modelMemoryBytes)

			assert.Equal(t, tc.expectedTP, wouldUseTP,
				"TP prediction mismatch for %s: %s", tc.name, tc.description)

			if tc.expectedTP {
				assert.True(t, len(assignedGPUs) > 1,
					"Expected multi-GPU assignment for %s", tc.name)
			} else {
				// When TP is rejected, assignedGPUs might be empty or single GPU
				assert.True(t, len(assignedGPUs) <= 1,
					"Expected single or no GPU assignment when TP rejected for %s", tc.name)
			}

			t.Logf("✅ %s: TP=%v, GPUs=%v - %s",
				tc.name, wouldUseTP, assignedGPUs, tc.description)
		})
	}

	t.Logf("✅ Ollama TP overhead calculation tests completed")
}
