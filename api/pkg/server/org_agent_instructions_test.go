package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"

	"go.uber.org/mock/gomock"
)

// TestUpdateAppFiresOrgAgentInstructionsChangedWhenSystemPromptChanges proves
// the App-side seam of the restart-required signal: saving a changed system
// prompt through the REST app-update path calls
// s.orgAgentInstructionsChanged with the App's own id. It does NOT prove the
// downstream Node resolution/stamping — that is covered separately by
// TestStampRestartRequiredForApp_*.
func TestUpdateAppFiresOrgAgentInstructionsChangedWhenSystemPromptChanges(t *testing.T) {
	ctrl := gomock.NewController(t)
	helixStore := store.NewMockStore(ctrl)

	existing := &types.App{
		ID:    "app-prompt",
		Owner: "user-test",
		Config: types.AppConfig{Helix: types.AppHelixConfig{
			Name:       "Prompted",
			Assistants: []types.AssistantConfig{{Name: "Prompted", SystemPrompt: "old instructions"}},
		}},
	}
	helixStore.EXPECT().GetApp(gomock.Any(), existing.ID).Return(existing, nil)
	helixStore.EXPECT().UpdateApp(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, updated *types.App) (*types.App, error) {
		return updated, nil
	})
	helixStore.EXPECT().ListKnowledge(gomock.Any(), gomock.Any()).Return(nil, nil)
	helixStore.EXPECT().ListTriggerConfigurations(gomock.Any(), gomock.Any()).Return(nil, nil)

	var firedForAppID string
	server := &HelixAPIServer{
		Store: helixStore,
		orgAgentInstructionsChanged: func(_ context.Context, appID string) {
			firedForAppID = appID
		},
	}

	update := *existing
	update.Config.Helix.Assistants = []types.AssistantConfig{{Name: "Prompted", SystemPrompt: "new instructions"}}
	body, err := json.Marshal(update)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, "/api/v1/agents/"+existing.ID, bytes.NewReader(body))
	require.NoError(t, err)
	req = mux.SetURLVars(req, map[string]string{"id": existing.ID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: existing.Owner}))

	updated, httpErr := server.updateAgent(nil, req)

	require.Nil(t, httpErr)
	require.NotNil(t, updated)
	require.Equal(t, existing.ID, firedForAppID)
}

// TestUpdateAppDoesNotFireOrgAgentInstructionsChangedWhenSystemPromptUnchanged
// is the no-op-save guard: a save that leaves the system prompt untouched
// (only the display name changes here) must not arm the restart-required
// flag.
func TestUpdateAppDoesNotFireOrgAgentInstructionsChangedWhenSystemPromptUnchanged(t *testing.T) {
	ctrl := gomock.NewController(t)
	helixStore := store.NewMockStore(ctrl)

	existing := &types.App{
		ID:    "app-noop",
		Owner: "user-test",
		Config: types.AppConfig{Helix: types.AppHelixConfig{
			Name:       "Unchanged",
			Assistants: []types.AssistantConfig{{Name: "Unchanged", SystemPrompt: "same instructions"}},
		}},
	}
	helixStore.EXPECT().GetApp(gomock.Any(), existing.ID).Return(existing, nil)
	helixStore.EXPECT().UpdateApp(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, updated *types.App) (*types.App, error) {
		return updated, nil
	})
	helixStore.EXPECT().ListKnowledge(gomock.Any(), gomock.Any()).Return(nil, nil)
	helixStore.EXPECT().ListTriggerConfigurations(gomock.Any(), gomock.Any()).Return(nil, nil)

	fired := false
	server := &HelixAPIServer{
		Store: helixStore,
		orgAgentInstructionsChanged: func(context.Context, string) {
			fired = true
		},
	}

	// Only the display name changes; SystemPrompt is identical.
	update := *existing
	update.Config.Helix.Name = "Renamed"
	update.Config.Helix.Assistants = []types.AssistantConfig{{Name: "Unchanged", SystemPrompt: "same instructions"}}
	body, err := json.Marshal(update)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, "/api/v1/agents/"+existing.ID, bytes.NewReader(body))
	require.NoError(t, err)
	req = mux.SetURLVars(req, map[string]string{"id": existing.ID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: existing.Owner}))

	updated, httpErr := server.updateAgent(nil, req)

	require.Nil(t, httpErr)
	require.NotNil(t, updated)
	require.False(t, fired, "no-op system prompt save must not arm restart-required")
}

// TestStampRestartRequiredForApp_ResolvesCorrectNodeAmongSeveralBots proves
// the App→Node resolution: with several Bots in the org, only the Node
// whose AgentID matches the edited App gets its restart-required container
// stamped.
func TestStampRestartRequiredForApp_ResolvesCorrectNodeAmongSeveralBots(t *testing.T) {
	ctx := context.Background()
	st := orgmemory.New()
	now := time.Now().UTC()

	other, err := orgchart.NewNode("b-other", "# Other", nil, now, "org-multi")
	require.NoError(t, err)
	require.NoError(t, st.Nodes.Create(ctx, other.WithAgentID("app-other")))

	target, err := orgchart.NewNode("b-target", "# Target", nil, now, "org-multi")
	require.NoError(t, err)
	require.NoError(t, st.Nodes.Create(ctx, target.WithAgentID("app-target")))

	session := &types.Session{ID: "ses-target"}
	session.Metadata.ContainerID = "container-target"
	require.NoError(t, runtimehelix.SaveSession(ctx, st, "org-multi", "b-target", "ses-target"))

	apps := fakeAppGetter{app: &types.App{ID: "app-target", OrganizationID: "org-multi"}}
	sessions := stampSessionsBySessionID{sessions: map[string]*types.Session{"ses-target": session}}

	stampRestartRequiredForApp(ctx, st, sessions, apps, "app-target")

	targetWS, err := runtimehelix.LoadState(ctx, st, "org-multi", "b-target")
	require.NoError(t, err)
	require.Equal(t, "container-target", targetWS.RestartRequiredContainer)

	// The non-matching Bot must be left untouched.
	otherWS, err := runtimehelix.LoadState(ctx, st, "org-multi", "b-other")
	require.NoError(t, err)
	require.Equal(t, "", otherWS.RestartRequiredContainer)
}

// TestStampRestartRequiredForApp_NoOpWhenAppDoesNotBackABot covers the
// common case: most Apps are not org-linked, or are org-linked but don't
// back any Bot. Neither should error or touch any Node.
func TestStampRestartRequiredForApp_NoOpWhenAppDoesNotBackABot(t *testing.T) {
	ctx := context.Background()
	st := orgmemory.New()

	apps := fakeAppGetter{app: &types.App{ID: "app-standalone", OrganizationID: ""}}
	sessions := stampSessionsBySessionID{sessions: map[string]*types.Session{}}

	// Must not panic and must not error out (there's nothing to assert
	// on besides "it returned").
	stampRestartRequiredForApp(ctx, st, sessions, apps, "app-standalone")
}

type fakeAppGetter struct{ app *types.App }

func (f fakeAppGetter) GetApp(context.Context, string) (*types.App, error) { return f.app, nil }

type stampSessionsBySessionID struct{ sessions map[string]*types.Session }

func (s stampSessionsBySessionID) GetSession(_ context.Context, id string) (*types.Session, error) {
	return s.sessions[id], nil
}
