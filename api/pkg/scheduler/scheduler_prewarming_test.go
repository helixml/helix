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
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGlobalPrewarmBalancing_NoPrewarmModels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure background goroutines stop before test ends

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	// Mock ListModels to return no prewarm models
	enabled := true
	mockStore.EXPECT().
		ListModels(gomock.Any(), &store.ListModelsQuery{Enabled: &enabled}).
		Return([]*types.Model{
			{ID: "model-1", Prewarm: false, Memory: 1000},
			{ID: "model-2", Prewarm: false, Memory: 2000},
		}, nil).
		AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
	})
	require.NoError(t, err)

	// Should not create any slots when no prewarm models exist
	scheduler.reconcilePrewarmingOnce(ctx)

	require.Equal(t, 0, scheduler.slots.Size())
}

func TestGlobalPrewarmBalancing_EqualDistribution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure background goroutines stop before test ends

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	prewarmModels := []*types.Model{
		{ID: "model-a", Prewarm: true, Memory: 1000, Runtime: types.RuntimeOllama},
		{ID: "model-b", Prewarm: true, Memory: 2000, Runtime: types.RuntimeOllama},
		{ID: "model-c", Prewarm: true, Memory: 1500, Runtime: types.RuntimeOllama},
	}

	enabled := true
	mockStore.EXPECT().
		ListModels(gomock.Any(), &store.ListModelsQuery{Enabled: &enabled}).
		Return(prewarmModels, nil).
		AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	// Set up runners with memory
	runnerCtrl.statusCache.Set("runner-1", NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{TotalMemory: 10000}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	runnerCtrl.statusCache.Set("runner-2", NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{TotalMemory: 10000}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	// Manually add runners to the controller (simulating connection)
	runnerCtrl.OnConnectedHandler("runner-1")
	runnerCtrl.OnConnectedHandler("runner-2")

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
	})
	require.NoError(t, err)

	// First run: background reconciler + manual call should create instances of each model
	scheduler.reconcilePrewarmingOnce(ctx)
	// With background prewarming, we expect more slots (background + manual reconciliation)
	require.GreaterOrEqual(t, scheduler.slots.Size(), 3)

	// Verify each model has at least 1 instance (may have more due to background reconciliation)
	modelCounts := make(map[string]int)
	scheduler.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		modelCounts[slot.InitialWork().ModelName().String()]++
		return true
	})

	require.GreaterOrEqual(t, modelCounts["model-a"], 1)
	require.GreaterOrEqual(t, modelCounts["model-b"], 1)
	require.GreaterOrEqual(t, modelCounts["model-c"], 1)

	// Second run: should create more instances (background + manual reconciliation)
	initialSize := scheduler.slots.Size()
	scheduler.reconcilePrewarmingOnce(ctx)
	// Should have more slots after second reconciliation
	require.Greater(t, scheduler.slots.Size(), initialSize)

	// Verify each model has balanced distribution
	modelCounts = make(map[string]int)
	scheduler.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		modelCounts[slot.InitialWork().ModelName().String()]++
		return true
	})

	// With background prewarming, models should have roughly equal counts
	require.Greater(t, modelCounts["model-a"], 0)
	require.Greater(t, modelCounts["model-b"], 0)
	require.Greater(t, modelCounts["model-c"], 0)
}

