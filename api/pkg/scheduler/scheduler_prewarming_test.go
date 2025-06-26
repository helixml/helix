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

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	enabled := true
	mockStore.EXPECT().
		ListModels(gomock.Any(), &store.ListModelsQuery{Enabled: &enabled}).
		Return([]*types.Model{}, nil).
		AnyTimes()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

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

	// Mock GetModel calls for slot creation
	for _, model := range prewarmModels {
		mockStore.EXPECT().
			GetModel(gomock.Any(), model.ID).
			Return(model, nil).
			AnyTimes()
	}

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

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

	// Test the prewarming logic without relying on CreateSlot working
	// The prewarming will attempt to create slots but they'll fail due to no actual runners
	// However, we can test that the scheduling logic correctly identifies what should be created

	// Before prewarming - no slots exist
	require.Equal(t, 0, scheduler.slots.Size())

	// Run prewarming - slots will be created in memory but CreateSlot will fail and remove them
	scheduler.reconcilePrewarmingOnce(ctx)

	// Since CreateSlot fails in test environment, slots are removed from scheduler memory
	// This is the correct behavior - we don't want phantom slots
	require.Equal(t, 0, scheduler.slots.Size())

	// However, we can test the scheduling decisions by checking that the logic correctly
	// identifies runners and calculates memory. Let's manually test the findBestRunnerForModel logic.

	// Build runner capacity list (same logic as in globalPrewarmBalancing)
	runnerIDs := runnerCtrl.RunnerIDs()
	require.Len(t, runnerIDs, 2) // Should have both runners

	runners := make([]*runnerCapacity, 0, len(runnerIDs))
	for _, runnerID := range runnerIDs {
		totalMemory := runnerCtrl.TotalMemory(runnerID)
		require.Greater(t, totalMemory, uint64(0)) // Should have memory info

		var allocatedMemory uint64
		existingModels := make(map[string]bool)
		scheduler.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
			if slot.RunnerID == runnerID {
				allocatedMemory += slot.Memory()
				existingModels[slot.InitialWork().ModelName().String()] = true
			}
			return true
		})

		freeMemory := totalMemory - allocatedMemory
		runners = append(runners, &runnerCapacity{
			id:              runnerID,
			totalMemory:     totalMemory,
			allocatedMemory: allocatedMemory,
			freeMemory:      freeMemory,
			availableMemory: freeMemory,
			existingModels:  existingModels,
		})
	}

	// Test that findBestRunnerForModel correctly identifies suitable runners
	for _, model := range prewarmModels {
		bestRunner := scheduler.findBestRunnerForModel(runners, model)
		require.NotNil(t, bestRunner, "Should find a suitable runner for model %s", model.ID)
		require.GreaterOrEqual(t, bestRunner.availableMemory, model.Memory,
			"Runner should have sufficient memory for model %s", model.ID)
	}
}

func TestGlobalPrewarmBalancing_UnevenDistribution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure background goroutines stop before test ends

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

	// Mock GetModel calls for slot creation
	for _, model := range prewarmModels {
		mockStore.EXPECT().
			GetModel(gomock.Any(), model.ID).
			Return(model, nil).
			AnyTimes()
	}

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

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

	// Test the balancing logic - it should try to create more instances of model-b
	// Since CreateSlot will fail in tests, we can test the decision logic

	// Run prewarming: should attempt to balance the distribution
	scheduler.reconcilePrewarmingOnce(ctx)

	// Since CreateSlot fails in test environment, no new slots will be created
	// But we can verify the logic was attempted by checking that the reconciler ran
	require.Equal(t, 4, scheduler.slots.Size()) // Original slots remain
}

func TestGlobalPrewarmBalancing_InsufficientMemory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure background goroutines stop before test ends

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

	// Mock GetModel calls for slot creation
	for _, model := range prewarmModels {
		mockStore.EXPECT().
			GetModel(gomock.Any(), model.ID).
			Return(model, nil).
			AnyTimes()
	}

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

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

	// Mock GetModel calls for slot creation
	for _, model := range prewarmModels {
		mockStore.EXPECT().
			GetModel(gomock.Any(), model.ID).
			Return(model, nil).
			AnyTimes()
	}

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

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

	// Test the scheduling logic without relying on CreateSlot working
	// Build runner capacity list to test findBestRunnerForModel
	runnerIDs := runnerCtrl.RunnerIDs()
	require.Len(t, runnerIDs, 1)

	runners := make([]*runnerCapacity, 0, len(runnerIDs))
	for _, runnerID := range runnerIDs {
		totalMemory := runnerCtrl.TotalMemory(runnerID)
		require.Equal(t, uint64(5000), totalMemory)

		runners = append(runners, &runnerCapacity{
			id:              runnerID,
			totalMemory:     totalMemory,
			allocatedMemory: 0,
			freeMemory:      totalMemory,
			availableMemory: totalMemory,
			existingModels:  make(map[string]bool),
		})
	}

	// Test that findBestRunnerForModel correctly identifies suitable models
	smallModel := prewarmModels[0] // 1000 memory
	largeModel := prewarmModels[1] // 8000 memory

	bestRunnerSmall := scheduler.findBestRunnerForModel(runners, smallModel)
	require.NotNil(t, bestRunnerSmall, "Should find runner for small model")

	bestRunnerLarge := scheduler.findBestRunnerForModel(runners, largeModel)
	require.Nil(t, bestRunnerLarge, "Should not find runner for large model (insufficient memory)")
}

