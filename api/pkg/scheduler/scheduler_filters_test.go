package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
)

func Test_filterRunnersByMemory_NoRunners(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctrl, err := NewRunnerController(context.Background(), &RunnerControllerConfig{
		PubSub: ps,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(context.Background(), &config.ServerConfig{}, &Params{
		RunnerController: ctrl,
	})
	require.NoError(t, err)

	runners, err := scheduler.filterRunnersByMemory(&Workload{
		model: &types.Model{
			Memory: 1000,
		},
	}, []string{})
	require.Error(t, err)
	require.Nil(t, runners)
}

func Test_filterRunnersByMemory_SomeRunnersSufficient(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctrl, err := NewRunnerController(context.Background(), &RunnerControllerConfig{
		PubSub: ps,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(context.Background(), &config.ServerConfig{}, &Params{
		RunnerController: ctrl,
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

	workload := &Workload{
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			Request: &openai.ChatCompletionRequest{
				Model: "test-model",
			},
		},
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		model: &types.Model{
			Memory: requiredMemory,
		},
	}

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

	ctrl, err := NewRunnerController(context.Background(), &RunnerControllerConfig{
		PubSub: ps,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(context.Background(), &config.ServerConfig{}, &Params{
		RunnerController: ctrl,
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

	workload := &Workload{
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			Request: &openai.ChatCompletionRequest{
				Model: "test-model",
			},
		},
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		model: &types.Model{
			Memory: requiredMemory,
		},
	}

	availableRunners := []string{runner1, runner2, runner3}
	filteredRunners, err := scheduler.filterRunnersByMemory(workload, availableRunners)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrModelWontFit, "Expected ErrModelWontFit error")
	require.Nil(t, filteredRunners, "Expected no runners to be returned")
}