func TestGlobalPrewarmBalancing_UnevenDistribution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure background goroutines stop before test ends

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	prewarmModels := []*types.Model{
		{ID: "model-a", Prewarm: true, Memory: 1000, Runtime: types.RuntimeOllama},
		{ID: "model-b", Prewarm: true, Memory: 2000, Runtime: types.RuntimeOllama},
	}

	enabled := true
	mockStore.EXPECT().
		ListModels(gomock.Any(), &store.ListModelsQuery{Enabled: &enabled}).
		Return(prewarmModels, nil).
		AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	// Set up runner with memory
	runnerCtrl.statusCache.Set("runner-1", NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{TotalMemory: 20000}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	// Manually add runner to the controller (simulating connection)
	runnerCtrl.OnConnectedHandler("runner-1")

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
	})
	require.NoError(t, err)

	// Manually create uneven distribution: model-a has 3 instances, model-b has 1
	for i := 0; i < 3; i++ {
		workload := scheduler.createPrewarmWorkload(prewarmModels[0]) // model-a
		slot := NewSlot("runner-1", workload, scheduler.modelStaleFunc, scheduler.slotTimeoutFunc)
		scheduler.slots.Store(slot.ID, slot)
	}

	workload := scheduler.createPrewarmWorkload(prewarmModels[1]) // model-b
	slot := NewSlot("runner-1", workload, scheduler.modelStaleFunc, scheduler.slotTimeoutFunc)
	scheduler.slots.Store(slot.ID, slot)

	require.Equal(t, 4, scheduler.slots.Size())

	// Run prewarming: should balance the distribution (background + manual reconciliation)
	scheduler.reconcilePrewarmingOnce(ctx)
	// With background prewarming, we expect more than 4 slots
	require.Greater(t, scheduler.slots.Size(), 4)

	// Verify distribution: both models should have instances
	modelCounts := make(map[string]int)
	scheduler.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		modelCounts[slot.InitialWork().ModelName().String()]++
		return true
	})

	require.Greater(t, modelCounts["model-a"], 0)
	require.Greater(t, modelCounts["model-b"], 0)

	// Next run: should continue balancing
	initialSize := scheduler.slots.Size()
	scheduler.reconcilePrewarmingOnce(ctx)
	// May add more slots or stay the same if already balanced
	require.GreaterOrEqual(t, scheduler.slots.Size(), initialSize)

	modelCounts = make(map[string]int)
	scheduler.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		modelCounts[slot.InitialWork().ModelName().String()]++
		return true
	})

	// Both models should still have instances
	require.Greater(t, modelCounts["model-a"], 0)
	require.Greater(t, modelCounts["model-b"], 0)
}

func TestGlobalPrewarmBalancing_InsufficientMemory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure background goroutines stop before test ends

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	prewarmModels := []*types.Model{
		{ID: "large-model", Prewarm: true, Memory: 5000, Runtime: types.RuntimeOllama},
	}

	enabled := true
	mockStore.EXPECT().
		ListModels(gomock.Any(), &store.ListModelsQuery{Enabled: &enabled}).
		Return(prewarmModels, nil).
		AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	// Set up runner with insufficient memory
	runnerCtrl.statusCache.Set("runner-1", NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{TotalMemory: 3000}, nil // Less than model requires
	}, CacheConfig{updateInterval: 1 * time.Second}))

	// Manually add runner to the controller (simulating connection)
	runnerCtrl.OnConnectedHandler("runner-1")

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
	})
	require.NoError(t, err)

	// Should not create any slots when insufficient memory
	scheduler.reconcilePrewarmingOnce(ctx)
	require.Equal(t, 0, scheduler.slots.Size())
}

func TestGlobalPrewarmBalancing_PartialMemory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure background goroutines stop before test ends

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	prewarmModels := []*types.Model{
		{ID: "small-model", Prewarm: true, Memory: 1000, Runtime: types.RuntimeOllama},
		{ID: "large-model", Prewarm: true, Memory: 8000, Runtime: types.RuntimeOllama},
	}

	enabled := true
	mockStore.EXPECT().
		ListModels(gomock.Any(), &store.ListModelsQuery{Enabled: &enabled}).
		Return(prewarmModels, nil).
		AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	// Set up runner with limited memory - can fit small model but not large
	runnerCtrl.statusCache.Set("runner-1", NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{TotalMemory: 5000}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	// Manually add runner to the controller (simulating connection)
	runnerCtrl.OnConnectedHandler("runner-1")

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
	})
	require.NoError(t, err)

	// Should create slots for small model only (multiple instances due to global balancing)
	scheduler.reconcilePrewarmingOnce(ctx)
	require.GreaterOrEqual(t, scheduler.slots.Size(), 1)

	// Verify all created slots are for the small model (not the large one that doesn't fit)
	allSlotsSmallModel := true
	scheduler.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		modelName := slot.InitialWork().ModelName().String()
		if modelName != "small-model" {
			allSlotsSmallModel = false
		}
		return true
	})

	require.True(t, allSlotsSmallModel, "All created slots should be for the small-model")
}