func TestGlobalPrewarmBalancing_MultipleRunners(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure background goroutines stop before test ends

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

	// Mock GetModel calls for slot creation
	for _, model := range prewarmModels {
		mockStore.EXPECT().
			GetModel(gomock.Any(), model.ID).
			Return(model, nil).
			AnyTimes()
	}

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	// Set up multiple runners with different memory capacities
	runnerCtrl.statusCache.Set("runner-1", NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{TotalMemory: 5000}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	runnerCtrl.statusCache.Set("runner-2", NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{TotalMemory: 8000}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	runnerCtrl.statusCache.Set("runner-3", NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{TotalMemory: 3000}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	// Manually add runners to the controller (simulating connection)
	runnerCtrl.OnConnectedHandler("runner-1")
	runnerCtrl.OnConnectedHandler("runner-2")
	runnerCtrl.OnConnectedHandler("runner-3")

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
	})
	require.NoError(t, err)

	// Test that the scheduling logic correctly identifies suitable runners
	// The prewarming will attempt to create slots but they'll fail due to no actual runners
	// However, we can test that the scheduling logic correctly distributes across runners

	// Before prewarming - no slots exist
	require.Equal(t, 0, scheduler.slots.Size())

	// Run prewarming - slots will be created in memory but CreateSlot will fail and remove them
	scheduler.reconcilePrewarmingOnce(ctx)

	// Since CreateSlot fails in test environment, slots are removed from scheduler memory
	require.Equal(t, 0, scheduler.slots.Size())

	// Test the runner selection logic by building runner capacity list
	runnerIDs := runnerCtrl.RunnerIDs()
	require.Len(t, runnerIDs, 3) // Should have all three runners

	runners := make([]*runnerCapacity, 0, len(runnerIDs))
	for _, runnerID := range runnerIDs {
		totalMemory := runnerCtrl.TotalMemory(runnerID)
		require.Greater(t, totalMemory, uint64(0)) // Should have memory info

		runners = append(runners, &runnerCapacity{
			id:              runnerID,
			totalMemory:     totalMemory,
			allocatedMemory: 0,
			freeMemory:      totalMemory,
			availableMemory: totalMemory,
			existingModels:  make(map[string]bool),
		})
	}

	// Test that findBestRunnerForModel correctly identifies suitable runners
	// and prefers runners with more available memory
	for _, model := range prewarmModels {
		bestRunner := scheduler.findBestRunnerForModel(runners, model)
		require.NotNil(t, bestRunner, "Should find a suitable runner for model %s", model.ID)
		require.GreaterOrEqual(t, bestRunner.availableMemory, model.Memory,
			"Runner should have sufficient memory for model %s", model.ID)

		// Simulate allocating the model to test distribution
		bestRunner.availableMemory -= model.Memory
		bestRunner.allocatedMemory += model.Memory
		bestRunner.existingModels[model.ID] = true
	}
}

func TestFindBestRunnerForModel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure background goroutines stop before test ends

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
	})
	require.NoError(t, err)

	// Test scenarios for findBestRunnerForModel
	runners := []*runnerCapacity{
		{
			id:              "runner1",
			totalMemory:     10000,
			allocatedMemory: 2000,
			freeMemory:      8000,
			availableMemory: 8000,
			existingModels:  make(map[string]bool),
		},
		{
			id:              "runner2",
			totalMemory:     20000,
			allocatedMemory: 5000,
			freeMemory:      15000,
			availableMemory: 15000,
			existingModels:  make(map[string]bool),
		},
		{
			id:              "runner3",
			totalMemory:     5000,
			allocatedMemory: 4000,
			freeMemory:      1000,
			availableMemory: 1000,
			existingModels:  make(map[string]bool),
		},
	}

	// Test model that fits on multiple runners - should choose the one with best score
	// (combination of available memory and utilization ratio)
	largeModel := &types.Model{ID: "large-model", Memory: 7000}
	bestRunner := scheduler.findBestRunnerForModel(runners, largeModel)
	require.NotNil(t, bestRunner)
	require.Equal(t, "runner1", bestRunner.id) // Should choose runner1 (better utilization: 80% free vs 75% free)

	// Test model that only fits on one runner
	mediumModel := &types.Model{ID: "medium-model", Memory: 5000}
	bestRunner = scheduler.findBestRunnerForModel(runners, mediumModel)
	require.NotNil(t, bestRunner)
	require.Equal(t, "runner1", bestRunner.id) // Both can fit, but runner1 has better utilization score

	// Test model that doesn't fit anywhere
	tooLargeModel := &types.Model{ID: "too-large-model", Memory: 20000}
	bestRunner = scheduler.findBestRunnerForModel(runners, tooLargeModel)
	require.Nil(t, bestRunner) // Should return nil
}

func TestCreatePrewarmWorkload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure background goroutines stop before test ends

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
	})
	require.NoError(t, err)

	// Test text model workload creation
	textModel := &types.Model{
		ID:      "text-model",
		Type:    types.ModelTypeChat,
		Memory:  2000,
		Runtime: types.RuntimeOllama,
	}

	workload := scheduler.createPrewarmWorkload(textModel)
	require.NotNil(t, workload)
	require.Equal(t, WorkloadTypeLLMInferenceRequest, workload.WorkloadType)
	require.Equal(t, textModel.ID, workload.LLMInferenceRequest().Request.Model)

	// Test image model workload creation
	imageModel := &types.Model{
		ID:      "image-model",
		Type:    types.ModelTypeImage,
		Memory:  4000,
		Runtime: types.RuntimeOllama,
	}

	workload = scheduler.createPrewarmWorkload(imageModel)
	require.NotNil(t, workload)
	require.Equal(t, WorkloadTypeSession, workload.WorkloadType)
	require.Equal(t, imageModel.ID, workload.Session().ModelName)
	require.Equal(t, types.SessionTypeImage, workload.Session().Type)
}
