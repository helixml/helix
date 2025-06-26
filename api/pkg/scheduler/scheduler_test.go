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

func TestScheduler_ChoosesAlternateRunnerWhenPrimaryHasBlockingSlot(t *testing.T) {
	ctx := context.Background()
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Set up mock store
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	enabled := true
	mockStore.EXPECT().
		ListModels(ctx, &store.ListModelsQuery{Enabled: &enabled}).
		Return([]*types.Model{}, nil).
		AnyTimes()

	// Add mock expectations for GetModel calls during slot creation
	mockStore.EXPECT().
		GetModel(gomock.Any(), gomock.Any()).
		Return(&types.Model{ContextLength: 4096}, nil).
		AnyTimes()

	// Create runner controller
	runnerCtrl, err := NewRunnerController(ctx, &RunnerControllerConfig{
		PubSub: ps,
		Store:  mockStore,
	})
	require.NoError(t, err)

	// Create scheduler
	scheduler, err := NewScheduler(ctx, &config.ServerConfig{
		Providers: config.Providers{
			Helix: config.Helix{
				ModelTTL: 300 * time.Second,
				SlotTTL:  300 * time.Second,
			},
		},
	}, &Params{
		RunnerController: runnerCtrl,
		OnSchedulingErr:  func(work *Workload, err error) {}, // No-op error handler
	})
	require.NoError(t, err)

	// Setup runner1 with lower total load (23GB) - will be picked first
	runner1ID := "runner1"
	runnerCtrl.statusCache.Set(runner1ID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory: 24 * 1024 * 1024 * 1024, // 24GB
			Models: []*types.RunnerModelStatus{
				{
					ModelID:            "embedding-model",
					DownloadInProgress: false,
					Runtime:            types.RuntimeVLLM,
				},
			},
		}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	// Setup runner2 with higher total load (24GB) - will be picked second
	runner2ID := "runner2"
	runnerCtrl.statusCache.Set(runner2ID, NewCache(ctx, func() (types.RunnerStatus, error) {
		return types.RunnerStatus{
			TotalMemory: 24 * 1024 * 1024 * 1024, // 24GB
			Models: []*types.RunnerModelStatus{
				{
					ModelID:            "embedding-model",
					DownloadInProgress: false,
					Runtime:            types.RuntimeVLLM,
				},
			},
		}, nil
	}, CacheConfig{updateInterval: 1 * time.Second}))

	// Add runners to controller
	runnerCtrl.OnConnectedHandler(runner1ID)
	runnerCtrl.OnConnectedHandler(runner2ID)

	// Create workloads
	qwenWorkload := &Workload{
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			Request: &openai.ChatCompletionRequest{Model: "qwen-model"},
		},
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		model: &types.Model{
			ID:      "qwen-model",
			Memory:  22 * 1024 * 1024 * 1024, // 22GB
			Runtime: types.RuntimeVLLM,
		},
	}

	smallWorkload := &Workload{
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			Request: &openai.ChatCompletionRequest{Model: "small-model"},
		},
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		model: &types.Model{
			ID:      "small-model",
			Memory:  1 * 1024 * 1024 * 1024, // 1GB (reduced to make runner1 have lower total load)
			Runtime: types.RuntimeVLLM,
		},
	}

	embeddingWorkload := &Workload{
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			Request: &openai.ChatCompletionRequest{Model: "embedding-model"},
		},
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		model: &types.Model{
			ID:      "embedding-model",
			Memory:  2 * 1024 * 1024 * 1024, // 2GB
			Runtime: types.RuntimeVLLM,
		},
	}

	// Setup runner1 with blocking slot configuration
	// Slot A: 22GB Qwen model - stuck in "starting" state (can't be evicted)
	qwenSlot := NewSlot(runner1ID, qwenWorkload, scheduler.modelStaleFunc, scheduler.slotTimeoutFunc)
	qwenSlot.isRunning = false             // Not running yet (stuck in starting)
	qwenSlot.LastActivityTime = time.Now() // Recent activity (not stale)
	scheduler.slots.Store(qwenSlot.ID, qwenSlot)

	// Slot B: 2GB small model - stale and evictable
	smallSlot := NewSlot(runner1ID, smallWorkload, scheduler.modelStaleFunc, scheduler.slotTimeoutFunc)
	smallSlot.isRunning = true                                      // Running
	smallSlot.LastActivityTime = time.Now().Add(-400 * time.Second) // Past stale timeout
	scheduler.slots.Store(smallSlot.ID, smallSlot)

	// Total runner1 load: 23GB (22GB + 1GB)
	// After evicting stale slot: 22GB used, only 2GB free
	// But we need 24GB total for the new 2GB embedding model (22GB qwen + 2GB new = 24GB)
	// Since runner1 only has 24GB total, this should fail due to insufficient memory

	// Setup runner2 with evictable slots
	// Both slots are stale and can be evicted, giving us 24GB available
	bigSlot1 := NewSlot(runner2ID, &Workload{
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			Request: &openai.ChatCompletionRequest{Model: "big-model-1"},
		},
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		model: &types.Model{
			ID:      "big-model-1",
			Memory:  20 * 1024 * 1024 * 1024, // 20GB (increased to give runner2 higher load)
			Runtime: types.RuntimeVLLM,
		},
	}, scheduler.modelStaleFunc, scheduler.slotTimeoutFunc)
	bigSlot1.isRunning = true
	bigSlot1.LastActivityTime = time.Now().Add(-400 * time.Second) // Stale
	scheduler.slots.Store(bigSlot1.ID, bigSlot1)

	bigSlot2 := NewSlot(runner2ID, &Workload{
		llmInferenceRequest: &types.RunnerLLMInferenceRequest{
			Request: &openai.ChatCompletionRequest{Model: "big-model-2"},
		},
		WorkloadType: WorkloadTypeLLMInferenceRequest,
		model: &types.Model{
			ID:      "big-model-2",
			Memory:  4 * 1024 * 1024 * 1024, // 4GB
			Runtime: types.RuntimeVLLM,
		},
	}, scheduler.modelStaleFunc, scheduler.slotTimeoutFunc)
	bigSlot2.isRunning = true
	bigSlot2.LastActivityTime = time.Now().Add(-400 * time.Second) // Stale
	scheduler.slots.Store(bigSlot2.ID, bigSlot2)

	// Total runner2 load: 24GB (20GB + 4GB), but both are evictable
	// Runner1 load: 24GB vs Runner2 load: 24GB - make runner1 slightly lower
	// Let's make runner1 have 23GB instead by reducing the small slot to 1GB

	// Now try to schedule the embedding model (2GB)
	// This should:
	// 1. Pick runner1 first (lower load: 23GB < 24GB)
	// 2. Try to free memory on runner1 via deleteMostStaleStrategy
	// 3. Find it can only free 2GB (small slot), but needs 4GB total (21GB qwen + 2GB new)
	// 4. Fail with ErrRunnersAreFull
	// 5. (BUG): Should retry with runner2, but currently doesn't

	// Enqueue the embedding work
	err = scheduler.Enqueue(embeddingWorkload)
	require.NoError(t, err)

	// Trigger slot reconciliation to test the fix

	// Trigger slot reconciliation manually
	scheduler.reconcileSlotsOnce(ctx)

	// Check scheduling decisions to verify the embedding model was scheduled successfully on runner2
	decisions := scheduler.GetSchedulingDecisions(100)

	t.Logf("Total scheduling decisions: %d", len(decisions))

	var embeddingDecision *types.SchedulingDecision
	for _, decision := range decisions {
		t.Logf("Decision: Type=%s, Model=%s, RunnerID=%s, Success=%t, Reason=%s",
			decision.DecisionType, decision.ModelName, decision.RunnerID, decision.Success, decision.Reason)
		if decision.ModelName == "embedding-model" && decision.DecisionType == types.SchedulingDecisionTypeCreateNewSlot {
			embeddingDecision = decision
		}
	}

	// This test should now PASS with the fix
	require.NotNil(t, embeddingDecision, "Embedding model should be scheduled successfully")
	require.True(t, embeddingDecision.Success, "Embedding model scheduling should succeed")
	require.Equal(t, runner2ID, embeddingDecision.RunnerID, "Embedding should be scheduled on runner2, not runner1")
	require.Contains(t, embeddingDecision.Reason, "attempt 2/2", "Should show fallback to second runner worked")
}
