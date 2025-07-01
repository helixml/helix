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

func TestIntelligentPrewarming_WithDefaultPrewarmModels(t *testing.T) {
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

	// Test that default prewarm models are returned when no specific distribution exists
	testRunnerID := "test-runner-1"
	prewarmModels := scheduler.getPrewarmModels(testRunnerID)
	require.Equal(t, 3, len(prewarmModels), "Should return default prewarm models from configuration")

	// Verify the expected models are included
	modelIDs := make(map[string]bool)
	for _, model := range prewarmModels {
		modelIDs[model.ID] = true
	}

	require.True(t, modelIDs["Qwen/Qwen2.5-VL-7B-Instruct"], "Should include Qwen2.5-VL-7B")
	require.True(t, modelIDs["MrLight/dse-qwen2-2b-mrl-v1"], "Should include MrLight model")
	require.True(t, modelIDs["llama3.1:8b-instruct-q8_0"], "Should include llama3.1")
}

func TestIntelligentPrewarming_UnevenDistribution(t *testing.T) {
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

	// Set up runners with uneven model distribution
	runnerCtrl.OnConnectedHandler("runner-1")
	runnerCtrl.OnConnectedHandler("runner-2")
	runnerCtrl.OnConnectedHandler("runner-3")

	// Simulate uneven distribution:
	// runner-1: 2x Qwen2.5-VL-3B, 0x others
	// runner-2: 1x llama3.1, 0x others
	// runner-3: empty (new runner)
	mockSlots1 := []*types.RunnerSlot{
		{Model: "Qwen/Qwen2.5-VL-3B-Instruct", Active: true},
		{Model: "Qwen/Qwen2.5-VL-3B-Instruct", Active: true},
	}
	mockSlots2 := []*types.RunnerSlot{
		{Model: "llama3.1:8b-instruct-q8_0", Active: true},
	}
	mockSlots3 := []*types.RunnerSlot{} // Empty

	runnerCtrl.slotsCache.Set("runner-1", NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: mockSlots1}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.slotsCache.Set("runner-2", NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: mockSlots2}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.slotsCache.Set("runner-3", NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: mockSlots3}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Test intelligent selection
	testRunnerID := "runner-3"
	prewarmModels := scheduler.getPrewarmModels(testRunnerID)

	// Should prioritize models with fewer instances
	require.Greater(t, len(prewarmModels), 0, "Should select models for prewarming")

	// Verify models with 0 instances are prioritized over models with 2 instances
	modelCounts := make(map[string]bool)
	for _, model := range prewarmModels {
		modelCounts[model.ID] = true
	}

	// MrLight/dse-qwen2-2b-mrl-v1 and Qwen/Qwen2.5-VL-7B-Instruct should be selected (0 instances)
	// Qwen/Qwen2.5-VL-3B-Instruct should NOT be prioritized (has 2 instances already)
	require.True(t, modelCounts["MrLight/dse-qwen2-2b-mrl-v1"] || modelCounts["Qwen/Qwen2.5-VL-7B-Instruct"],
		"Should prioritize models with 0 instances")

	t.Logf("Selected %d models for intelligent prewarming in uneven scenario", len(prewarmModels))
}

func TestIntelligentPrewarming_BalancedDistribution(t *testing.T) {
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

	runnerCtrl.OnConnectedHandler("runner-1")
	runnerCtrl.OnConnectedHandler("runner-2")

	// Simulate balanced distribution - each runner has same number of each model
	mockSlots1 := []*types.RunnerSlot{
		{Model: "Qwen/Qwen2.5-VL-3B-Instruct", Active: true},
		{Model: "Qwen/Qwen2.5-VL-7B-Instruct", Active: true},
		{Model: "MrLight/dse-qwen2-2b-mrl-v1", Active: true},
		{Model: "llama3.1:8b-instruct-q8_0", Active: true},
	}
	mockSlots2 := []*types.RunnerSlot{
		{Model: "Qwen/Qwen2.5-VL-3B-Instruct", Active: true},
		{Model: "Qwen/Qwen2.5-VL-7B-Instruct", Active: true},
		{Model: "MrLight/dse-qwen2-2b-mrl-v1", Active: true},
		{Model: "llama3.1:8b-instruct-q8_0", Active: true},
	}

	runnerCtrl.slotsCache.Set("runner-1", NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: mockSlots1}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.slotsCache.Set("runner-2", NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: mockSlots2}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Test with balanced distribution
	testRunnerID := "runner-1"
	prewarmModels := scheduler.getPrewarmModels(testRunnerID)

	// With perfectly balanced distribution (difference <= 1), should prewarm all models
	require.Equal(t, 3, len(prewarmModels), "Should prewarm all models when distribution is balanced")

	t.Logf("Balanced scenario - prewarming all %d models", len(prewarmModels))
}

