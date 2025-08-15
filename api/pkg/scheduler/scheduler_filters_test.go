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
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_filterRunnersByMemory_NoRunners(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStore := store.NewMockStore(mockCtrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Mock slot operations
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	ctrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		HealthChecker: &MockHealthChecker{},
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: ctrl,
		Store:            mockStore,
	})
	require.NoError(t, err)

	// Create a proper workload using the constructor
	workload, err := NewSessionWorkload(&types.Session{
		ID:        "test-session",
		ModelName: "test-model",
		Mode:      types.SessionModeInference,
		Type:      types.SessionTypeText,
	}, &types.Model{
		ID:     "test-model",
		Memory: 1000,
	})
	require.NoError(t, err)

	runners, err := scheduler.filterRunnersByMemory(workload, []string{})
	require.Error(t, err)
	require.Nil(t, runners)
}

func Test_filterRunnersByMemory_SomeRunnersSufficient(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStore := store.NewMockStore(mockCtrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Mock slot operations
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	ctrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		HealthChecker: &MockHealthChecker{},
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: ctrl,
		Store:            mockStore,
	})
	require.NoError(t, err)

	runner1 := "runner-1"
	runner2 := "runner-2"
	runner3 := "runner-3"
	requiredMemory := uint64(2000)
	runnerMemories := map[string]uint64{
		runner1: 3000, // Enough
		runner2: 1000, // Not enough
		runner3: 2500, // Enough
	}

	// Set cache for each runner
	for runnerID, memory := range runnerMemories {
		ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
			return types.RunnerStatus{
				TotalMemory: memory,
				GPUCount:    1, // Single GPU setup for test
				GPUs: []*types.GPUStatus{
					{
						Index:       0,
						TotalMemory: memory,
						FreeMemory:  memory, // Assume all memory is free for test
						UsedMemory:  0,
					},
				},
				Models: []*types.RunnerModelStatus{
					{
						ModelID:            "test-model",
						DownloadInProgress: false,
						Runtime:            types.RuntimeOllama,
					},
				},
			}, nil
		}, CacheConfig{
			updateInterval: 1 * time.Second,
		}))
	}

	workload, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
		Request: &openai.ChatCompletionRequest{
			Model: "test-model",
		},
	}, &types.Model{
		ID:     "test-model",
		Memory: requiredMemory,
	})
	require.NoError(t, err)

	availableRunners := []string{runner1, runner2, runner3}
	filteredRunners, err := scheduler.filterRunnersByMemory(workload, availableRunners)

	require.NoError(t, err)
	require.NotNil(t, filteredRunners)
	require.Len(t, filteredRunners, 2)
	require.ElementsMatch(t, []string{runner1, runner3}, filteredRunners, "Expected runners with enough memory")
}

func Test_filterRunnersByMemory_NoRunnersSufficient(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStore := store.NewMockStore(mockCtrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Mock slot operations
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	ctrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		HealthChecker: &MockHealthChecker{},
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: ctrl,
		Store:            mockStore,
	})
	require.NoError(t, err)

	runner1 := "runner-1"
	runner2 := "runner-2"
	runner3 := "runner-3"
	requiredMemory := uint64(4000)
	runnerMemories := map[string]uint64{
		runner1: 3000, // Not enough
		runner2: 1000, // Not enough
		runner3: 2500, // Not enough
	}

	// Set cache for each runner
	for runnerID, memory := range runnerMemories {
		ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
			return types.RunnerStatus{
				TotalMemory: memory,
				GPUCount:    1, // Single GPU setup for test
				GPUs: []*types.GPUStatus{
					{
						Index:       0,
						TotalMemory: memory,
						FreeMemory:  memory, // Assume all memory is free for test
						UsedMemory:  0,
					},
				},
				Models: []*types.RunnerModelStatus{
					{
						ModelID:            "test-model",
						DownloadInProgress: false,
						Runtime:            types.RuntimeOllama,
					},
				},
			}, nil
		}, CacheConfig{
			updateInterval: 1 * time.Second,
		}))
	}

	workload, err := NewLLMWorkload(&types.RunnerLLMInferenceRequest{
		Request: &openai.ChatCompletionRequest{
			Model: "test-model",
		},
	}, &types.Model{
		ID:     "test-model",
		Memory: requiredMemory,
	})
	require.NoError(t, err)

	availableRunners := []string{runner1, runner2, runner3}
	filteredRunners, err := scheduler.filterRunnersByMemory(workload, availableRunners)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrModelWontFit, "Expected ErrModelWontFit error")
	require.Nil(t, filteredRunners, "Expected no runners to be returned")
}

