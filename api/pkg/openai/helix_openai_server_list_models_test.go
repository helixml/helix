package openai

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/inferencerouter"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestListModels_FiltersOutModelsNoRunnerCanServe is the regression test for
// the picker-vs-router mismatch we hit on yd.helix.ml on 2026-06-17. A user
// could pick Qwen2.5-VL-3B-Instruct from the dropdown even though the only
// connected runner had qwen2.5-0.5b in its active Runner Profile. The chat
// then errored with "model X is not available (currently configured: Y)"
// from the inferencerouter.
//
// Pre-fix: ListModels included VLLM rows even when no runner had them in
// an active profile, on the theory the scheduler would pull-and-start them
// on demand. Post-sandbox-absorbs-runner pivot that's false: a VLLM model
// only runs if it's in an active Runner Profile. The picker has to mirror
// what the router can actually route.
func TestListModels_FiltersOutModelsNoRunnerCanServe(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{
		{ID: "qwen2.5-0.5b", Type: types.ModelTypeChat, Runtime: types.RuntimeVLLM, Enabled: true},
		{ID: "Qwen/Qwen2.5-VL-3B-Instruct", Type: types.ModelTypeChat, Runtime: types.RuntimeVLLM, Enabled: true},
		{ID: "Qwen/Qwen2.5-VL-7B-Instruct", Type: types.ModelTypeChat, Runtime: types.RuntimeVLLM, Enabled: true},
		{ID: "BAAI/bge-small-en", Type: types.ModelTypeEmbed, Runtime: types.RuntimeVLLM, Enabled: true},
	}, nil)

	rtr := inferencerouter.NewRouter()
	rtr.SetRunnerState(&inferencerouter.RunnerState{
		ID:     "runner-1",
		URL:    "http://10.0.0.5",
		Status: "running",
		ActiveProfile: &types.RunnerProfile{
			ID:   "rprof_qwen_0_5b",
			Name: "qwen-0.5b",
			Models: []types.ProfileModel{
				{Name: "qwen2.5-0.5b", ContainerName: "vllm-tiny", InternalPort: 8000},
			},
		},
	})

	srv := NewInternalHelixServer(&config.ServerConfig{}, mockStore, nil)
	srv.SetInferenceRouter(rtr)

	models, err := srv.ListModels(context.Background())
	require.NoError(t, err)

	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, m.ID)
	}
	require.ElementsMatch(t, []string{"qwen2.5-0.5b"}, ids,
		"picker must only include models a connected runner can serve right now; "+
			"VLLM rows that aren't in any active profile MUST be filtered (the bug fixed here)")
}

// TestListModels_FiltersOutModelsWhenRunnerNotRunning verifies that a runner
// in pulling/starting/assigning state contributes nothing to the picker.
// Otherwise we'd advertise a model that PickRunner can't actually pick yet
// (RouteableModels gates on Status=="running"), and the user would still hit
// "model X is not available".
func TestListModels_FiltersOutModelsWhenRunnerNotRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{
		{ID: "qwen2.5-0.5b", Type: types.ModelTypeChat, Runtime: types.RuntimeVLLM, Enabled: true},
	}, nil)

	rtr := inferencerouter.NewRouter()
	rtr.SetRunnerState(&inferencerouter.RunnerState{
		ID:     "runner-1",
		URL:    "http://10.0.0.5",
		Status: "pulling",
		ActiveProfile: &types.RunnerProfile{
			Models: []types.ProfileModel{
				{Name: "qwen2.5-0.5b"},
			},
		},
	})

	srv := NewInternalHelixServer(&config.ServerConfig{}, mockStore, nil)
	srv.SetInferenceRouter(rtr)

	models, err := srv.ListModels(context.Background())
	require.NoError(t, err)
	require.Empty(t, models,
		"a runner that isn't running can't route; its profile must not populate the picker")
}

// TestListModels_NilRouterReturnsEmpty guards the partially-constructed
// server case (SetInferenceRouter not called yet). Returning ALL DB models
// here would re-introduce the original bug at cold start.
func TestListModels_NilRouterReturnsEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{
		{ID: "qwen2.5-0.5b", Type: types.ModelTypeChat, Runtime: types.RuntimeVLLM, Enabled: true},
	}, nil)

	srv := NewInternalHelixServer(&config.ServerConfig{}, mockStore, nil)
	// deliberately no SetInferenceRouter

	models, err := srv.ListModels(context.Background())
	require.NoError(t, err)
	require.Empty(t, models, "with no router, nothing is routeable, so the picker must be empty")
}
