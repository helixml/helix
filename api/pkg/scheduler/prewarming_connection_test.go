package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestPrewarmNewRunner_Success(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
		QueueSize:        10,
	})
	require.NoError(t, err)

	// Track the initial queue size
	initialQueueSize := len(scheduler.queue.Queue())

	// Call PrewarmNewRunner directly
	runnerID := "test-runner-1"
	scheduler.PrewarmNewRunner(runnerID)

	// Give some time for async processing
	time.Sleep(50 * time.Millisecond)

	// Check that workloads were enqueued
	finalQueueSize := len(scheduler.queue.Queue())
	require.Greater(t, finalQueueSize, initialQueueSize, "Should have enqueued prewarming workloads")

	// Get the enqueued workloads
	queuedWorkloads := scheduler.queue.Queue()
	prewarmWorkloads := queuedWorkloads[initialQueueSize:] // Get only the newly added ones

	require.Greater(t, len(prewarmWorkloads), 0, "Should have at least one prewarming workload")

	// Verify workload properties
	for _, workload := range prewarmWorkloads {
		require.Equal(t, WorkloadTypeLLMInferenceRequest, workload.WorkloadType, "Should be LLM inference workload")
		require.Equal(t, runnerID, workload.PreferredRunnerID(), "Should prefer the new runner")

		req := workload.LLMInferenceRequest()
		require.NotNil(t, req, "Should have LLM inference request")
		require.Contains(t, req.RequestID, "prewarm-", "Request ID should indicate prewarming")
		require.Contains(t, req.RequestID, runnerID, "Request ID should contain runner ID")
		require.Equal(t, false, req.Priority, "Should have low priority")

		require.NotNil(t, req.Request, "Should have OpenAI request")
		require.Equal(t, "warmup", req.Request.Messages[0].Content, "Should have warmup message")
	}

	t.Logf("Successfully enqueued %d prewarming workloads for runner %s", len(prewarmWorkloads), runnerID)
}

func TestPrewarmNewRunner_VerifyWorkloadCreation(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
		QueueSize:        10,
	})
	require.NoError(t, err)

	initialQueueSize := len(scheduler.queue.Queue())

	// Call PrewarmNewRunner
	runnerID := "test-runner-verify"
	scheduler.PrewarmNewRunner(runnerID)

	time.Sleep(50 * time.Millisecond)

	// Should have created workloads for default prewarm models
	finalQueueSize := len(scheduler.queue.Queue())
	require.Greater(t, finalQueueSize, initialQueueSize, "Should enqueue workloads for default prewarm models")

	// Get the specific models that should be prewarmed
	prewarmModels := scheduler.getPrewarmModels(runnerID)
	expectedWorkloads := len(prewarmModels)
	actualWorkloads := finalQueueSize - initialQueueSize

	require.Equal(t, expectedWorkloads, actualWorkloads, "Should create one workload per prewarm model")
}

func TestOnRunnerConnectedCallback(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
		QueueSize:        10,
	})
	require.NoError(t, err)

	// Set up the prewarming callback (as done in serve.go)
	runnerCtrl.SetOnRunnerConnectedCallback(scheduler.PrewarmNewRunner)

	initialQueueSize := len(scheduler.queue.Queue())

	// Simulate a runner connecting (this should trigger prewarming)
	runnerID := "test-runner-callback"
	runnerCtrl.OnConnectedHandler(runnerID)

	time.Sleep(100 * time.Millisecond)

	// Check that prewarming was triggered
	finalQueueSize := len(scheduler.queue.Queue())
	require.Greater(t, finalQueueSize, initialQueueSize, "Runner connection should trigger prewarming")

	// Verify the runner was added to the controller
	runners := runnerCtrl.RunnerIDs()
	require.Contains(t, runners, runnerID, "Runner should be added to controller")

	t.Logf("Runner connection successfully triggered prewarming: %d workloads enqueued", finalQueueSize-initialQueueSize)
}

