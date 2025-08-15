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

	// Mock GetModel calls for each model
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
		QueueSize:        10,
	})
	require.NoError(t, err)

	// Set up the test runner with proper status so memory calculation works
	runnerID := "test-runner-1"
	totalMemory := uint64(80 * 1024 * 1024 * 1024) // 80GB

	// Set up the cache BEFORE connecting the runner (important for tests)
	runnerCtrl.statusCache.Set(runnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			ID:          runnerID,
			TotalMemory: totalMemory,
			GPUCount:    1,
			GPUs: []*types.GPUStatus{
				{
					Index:       0,
					TotalMemory: totalMemory,
					FreeMemory:  totalMemory, // Initially all free
					UsedMemory:  0,
					ModelName:   "Test GPU",
				},
			},
			Models: []*types.RunnerModelStatus{
				// Mock that the runner has the models we want to test
				{ModelID: "gpt-oss:20b", Runtime: types.RuntimeOllama, DownloadInProgress: false},
				{ModelID: "qwen3:8b", Runtime: types.RuntimeOllama, DownloadInProgress: false},
				{ModelID: "Qwen/Qwen2.5-VL-7B-Instruct", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
				{ModelID: "MrLight/dse-qwen2-2b-mrl-v1", Runtime: types.RuntimeVLLM, DownloadInProgress: false},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Also set up slots cache to return empty slots initially
	runnerCtrl.slotsCache.Set(runnerID, NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: []*types.RunnerSlot{}}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Track the initial queue size
	initialQueueSize := len(scheduler.queue.Queue())

	// Call PrewarmNewRunner directly (don't use OnConnectedHandler as it clears caches)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},
		RunnerClient:  DefaultMockRunnerClient(),
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},
		RunnerClient:  DefaultMockRunnerClient(),
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

func TestOnRunnerReconnectedCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},
		RunnerClient:  DefaultMockRunnerClient(),
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
		QueueSize:        10,
	})
	require.NoError(t, err)

	// Set up the prewarming callback (as done in serve.go)
	runnerCtrl.SetOnRunnerConnectedCallback(scheduler.PrewarmNewRunner)

	runnerID := "test-runner-reconnect"

	// First connection (should trigger prewarming for new runner)
	initialQueueSize := len(scheduler.queue.Queue())
	runnerCtrl.OnConnectedHandler(runnerID)
	time.Sleep(100 * time.Millisecond)
	afterFirstConnection := len(scheduler.queue.Queue())

	// Verify the runner was added and prewarming happened
	runners := runnerCtrl.RunnerIDs()
	require.Contains(t, runners, runnerID, "Runner should be added to controller")
	require.Greater(t, afterFirstConnection, initialQueueSize, "First connection should trigger prewarming")

	// Second connection (should trigger prewarming attempt for reconnected runner)
	// Note: The actual workloads may not be added if they're already in the queue (deduplication)
	runnerCtrl.OnConnectedHandler(runnerID)
	time.Sleep(100 * time.Millisecond)
	afterSecondConnection := len(scheduler.queue.Queue())

	// Verify prewarming was attempted (even if workloads were deduplicated)
	// The queue size may not increase due to deduplication, but we should see the attempt in logs
	require.GreaterOrEqual(t, afterSecondConnection, afterFirstConnection, "Queue should not shrink after reconnection")

	t.Logf("First connection triggered prewarming: %d workloads", afterFirstConnection-initialQueueSize)
	t.Logf("Reconnection queue size: %d (may be same due to deduplication)", afterSecondConnection)
}

func TestPrewarmWorkloadProperties(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},
		RunnerClient:  DefaultMockRunnerClient(),
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},
		RunnerClient:  DefaultMockRunnerClient(),
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

// TestPrewarmingMemoryAwareSelection tests the memory-first prewarming algorithm
// which prioritizes filling GPU memory while improving model distribution
func TestPrewarmingMemoryAwareSelection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},
		RunnerClient:  DefaultMockRunnerClient(),
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
	// the new runner should still get prewarming workloads using memory-first selection
	require.Greater(t, prewarmWorkloads, 0, "New runner should still get prewarming workloads even when existing distribution seems balanced")

	// Verify the new runner was added
	runners := runnerCtrl.RunnerIDs()
	require.Contains(t, runners, newRunnerID, "New runner should be added to controller")

	t.Logf("Memory-aware selection test: %d prewarming workloads created for new runner using memory-first algorithm", prewarmWorkloads)
}

// TestPrewarmingMemoryConstrainedSelection tests memory-constrained scenarios
// where the algorithm must choose a subset of models based on available GPU memory
func TestPrewarmingMemoryConstrainedSelection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},
		RunnerClient:  DefaultMockRunnerClient(),
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
	// the new runner should still get prewarming workloads using memory-aware selection
	require.Greater(t, prewarmWorkloads, 0, "New runner should get prewarming workloads even in heavily loaded cluster (memory-first algorithm should work)")

	// Verify the new runner was added
	runners := runnerCtrl.RunnerIDs()
	require.Contains(t, runners, newRunnerID, "New runner should be added to controller")

	t.Logf("Memory-constrained selection test: %d prewarming workloads created for new runner with limited memory", prewarmWorkloads)
}

