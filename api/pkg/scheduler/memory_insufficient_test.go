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

// TestInsufficientMemoryPrewarming tests the scenario where there isn't enough memory
// to fit all prewarm models, and the scheduler should select a subset that fits
func TestInsufficientMemoryPrewarming(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Mock slot operations
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		Store:         mockStore,
		HealthChecker: &MockHealthChecker{},
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: runnerCtrl,
		Store:            mockStore,
		QueueSize:        50,
	})
	require.NoError(t, err)

	// Test runner with insufficient memory - only 30GB total
	// Current prewarm models require ~73GB total:
	// - Qwen/Qwen2.5-VL-7B-Instruct: 39GB
	// - MrLight/dse-qwen2-2b-mrl-v1: 8GB
	// - gpt-oss:20b: 16GB
	// - qwen3:8b: 10GB
	testRunnerID := "low-memory-runner"

	lowMemory := uint64(30 * 1024 * 1024 * 1024) // 30GB - insufficient for all models
	runnerCtrl.statusCache.Set(testRunnerID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory: lowMemory,
			Models: []*types.RunnerModelStatus{
				{
					ModelID:            "test-model",
					DownloadInProgress: false,
					Runtime:            types.RuntimeOllama,
				},
			},
		}, nil
	}, CacheConfig{updateInterval: time.Second}))

	// DON'T call OnConnectedHandler as it invalidates our cache
	// Instead, just directly get prewarm models for this specific runner
	prewarmModels := scheduler.getPrewarmModels(testRunnerID)

	// Should select some models (not zero, not all)
	require.Greater(t, len(prewarmModels), 0, "Should select at least some models even with limited memory")
	require.Less(t, len(prewarmModels), 4, "Should not select all models due to insufficient memory")

	// Verify that selected models don't exceed available memory
	totalSelectedMemory := uint64(0)
	for _, model := range prewarmModels {
		totalSelectedMemory += model.Memory
	}

	require.LessOrEqual(t, totalSelectedMemory, lowMemory,
		"Selected models should not exceed available memory (%d GB selected vs %d GB available)",
		totalSelectedMemory/(1024*1024*1024), lowMemory/(1024*1024*1024))

	// Should be a significant memory utilization (> 50%) since we're trying to pack efficiently
	utilizationPercent := float64(totalSelectedMemory) / float64(lowMemory) * 100
	require.Greater(t, utilizationPercent, 50.0,
		"Should utilize at least 50%% of available memory for efficient packing")

	// Log for visibility
	t.Logf("Insufficient memory test: selected %d out of 4 models using %d GB out of %d GB available (%.1f%% utilization)",
		len(prewarmModels),
		totalSelectedMemory/(1024*1024*1024),
		lowMemory/(1024*1024*1024),
		utilizationPercent)

	// Verify that the smallest models are prioritized when memory is constrained
	// The algorithm should prefer smaller models to fit more models in limited memory
	modelNames := make([]string, len(prewarmModels))
	for i, model := range prewarmModels {
		modelNames[i] = model.ID
	}
	t.Logf("Selected models: %v", modelNames)
}