func TestPrewarmWorkloadProperties(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
		QueueSize:        10,
	})
	require.NoError(t, err)

	runnerID := "test-runner-properties"
	scheduler.PrewarmNewRunner(runnerID)

	time.Sleep(50 * time.Millisecond)

	queuedWorkloads := scheduler.queue.Queue()
	require.Greater(t, len(queuedWorkloads), 0, "Should have prewarming workloads")

	// Test each prewarming workload
	for _, workload := range queuedWorkloads {
		// Test workload type
		require.Equal(t, WorkloadTypeLLMInferenceRequest, workload.WorkloadType)

		// Test preferred runner
		require.Equal(t, runnerID, workload.PreferredRunnerID())

		// Test model assignment
		require.NotNil(t, workload.model, "Should have model assigned")
		require.True(t, workload.model.Prewarm, "Should only create workloads for prewarm models")

		// Test LLM inference request
		req := workload.LLMInferenceRequest()
		require.NotNil(t, req)

		// Test request ID format
		require.Contains(t, req.RequestID, "prewarm-")
		require.Contains(t, req.RequestID, runnerID)
		require.Contains(t, req.RequestID, workload.model.ID)

		// Test priority (should be low for prewarming)
		require.False(t, req.Priority, "Prewarming should have low priority")

		// Test OpenAI request structure
		require.NotNil(t, req.Request)
		require.Equal(t, workload.model.ID, req.Request.Model)
		require.Len(t, req.Request.Messages, 1)
		require.Equal(t, "user", req.Request.Messages[0].Role)
		require.Equal(t, "warmup", req.Request.Messages[0].Content)

		// Test timing
		require.WithinDuration(t, time.Now(), req.CreatedAt, 5*time.Second, "Should have recent timestamp")
	}
}

func TestMultipleRunnerConnections(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
		QueueSize:        50, // Increase queue size for multiple runners
	})
	require.NoError(t, err)

	runnerCtrl.SetOnRunnerConnectedCallback(scheduler.PrewarmNewRunner)

	initialQueueSize := len(scheduler.queue.Queue())

	// Connect multiple runners
	runnerIDs := []string{"runner-1", "runner-2", "runner-3"}
	for _, runnerID := range runnerIDs {
		runnerCtrl.OnConnectedHandler(runnerID)
		time.Sleep(20 * time.Millisecond) // Small delay between connections
	}

	time.Sleep(100 * time.Millisecond)

	finalQueueSize := len(scheduler.queue.Queue())
	totalPrewarmWorkloads := finalQueueSize - initialQueueSize

	// Should have prewarming workloads for all runners
	require.Greater(t, totalPrewarmWorkloads, 0, "Should have prewarming workloads for multiple runners")

	// Verify all runners were added
	runners := runnerCtrl.RunnerIDs()
	for _, runnerID := range runnerIDs {
		require.Contains(t, runners, runnerID, "All runners should be added to controller")
	}

	t.Logf("Multiple runner connections: %d runners connected, %d prewarming workloads created",
		len(runnerIDs), totalPrewarmWorkloads)
}

// TestPrewarmingIntelligentSelectionFallback tests the scenario where intelligent selection
// might be too conservative and the fallback mechanism ensures pre-warming still works
func TestPrewarmingIntelligentSelectionFallback(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
		QueueSize:        50,
	})
	require.NoError(t, err)

	// Simulate existing runners with balanced model distribution
	runnerCtrl.OnConnectedHandler("existing-runner-1")
	runnerCtrl.OnConnectedHandler("existing-runner-2")

	// Mock slots for existing runners to simulate a scenario where models are well-distributed
	// This could potentially cause the intelligent selection to be too conservative
	mockSlots1 := []*types.RunnerSlot{
		{Model: "Qwen/Qwen2.5-VL-7B-Instruct", Active: true},
		{Model: "MrLight/dse-qwen2-2b-mrl-v1", Active: true},
		{Model: "llama3.1:8b-instruct-q8_0", Active: true},
	}
	mockSlots2 := []*types.RunnerSlot{
		{Model: "Qwen/Qwen2.5-VL-7B-Instruct", Active: true},
		{Model: "MrLight/dse-qwen2-2b-mrl-v1", Active: true},
		{Model: "llama3.1:8b-instruct-q8_0", Active: true},
	}

	runnerCtrl.slotsCache.Set("existing-runner-1", NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: mockSlots1}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.slotsCache.Set("existing-runner-2", NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: mockSlots2}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Set up prewarming callback
	runnerCtrl.SetOnRunnerConnectedCallback(scheduler.PrewarmNewRunner)

	initialQueueSize := len(scheduler.queue.Queue())

	// Now connect a new runner - this is where the issue might occur
	newRunnerID := "new-runner-3"
	runnerCtrl.OnConnectedHandler(newRunnerID)

	time.Sleep(100 * time.Millisecond)

	finalQueueSize := len(scheduler.queue.Queue())
	prewarmWorkloads := finalQueueSize - initialQueueSize

	// The key assertion: even with "perfectly balanced" existing distribution,
	// the new runner should still get prewarming workloads due to our fallback mechanism
	require.Greater(t, prewarmWorkloads, 0, "New runner should still get prewarming workloads even when existing distribution seems balanced")

	// Verify the new runner was added
	runners := runnerCtrl.RunnerIDs()
	require.Contains(t, runners, newRunnerID, "New runner should be added to controller")

	t.Logf("Intelligent selection fallback test: %d prewarming workloads created for new runner despite balanced existing distribution", prewarmWorkloads)
}