// TestMemoryAwarePrewarming tests that prewarming prioritizes filling available GPU memory
func TestMemoryAwarePrewarming(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},
		RunnerClient:  NewMockRunnerClient(80, 1), // 80GB total memory, 1 GPU as test expects
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
		QueueSize:        50,
	})
	require.NoError(t, err)

	// Test runner with specific memory constraints
	testRunnerID := "memory-test-runner"
	totalMemory := uint64(80 * 1024 * 1024 * 1024) // 80GB - matches our MockRunnerClient configuration

	// Connect the test runner
	runnerCtrl.OnConnectedHandler(testRunnerID)

	// Get prewarm models for this specific runner
	prewarmModels := scheduler.getPrewarmModels(testRunnerID)

	// The memory-aware selection should have chosen models that fit within the 80GB limit
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

func TestPingFrequencyPrewarmingBehavior(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	testModels := GetDefaultTestModels()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(testModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

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
		QueueSize:        100, // Large queue to handle multiple workloads
	})
	require.NoError(t, err)

	// Set up the prewarming callback
	runnerCtrl.SetOnRunnerConnectedCallback(scheduler.PrewarmNewRunner)

	runnerID := "test-runner-ping-frequency"
	initialQueueSize := len(scheduler.queue.Queue())

	// Test 1: Initial connection should trigger prewarming
	err = ps.Publish(ctx, pubsub.GetRunnerConnectedQueue(runnerID), []byte("connected"))
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	afterInitialConnection := len(scheduler.queue.Queue())
	initialWorkloads := afterInitialConnection - initialQueueSize
	require.Greater(t, initialWorkloads, 0, "Initial connection should create prewarming workloads")

	t.Logf("Initial connection created %d prewarming workloads", initialWorkloads)

	// Test 2: Simulate rapid ping messages (every 1 second for 10 seconds)
	// This simulates the 10-second ping interval but faster to speed up the test
	pingCount := 0
	for i := 0; i < 10; i++ {
		err = ps.Publish(ctx, pubsub.GetRunnerConnectedQueue(runnerID), []byte("ping"))
		require.NoError(t, err)
		pingCount++
		time.Sleep(100 * time.Millisecond) // Faster than real 10-second interval
	}

	afterPings := len(scheduler.queue.Queue())
	pingWorkloads := afterPings - afterInitialConnection

	t.Logf("After %d ping messages: queue size went from %d to %d (+%d workloads)",
		pingCount, afterInitialConnection, afterPings, pingWorkloads)

	// The key test: ping messages should not cause exponential growth of prewarming workloads
	// We allow some workloads due to timing/deduplication behavior, but not proportional to ping count
	require.Less(t, pingWorkloads, pingCount,
		"Ping messages should not create workloads proportional to ping frequency")

	// Test 3: Wait for prewarming timeout and send another connected message
	// This simulates a real reconnection after some time
	time.Sleep(100 * time.Millisecond) // Simulate some time passing

	err = ps.Publish(ctx, pubsub.GetRunnerConnectedQueue(runnerID), []byte("connected"))
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	afterReconnection := len(scheduler.queue.Queue())
	reconnectionWorkloads := afterReconnection - afterPings

	t.Logf("Reconnection after ping spam: +%d workloads (may be deduplicated)", reconnectionWorkloads)

	// Verify the system is still functional and not overwhelmed
	require.Less(t, len(scheduler.queue.Queue()), 50,
		"Queue should not be overwhelmed with redundant prewarming workloads")

	// Verify runner is properly tracked
	runners := runnerCtrl.RunnerIDs()
	require.Contains(t, runners, runnerID, "Runner should be tracked after all operations")
}

func TestTimeBasedPrewarmingLogic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	testModels := GetDefaultTestModels()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(testModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

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
		QueueSize:        100,
	})
	require.NoError(t, err)

	// Track prewarming callback invocations
	callbackCount := 0
	runnerCtrl.SetOnRunnerConnectedCallback(func(runnerID string) {
		callbackCount++
		t.Logf("Prewarming callback #%d for runner %s", callbackCount, runnerID)
		scheduler.PrewarmNewRunner(runnerID)
	})

	runnerID := "test-time-based-runner"

	// Test 1: Initial connection should always trigger prewarming
	err = ps.Publish(ctx, pubsub.GetRunnerConnectedQueue(runnerID), []byte("connected"))
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	require.Equal(t, 1, callbackCount, "Initial connection should trigger prewarming")
	require.Greater(t, len(scheduler.queue.Queue()), 0, "Should create prewarming workloads")

	// Test 2: Immediate pings should not trigger additional prewarming
	for i := 0; i < 5; i++ {
		err = ps.Publish(ctx, pubsub.GetRunnerConnectedQueue(runnerID), []byte("ping"))
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
	}

	require.Equal(t, 1, callbackCount, "Immediate pings should not trigger additional prewarming")

	// Test 3: Connection after short time should not trigger prewarming
	err = ps.Publish(ctx, pubsub.GetRunnerConnectedQueue(runnerID), []byte("connected"))
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	require.Equal(t, 1, callbackCount, "Connection within 30 seconds should not trigger additional prewarming")

	t.Logf("Time-based prewarming deduplication is working correctly")
}