func Test_filterRunnersByModel_NoRunners(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStore := store.NewMockStore(mockCtrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Mock slot operations
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	ctrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		HealthChecker: &MockHealthChecker{},
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: ctrl,
		Store:            mockStore,
	})
	require.NoError(t, err)

	workload, err := NewSessionWorkload(&types.Session{
		ID:        "test-session",
		ModelName: "test-model",
		Mode:      types.SessionModeInference,
		Type:      types.SessionTypeText,
	}, &types.Model{
		ID:      "test-model",
		Runtime: types.RuntimeOllama,
	})
	require.NoError(t, err)

	filteredRunners, err := scheduler.filterRunnersByModel(workload, []string{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no runners available")
	require.Nil(t, filteredRunners)
}

func Test_filterRunnersByModel_RuntimeNotOllama(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStore := store.NewMockStore(mockCtrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Mock slot operations
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	ctrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		HealthChecker: &MockHealthChecker{},
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: ctrl,
		Store:            mockStore,
	})
	require.NoError(t, err)

	workload, err := NewSessionWorkload(&types.Session{
		ID:        "test-session",
		ModelName: "test-model",
		Mode:      types.SessionModeInference,
		Type:      types.SessionTypeText,
	}, &types.Model{
		ID:      "test-model",
		Runtime: types.RuntimeVLLM, // Not Ollama
	})
	require.NoError(t, err)

	runnerIDs := []string{"runner-1", "runner-2"}
	filteredRunners, err := scheduler.filterRunnersByModel(workload, runnerIDs)

	require.NoError(t, err)
	require.NotNil(t, filteredRunners)
	require.ElementsMatch(t, runnerIDs, filteredRunners, "Should return all runners if runtime is not Ollama")
}

func Test_filterRunnersByModel_AllRunnersHaveModel(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStore := store.NewMockStore(mockCtrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Mock slot operations
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	ctrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		HealthChecker: &MockHealthChecker{},
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: ctrl,
		Store:            mockStore,
	})
	require.NoError(t, err)

	modelID := "test-ollama-model"
	runnerIDs := []string{"runner-1", "runner-2"}

	// Mock status cache for runners
	for _, runnerID := range runnerIDs {
		ctrl.statusCache.Set(runnerID, NewCache(context.Background(), func() (types.RunnerStatus, error) {
			return types.RunnerStatus{
				Models: []*types.RunnerModelStatus{
					{
						ModelID:            modelID,
						DownloadInProgress: false,
						Runtime:            types.RuntimeOllama,
					},
				},
			}, nil
		}, CacheConfig{
			updateInterval: 1 * time.Second,
		}))
	}

	workload, err := NewSessionWorkload(&types.Session{
		ID:        "test-session",
		ModelName: modelID,
		Mode:      types.SessionModeInference,
		Type:      types.SessionTypeText,
	}, &types.Model{
		ID:      modelID,
		Runtime: types.RuntimeOllama,
	})
	require.NoError(t, err)

	filteredRunners, err := scheduler.filterRunnersByModel(workload, runnerIDs)
	require.NoError(t, err)
	require.NotNil(t, filteredRunners)
	require.ElementsMatch(t, runnerIDs, filteredRunners, "All runners should have the model")
}

