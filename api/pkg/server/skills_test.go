package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apiskill "github.com/helixml/helix/api/pkg/agent/skill/api_skills"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newTestSkillManager(t *testing.T) *apiskill.Manager {
	t.Helper()
	m := apiskill.NewManager()
	require.NoError(t, m.LoadSkills(context.Background()))
	return m
}

// TestEnableSkill_CodeIntelligence verifies that POST /api/v1/apps/{id}/skills/code-intelligence/enable
// adds an AssistantMCP entry with the Kodit URL and the user's API key.
func TestEnableSkill_CodeIntelligence(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	serverURL := "http://helix.example.com"
	userID := "user-1"
	appID := "app-abc"
	apiKey := "hl-test-key-123"

	app := &types.App{
		ID:    appID,
		Owner: userID,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{Name: "My Agent"},
				},
			},
		},
	}

	// Expect: get app → authorize (owner match) → list API keys → update app
	mockStore.EXPECT().GetApp(gomock.Any(), appID).Return(app, nil)
	mockStore.EXPECT().ListAPIKeys(gomock.Any(), &store.ListAPIKeysQuery{
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
		Type:      types.APIkeytypeAPI,
	}).Return([]*types.ApiKey{{Key: apiKey}}, nil)
	mockStore.EXPECT().UpdateApp(gomock.Any(), gomock.Any()).DoAndReturn(func(_ interface{}, updated *types.App) (*types.App, error) {
		return updated, nil
	})

	srv := &HelixAPIServer{
		Store: mockStore,
		Cfg: &config.ServerConfig{
			WebServer: config.WebServer{URL: serverURL},
		},
		skillManager: newTestSkillManager(t),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/"+appID+"/skills/code-intelligence/enable", nil)
	req = mux.SetURLVars(req, map[string]string{"id": appID, "skill": "code-intelligence"})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID, Type: types.OwnerTypeUser}))

	rr := httptest.NewRecorder()
	srv.handleEnableSkill(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var returned types.App
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&returned))

	mcps := returned.Config.Helix.Assistants[0].MCPs
	require.Len(t, mcps, 1, "expected one MCP entry")
	assert.Equal(t, "Code Intelligence", mcps[0].Name)
	assert.Equal(t, serverURL+"/api/v1/mcp/kodit", mcps[0].URL)
	assert.Equal(t, "Bearer "+apiKey, mcps[0].Headers["Authorization"])
	assert.Equal(t, "http", mcps[0].Transport)
}

// TestEnableSkill_NoAPIKey verifies that the endpoint returns an error when the user has no API key.
func TestEnableSkill_NoAPIKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	userID := "user-nokey"
	appID := "app-nokey"

	app := &types.App{
		ID:    appID,
		Owner: userID,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{Name: "Agent"}},
			},
		},
	}

	mockStore.EXPECT().GetApp(gomock.Any(), appID).Return(app, nil)
	mockStore.EXPECT().ListAPIKeys(gomock.Any(), gomock.Any()).Return([]*types.ApiKey{}, nil)

	srv := &HelixAPIServer{
		Store: mockStore,
		Cfg: &config.ServerConfig{
			WebServer: config.WebServer{URL: "http://helix.example.com"},
		},
		skillManager: newTestSkillManager(t),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/"+appID+"/skills/code-intelligence/enable", nil)
	req = mux.SetURLVars(req, map[string]string{"id": appID, "skill": "code-intelligence"})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID, Type: types.OwnerTypeUser}))

	rr := httptest.NewRecorder()
	srv.handleEnableSkill(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestEnableSkill_NotMCPSkill verifies that trying to enable a non-autoProvision skill
// (e.g. github) via this endpoint returns a 400.
func TestEnableSkill_NotMCPSkill(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	userID := "user-1"
	appID := "app-abc"

	app := &types.App{ID: appID, Owner: userID}
	mockStore.EXPECT().GetApp(gomock.Any(), appID).Return(app, nil)

	srv := &HelixAPIServer{
		Store:        mockStore,
		Cfg:          &config.ServerConfig{},
		skillManager: newTestSkillManager(t),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/"+appID+"/skills/github/enable", nil)
	req = mux.SetURLVars(req, map[string]string{"id": appID, "skill": "github"})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID, Type: types.OwnerTypeUser}))

	rr := httptest.NewRecorder()
	srv.handleEnableSkill(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