func TestPrewarmAllConnectedRunners(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	testModels := GetDefaultTestModels()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(testModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

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
		QueueSize:        100,
	})
	require.NoError(t, err)

	// Track prewarming callback invocations
	callbackCount := 0
	runnerCtrl.SetOnRunnerConnectedCallback(func(runnerID string) {
		callbackCount++
		t.Logf("Prewarming callback #%d for runner %s", callbackCount, runnerID)
		scheduler.PrewarmNewRunner(runnerID)
	})

	// Simulate multiple runners already connected (via pings, simulating server restart scenario)
	runnerIDs := []string{"existing-runner-1", "existing-runner-2", "existing-runner-3"}

	for _, runnerID := range runnerIDs {
		// Add runners to the list without triggering prewarming (simulating they were connected before callback was set)
		runnerCtrl.handleRunnerPing(runnerID)
	}

	initialQueueSize := len(scheduler.queue.Queue())
	require.Equal(t, 0, callbackCount, "No callbacks should have been triggered yet")

	// Now trigger prewarming for all existing runners (simulating server startup)
	runnerCtrl.PrewarmAllConnectedRunners()
	time.Sleep(200 * time.Millisecond) // Give time for async processing

	finalQueueSize := len(scheduler.queue.Queue())
	totalWorkloads := finalQueueSize - initialQueueSize

	require.Equal(t, len(runnerIDs), callbackCount, "Should trigger prewarming for all existing runners")
	require.Greater(t, totalWorkloads, 0, "Should create prewarming workloads for existing runners")

	t.Logf("Initial prewarming: %d runners triggered %d callbacks and created %d workloads",
		len(runnerIDs), callbackCount, totalWorkloads)
}

func TestPingVsConnectedMessageBehavior(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	testModels := GetDefaultTestModels()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return(testModels, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

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
		QueueSize:        10,
	})
	require.NoError(t, err)

	// Set up the prewarming callback
	runnerCtrl.SetOnRunnerConnectedCallback(scheduler.PrewarmNewRunner)

	runnerID := "test-runner-ping-behavior"
	initialQueueSize := len(scheduler.queue.Queue())

	// Test 1: Send a "connected" message - should trigger prewarming
	err = ps.Publish(ctx, pubsub.GetRunnerConnectedQueue(runnerID), []byte("connected"))
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	afterConnectedQueueSize := len(scheduler.queue.Queue())
	connectedWorkloads := afterConnectedQueueSize - initialQueueSize
	require.Greater(t, connectedWorkloads, 0, "Connected message should trigger prewarming workloads")

	t.Logf("Connected message created %d prewarming workloads", connectedWorkloads)

	// Test 2: Simulate workload processing by manually removing workloads from queue
	// This simulates what happens when prewarming workloads are actually processed
	for len(scheduler.queue.Queue()) > 0 {
		workload := scheduler.queue.TakeNext(func(w *Workload) bool { return true })
		if workload != nil {
			t.Logf("Processed prewarming workload: %s", workload.ID())
		}
	}

	clearedQueueSize := len(scheduler.queue.Queue())
	require.Equal(t, 0, clearedQueueSize, "Queue should be empty after processing workloads")

	// Test 3: Now send ping messages after workloads are processed
	preCallbackCount := 0
	runnerCtrl.SetOnRunnerConnectedCallback(func(runnerID string) {
		preCallbackCount++
		t.Logf("Prewarming callback triggered for runner %s (call #%d)", runnerID, preCallbackCount)
		scheduler.PrewarmNewRunner(runnerID)
	})

	for i := 0; i < 3; i++ {
		err = ps.Publish(ctx, pubsub.GetRunnerConnectedQueue(runnerID), []byte("ping"))
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
	}

	afterPingsQueueSize := len(scheduler.queue.Queue())
	t.Logf("After 3 ping messages: %d workloads created, callback called %d times", afterPingsQueueSize, preCallbackCount)

	// This is the key test: ping messages should NOT repeatedly trigger prewarming
	require.Less(t, preCallbackCount, 3, "Ping messages should not all trigger prewarming callbacks")

	// Verify runner is in the list after all operations
	runners := runnerCtrl.RunnerIDs()
	require.Contains(t, runners, runnerID, "Runner should be in the list after ping and connected messages")
}