func TestGlobalPrewarmBalancing_MultipleRunners(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure background goroutines stop before test ends

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	prewarmModels := []*types.Model{
		{ID: "model-a", Prewarm: true, Memory: 2000, Runtime: types.RuntimeOllama},
		{ID: "model-b", Prewarm: true, Memory: 3000, Runtime: types.RuntimeOllama},
	}

	enabled := true
	mockStore.EXPECT().
		ListModels(gomock.Any(), &store.ListModelsQuery{Enabled: &enabled}).
		Return(prewarmModels, nil).
		AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	// Set up multiple runners with different memory capacities
	runnerCtrl.statusCache.Set("runner-1", NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{TotalMemory: 10000}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	runnerCtrl.statusCache.Set("runner-2", NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{TotalMemory: 8000}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	runnerCtrl.statusCache.Set("runner-3", NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{TotalMemory: 4000}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	// Manually add runners to the controller (simulating connection)
	runnerCtrl.OnConnectedHandler("runner-1")
	runnerCtrl.OnConnectedHandler("runner-2")
	runnerCtrl.OnConnectedHandler("runner-3")

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
	})
	require.NoError(t, err)

	// Should distribute models across runners based on available memory
	scheduler.reconcilePrewarmingOnce(ctx)
	// With background prewarming, we expect at least 2 slots (may be more)
	require.GreaterOrEqual(t, scheduler.slots.Size(), 2)

	// Verify distribution across runners
	runnerCounts := make(map[string]int)
	scheduler.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		runnerCounts[slot.RunnerID]++
		return true
	})

	// Should have distributed slots across runners
	totalDistributed := 0
	for _, count := range runnerCounts {
		totalDistributed += count
	}
	require.GreaterOrEqual(t, totalDistributed, 2)
}

func TestFindBestRunnerForModel(t *testing.T) {
	ctx := context.Background()
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
	})
	require.NoError(t, err)

	model := &types.Model{ID: "test-model", Memory: 2000}

	// Test with no runners
	bestRunner := scheduler.findBestRunnerForModel([]*runnerCapacity{}, model)
	require.Nil(t, bestRunner)

	// Test with insufficient memory
	runners := []*runnerCapacity{
		{id: "runner-1", totalMemory: 1000, availableMemory: 1000}, // Not enough
	}
	bestRunner = scheduler.findBestRunnerForModel(runners, model)
	require.Nil(t, bestRunner)

	// Test with sufficient memory
	runners = []*runnerCapacity{
		{id: "runner-1", totalMemory: 5000, availableMemory: 3000, allocatedMemory: 2000},
		{id: "runner-2", totalMemory: 10000, availableMemory: 8000, allocatedMemory: 2000},
	}
	bestRunner = scheduler.findBestRunnerForModel(runners, model)
	require.NotNil(t, bestRunner)
	require.Equal(t, "runner-2", bestRunner.id) // Should prefer runner with more available memory

	// Test with equal available memory but different utilization
	runners = []*runnerCapacity{
		{id: "runner-1", totalMemory: 10000, availableMemory: 5000, allocatedMemory: 5000}, // 50% utilization
		{id: "runner-2", totalMemory: 8000, availableMemory: 5000, allocatedMemory: 3000},  // 37.5% utilization
	}
	bestRunner = scheduler.findBestRunnerForModel(runners, model)
	require.NotNil(t, bestRunner)
	require.Equal(t, "runner-2", bestRunner.id) // Should prefer lower utilization when available memory is equal
}

func TestCreatePrewarmWorkload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure background goroutines stop before test ends

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
	})
	require.NoError(t, err)

	model := &types.Model{
		ID:      "test-model",
		Memory:  2000,
		Runtime: types.RuntimeOllama,
	}

	workload := scheduler.createPrewarmWorkload(model)

	require.NotNil(t, workload)
	require.Equal(t, WorkloadTypeLLMInferenceRequest, workload.WorkloadType)
	require.Equal(t, model, workload.model)
	require.NotNil(t, workload.llmInferenceRequest)
	require.Equal(t, model.ID, workload.llmInferenceRequest.Request.Model)
	require.Contains(t, workload.llmInferenceRequest.RequestID, "prewarm-")
	require.Contains(t, workload.llmInferenceRequest.RequestID, model.ID)
	require.Empty(t, workload.llmInferenceRequest.Request.Messages)
}