// TestPrewarmingZeroSelectionFallback tests a scenario where intelligent selection
// returns zero models and our fallback mechanism kicks in
func TestPrewarmingZeroSelectionFallback(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
		QueueSize:        50,
	})
	require.NoError(t, err)

	// Create a scenario where models are heavily loaded on existing runners
	// This might make the intelligent selection too conservative
	runnerCtrl.OnConnectedHandler("heavy-runner-1")
	runnerCtrl.OnConnectedHandler("heavy-runner-2")
	runnerCtrl.OnConnectedHandler("heavy-runner-3")

	// Mock slots to simulate heavily loaded runners with many instances of each model
	mockHeavySlots := []*types.RunnerSlot{
		// Multiple instances of each model to simulate heavy usage
		{Model: "Qwen/Qwen2.5-VL-7B-Instruct", Active: true},
		{Model: "Qwen/Qwen2.5-VL-7B-Instruct", Active: true},
		{Model: "Qwen/Qwen2.5-VL-7B-Instruct", Active: true},
		{Model: "MrLight/dse-qwen2-2b-mrl-v1", Active: true},
		{Model: "MrLight/dse-qwen2-2b-mrl-v1", Active: true},
		{Model: "MrLight/dse-qwen2-2b-mrl-v1", Active: true},
		{Model: "llama3.1:8b-instruct-q8_0", Active: true},
		{Model: "llama3.1:8b-instruct-q8_0", Active: true},
		{Model: "llama3.1:8b-instruct-q8_0", Active: true},
	}

	// Set the same heavy load on all existing runners
	for _, runnerID := range []string{"heavy-runner-1", "heavy-runner-2", "heavy-runner-3"} {
		runnerCtrl.slotsCache.Set(runnerID, NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
			return types.ListRunnerSlotsResponse{Slots: mockHeavySlots}, nil
		}, CacheConfig{updateInterval: time.Second}))
	}

	// Set up prewarming callback
	runnerCtrl.SetOnRunnerConnectedCallback(scheduler.PrewarmNewRunner)

	initialQueueSize := len(scheduler.queue.Queue())

	// Connect a new runner to this heavily loaded cluster
	newRunnerID := "new-runner-4"
	runnerCtrl.OnConnectedHandler(newRunnerID)

	time.Sleep(100 * time.Millisecond)

	finalQueueSize := len(scheduler.queue.Queue())
	prewarmWorkloads := finalQueueSize - initialQueueSize

	// The critical test: even in a heavily loaded cluster where models seem "over-distributed",
	// the new runner should still get prewarming workloads thanks to our fallback
	require.Greater(t, prewarmWorkloads, 0, "New runner should get prewarming workloads even in heavily loaded cluster (fallback should activate)")

	// Verify the new runner was added
	runners := runnerCtrl.RunnerIDs()
	require.Contains(t, runners, newRunnerID, "New runner should be added to controller")

	t.Logf("Zero selection fallback test: %d prewarming workloads created for new runner in heavily loaded cluster", prewarmWorkloads)
}

// TestMemoryAwarePrewarming tests that prewarming prioritizes filling available GPU memory
func TestMemoryAwarePrewarming(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
		QueueSize:        50,
	})
	require.NoError(t, err)

	// Test runner with specific memory constraints
	testRunnerID := "memory-test-runner"

	// Mock the runner to have specific total memory - let's say 50GB total
	totalMemory := uint64(50 * 1024 * 1024 * 1024) // 50GB
	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory: totalMemory,
			Models: []*types.RunnerModelStatus{
				{
					ModelID:            "test-model",
					DownloadInProgress: false,
					Runtime:            types.RuntimeOllama,
				},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Connect the test runner
	runnerCtrl.OnConnectedHandler(testRunnerID)

	// Get prewarm models for this specific runner
	prewarmModels := scheduler.getPrewarmModels(testRunnerID)

	// The memory-aware selection should have chosen models that fit within the 50GB limit
	require.Greater(t, len(prewarmModels), 0, "Should select at least some models for prewarming")

	// Verify that selected models don't exceed available memory
	totalSelectedMemory := uint64(0)
	for _, model := range prewarmModels {
		totalSelectedMemory += model.Memory
	}

	require.LessOrEqual(t, totalSelectedMemory, totalMemory,
		"Selected models should not exceed available memory (%d GB selected vs %d GB available)",
		totalSelectedMemory/(1024*1024*1024), totalMemory/(1024*1024*1024))

	// Log for visibility
	t.Logf("Memory-aware prewarming test: selected %d models using %d GB out of %d GB available (%.1f%% utilization)",
		len(prewarmModels),
		totalSelectedMemory/(1024*1024*1024),
		totalMemory/(1024*1024*1024),
		float64(totalSelectedMemory)/float64(totalMemory)*100)
}