func Test_filterRunnersByModel_OneRunnerHasModel(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStore := store.NewMockStore(mockCtrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Mock slot operations
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	ctrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		HealthChecker: &MockHealthChecker{},
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: ctrl,
		Store:            mockStore,
	})
	require.NoError(t, err)

	modelID := "test-ollama-model"
	runnerWithModel := "runner-1"
	runnerWithoutModel := "runner-2"
	runnerDownloading := "runner-3"
	runnerIDs := []string{runnerWithModel, runnerWithoutModel, runnerDownloading}

	// Mock status for runner-1 (has model)
	ctrl.statusCache.Set(runnerWithModel, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			Models: []*types.RunnerModelStatus{
				{
					ModelID:            modelID,
					DownloadInProgress: false,
					Runtime:            types.RuntimeOllama,
				},
			},
		}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	// Mock status for runner-2 (different model)
	ctrl.statusCache.Set(runnerWithoutModel, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			Models: []*types.RunnerModelStatus{
				{
					ModelID:            "different-model",
					DownloadInProgress: false,
					Runtime:            types.RuntimeOllama,
				},
			},
		}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	// Mock status for runner-3 (downloading)
	ctrl.statusCache.Set(runnerDownloading, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			Models: []*types.RunnerModelStatus{
				{
					ModelID:            modelID,
					DownloadInProgress: true, // Downloading
					Runtime:            types.RuntimeOllama,
				},
			},
		}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	workload, err := NewSessionWorkload(&types.Session{
		ID:        "test-session",
		ModelName: modelID,
		Mode:      types.SessionModeInference,
		Type:      types.SessionTypeText,
	}, &types.Model{
		ID:      modelID,
		Runtime: types.RuntimeOllama,
	})
	require.NoError(t, err)

	filteredRunners, err := scheduler.filterRunnersByModel(workload, runnerIDs)
	require.NoError(t, err)
	require.NotNil(t, filteredRunners)
	require.Len(t, filteredRunners, 1)
	require.Equal(t, runnerWithModel, filteredRunners[0], "Only runner-1 should have the model ready")
}

func Test_filterRunnersByModel_NoRunnerHasModel(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStore := store.NewMockStore(mockCtrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	mockStore.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Mock slot operations
	mockStore.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	mockStore.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	ctrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub:        ps,
		HealthChecker: &MockHealthChecker{},
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(ctx, &config.ServerConfig{}, &Params{
		RunnerController: ctrl,
		Store:            mockStore,
	})
	require.NoError(t, err)

	modelID := "test-ollama-model"
	runnerWithoutModel := "runner-1"
	runnerDownloading := "runner-2"
	runnerIDs := []string{runnerWithoutModel, runnerDownloading}

	// Mock status for runner-1 (different model)
	ctrl.statusCache.Set(runnerWithoutModel, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			Models: []*types.RunnerModelStatus{
				{
					ModelID:            "different-model",
					DownloadInProgress: false,
					Runtime:            types.RuntimeOllama,
				},
			},
		}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	// Mock status for runner-2 (downloading)
	ctrl.statusCache.Set(runnerDownloading, NewCache(context.Background(), func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			Models: []*types.RunnerModelStatus{
				{
					ModelID:            modelID,
					DownloadInProgress: true, // Downloading
					Runtime:            types.RuntimeOllama,
				},
			},
		}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	workload, err := NewSessionWorkload(&types.Session{
		ID:        "test-session",
		ModelName: modelID,
		Mode:      types.SessionModeInference,
		Type:      types.SessionTypeText,
	}, &types.Model{
		ID:      modelID,
		Runtime: types.RuntimeOllama,
	})
	require.NoError(t, err)

	filteredRunners, err := scheduler.filterRunnersByModel(workload, runnerIDs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no runner has the model "+modelID)
	require.Nil(t, filteredRunners, "Expected no runners to be returned")
}