func TestIntelligentPrewarming_EmptyCluster(t *testing.T) {
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

	// Add a runner but no slots (empty cluster)
	runnerCtrl.OnConnectedHandler("runner-1")
	runnerCtrl.slotsCache.Set("runner-1", NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: []*types.RunnerSlot{}}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Test with empty cluster - should prewarm models with lowest counts (all are 0)
	testRunnerID := "runner-1"
	prewarmModels := scheduler.getPrewarmModels(testRunnerID)

	require.Greater(t, len(prewarmModels), 0, "Should prewarm models in empty cluster")
	require.LessOrEqual(t, len(prewarmModels), 4, "Should not exceed total available prewarm models")

	t.Logf("Empty cluster - prewarming %d models", len(prewarmModels))
}

func TestAnalyzeGlobalModelDistribution(t *testing.T) {
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

	// Test the analysis function directly
	prewarmModels := []*types.Model{
		{ID: "model-a", Prewarm: true},
		{ID: "model-b", Prewarm: true},
		{ID: "model-c", Prewarm: true},
	}

	// Set up runners with known distribution
	runnerCtrl.OnConnectedHandler("runner-1")
	runnerCtrl.OnConnectedHandler("runner-2")

	mockSlots1 := []*types.RunnerSlot{
		{Model: "model-a", Active: true},
		{Model: "model-a", Active: true},
		{Model: "model-b", Active: true},
		{Model: "non-prewarm-model", Active: true}, // Should be ignored
	}
	mockSlots2 := []*types.RunnerSlot{
		{Model: "model-a", Active: true},
		{Model: "model-c", Active: true},
		{Model: "model-inactive", Active: false}, // Should be ignored (inactive)
	}

	runnerCtrl.slotsCache.Set("runner-1", NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: mockSlots1}, nil
	}, CacheConfig{updateInterval: time.Second}))

	runnerCtrl.slotsCache.Set("runner-2", NewCache(ctx, func() (types.ListRunnerSlotsResponse, error) {
		return types.ListRunnerSlotsResponse{Slots: mockSlots2}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// Test the analysis
	modelCounts := scheduler.analyzeGlobalModelDistribution(prewarmModels)

	require.Equal(t, 3, modelCounts["model-a"], "model-a should have 3 active instances")
	require.Equal(t, 1, modelCounts["model-b"], "model-b should have 1 active instance")
	require.Equal(t, 1, modelCounts["model-c"], "model-c should have 1 active instance")

	// Verify non-prewarm models are not counted
	require.NotContains(t, modelCounts, "non-prewarm-model")
	require.NotContains(t, modelCounts, "model-inactive")
}

func TestSelectModelsForBalancing(t *testing.T) {
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

	// Test the selection logic directly
	prewarmModels := []*types.Model{
		{ID: "model-high", Memory: 1000, Runtime: types.RuntimeOllama, Prewarm: true},
		{ID: "model-medium", Memory: 1000, Runtime: types.RuntimeOllama, Prewarm: true},
		{ID: "model-low", Memory: 1000, Runtime: types.RuntimeOllama, Prewarm: true},
		{ID: "model-zero", Memory: 1000, Runtime: types.RuntimeOllama, Prewarm: true},
	}

	// Test uneven distribution
	modelCounts := map[string]int{
		"model-high":   5, // High count
		"model-medium": 3, // Medium count
		"model-low":    1, // Low count
		"model-zero":   0, // Zero instances
	}

	// Use large free memory for this test since we're testing distribution logic
	freeMemory := uint64(100 * 1024 * 1024 * 1024) // 100GB
	selectedModels := scheduler.selectModelsForMemoryAndDistribution(prewarmModels, modelCounts, freeMemory)

	require.Greater(t, len(selectedModels), 0, "Should select models for balancing")

	// Should prioritize models with lower counts
	selectedIDs := make(map[string]bool)
	for _, model := range selectedModels {
		selectedIDs[model.ID] = true
	}

	// Zero and low count models should be prioritized
	require.True(t, selectedIDs["model-zero"], "Should select model with zero instances")
	require.True(t, selectedIDs["model-low"], "Should select model with low instances")

	t.Logf("Selected models for balancing: %v", selectedIDs)
}
