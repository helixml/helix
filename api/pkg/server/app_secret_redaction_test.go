package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// secretValues are planted in every credential field of the test agent; none
// of them may appear in a reader's response.
var secretValues = []string{
	"app-secret-value", "api-header-value", "api-query-value", "mcp-header-value",
	"mcp-env-value", "zapier-key-value", "ado-pat-value", "web-password-value",
	"firecrawl-key-value", "tool-header-value", "slack-app-value", "slack-bot-value",
	"teams-password-value", "crisp-token-value",
}

func secretBearingApp() *types.App {
	return &types.App{
		ID:     "app_shared",
		Owner:  "usr_author",
		Global: true,
		Config: types.AgentConfig{
			Secrets: map[string]string{"API_TOKEN": "app-secret-value"},
			Helix: types.AgentHelixConfig{
				Name: "shared",
				Assistants: []types.AssistantConfig{{
					Name: "main",
					APIs: []types.AssistantAPI{{
						Name:    "weather",
						URL:     "https://api.example.com",
						Headers: map[string]string{"Authorization": "api-header-value"},
						Query:   map[string]string{"key": "api-query-value"},
					}},
					MCPs: []types.AssistantMCP{{
						Name:    "tracker",
						Headers: map[string]string{"Authorization": "mcp-header-value"},
						Env:     map[string]string{"TOKEN": "mcp-env-value"},
					}},
					Zapier:      []types.AssistantZapier{{Name: "zap", APIKey: "zapier-key-value"}},
					AzureDevOps: types.AssistantAzureDevOps{Enabled: true, PersonalAccessToken: "ado-pat-value"},
					Knowledge: []*types.AssistantKnowledge{{
						Name: "docs",
						Source: types.KnowledgeSource{Web: &types.KnowledgeSourceWeb{
							URLs:    []string{"https://docs.example.com"},
							Auth:    types.KnowledgeSourceWebAuth{Username: "crawler", Password: "web-password-value"},
							Crawler: &types.WebsiteCrawler{Firecrawl: &types.Firecrawl{APIKey: "firecrawl-key-value"}},
						}},
					}},
					Tools: []*types.Tool{{
						Name: "weather",
						Config: types.ToolConfig{API: &types.ToolAPIConfig{
							Headers: map[string]string{"Authorization": "tool-header-value"},
						}},
					}},
				}},
				Triggers: []types.Trigger{
					{Slack: &types.SlackTrigger{AppToken: "slack-app-value", BotToken: "slack-bot-value"}},
					{Teams: &types.TeamsTrigger{AppID: "teams-app", AppPassword: "teams-password-value"}},
					{Crisp: &types.CrispTrigger{Identifier: "crisp-id", Token: "crisp-token-value"}},
				},
			},
		},
	}
}

func requireNoSecrets(t *testing.T, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	for _, secret := range secretValues {
		require.NotContains(t, string(raw), secret)
	}
}

func requireAllSecrets(t *testing.T, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	for _, secret := range secretValues {
		require.Contains(t, string(raw), secret)
	}
}

func agentRequest(method, id string, user types.User) *http.Request {
	req := httptest.NewRequest(method, "/api/v1/agents/"+id, nil)
	req = req.WithContext(setRequestUser(context.Background(), user))
	return mux.SetURLVars(req, map[string]string{"id": id})
}

func TestGetAgent_ReaderGetsMaskedCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetApp(gomock.Any(), "app_shared").DoAndReturn(
		func(context.Context, string) (*types.App, error) { return secretBearingApp(), nil }).AnyTimes()
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}

	app, httpErr := server.getAgent(nil, agentRequest(http.MethodGet, "app_shared", types.User{ID: "usr_reader"}))
	require.Nil(t, httpErr)
	requireNoSecrets(t, app)
	require.Equal(t, maskedSecret, app.Config.Secrets["API_TOKEN"])
	require.Equal(t, maskedSecret, app.Config.Helix.Triggers[0].Slack.BotToken)
	require.Equal(t, "crawler", app.Config.Helix.Assistants[0].Knowledge[0].Source.Web.Auth.Username)

	app, httpErr = server.getAgent(nil, agentRequest(http.MethodGet, "app_shared", types.User{ID: "usr_author"}))
	require.Nil(t, httpErr)
	requireAllSecrets(t, app)

	app, httpErr = server.getAgent(nil, agentRequest(http.MethodGet, "app_shared", types.User{ID: "usr_admin", Admin: true}))
	require.Nil(t, httpErr)
	requireAllSecrets(t, app)
}

func TestListAgents_GlobalAgentsMaskedForReaders(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListApps(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, q *store.ListAppsQuery) ([]*types.App, error) {
			if q.Global {
				return []*types.App{secretBearingApp()}, nil
			}
			return nil, nil
		}).AnyTimes()
	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{ID: "usr_author"}, nil).AnyTimes()
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "usr_reader"}))
	apps, httpErr := server.listAgents(nil, req)
	require.Nil(t, httpErr)
	require.Len(t, apps, 1)
	requireNoSecrets(t, apps)
}

// Duplicating an agent you can only read must not hand you its credentials.
func TestDuplicateApp_ReaderCopyHasNoCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetApp(gomock.Any(), "app_shared").DoAndReturn(
		func(context.Context, string) (*types.App, error) { return secretBearingApp(), nil })
	var created *types.App
	mockStore.EXPECT().CreateApp(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, app *types.App) (*types.App, error) { created = app; return app, nil })
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}

	_, httpErr := server.duplicateApp(nil, agentRequest(http.MethodPost, "app_shared", types.User{ID: "usr_reader"}))
	require.Nil(t, httpErr)
	require.NotNil(t, created)
	requireNoSecrets(t, created)
	require.NotContains(t, created.Config.Secrets, "API_TOKEN")
}

func TestDuplicateApp_OwnerCopyKeepsCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetApp(gomock.Any(), "app_shared").DoAndReturn(
		func(context.Context, string) (*types.App, error) { return secretBearingApp(), nil })
	var created *types.App
	mockStore.EXPECT().CreateApp(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, app *types.App) (*types.App, error) { created = app; return app, nil })
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}

	_, httpErr := server.duplicateApp(nil, agentRequest(http.MethodPost, "app_shared", types.User{ID: "usr_author"}))
	require.Nil(t, httpErr)
	requireAllSecrets(t, created)
}

// A client that saves back what it read sends masked placeholders; they must
// keep the stored credentials, while new values replace them.
func TestRestoreMaskedAgentSecrets(t *testing.T) {
	stored := secretBearingApp().Config
	update := maskedAgentConfig(stored)
	update.Secrets["NEW"] = "new-value"
	update.Helix.Triggers[0].Slack.BotToken = "rotated-bot-token"
	update.Helix.Assistants[0].MCPs[0].Headers["X-Ghost"] = maskedSecret

	restoreMaskedAgentSecrets(stored, &update)

	require.Equal(t, "app-secret-value", update.Secrets["API_TOKEN"])
	require.Equal(t, "new-value", update.Secrets["NEW"])
	require.Equal(t, "rotated-bot-token", update.Helix.Triggers[0].Slack.BotToken)
	require.Equal(t, "slack-app-value", update.Helix.Triggers[0].Slack.AppToken)
	require.Equal(t, "api-header-value", update.Helix.Assistants[0].APIs[0].Headers["Authorization"])
	require.Equal(t, "mcp-env-value", update.Helix.Assistants[0].MCPs[0].Env["TOKEN"])
	require.Equal(t, "web-password-value", update.Helix.Assistants[0].Knowledge[0].Source.Web.Auth.Password)
	require.Equal(t, "crisp-token-value", update.Helix.Triggers[2].Crisp.Token)
	require.NotContains(t, update.Helix.Assistants[0].MCPs[0].Headers, "X-Ghost",
		"a placeholder with no stored value must not be saved")

	// The stored config is not modified by masking or restoring.
	require.Equal(t, "app-secret-value", stored.Secrets["API_TOKEN"])
}

func evaluationRunRequest(agentID, runID string, user types.User) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID+"/evaluation-runs/"+runID, nil)
	req = req.WithContext(setRequestUser(context.Background(), user))
	return mux.SetURLVars(req, map[string]string{"agent_id": agentID, "run_id": runID})
}

// An evaluation run snapshots the agent config. Org members who may read but
// not edit the agent must get the snapshot masked, and a run is only reachable
// through the agent it belongs to.
func TestGetEvaluationRun_SnapshotMaskedAndScopedToAgent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	app := secretBearingApp()
	app.Global = false
	app.OrganizationID = "org_1"
	mockStore.EXPECT().GetAppWithTools(gomock.Any(), "app_shared").Return(app, nil).AnyTimes()
	mockStore.EXPECT().GetAppWithTools(gomock.Any(), "app_other").Return(&types.App{ID: "app_other", Owner: "usr_member"}, nil).AnyTimes()
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(
		&types.OrganizationMembership{OrganizationID: "org_1", UserID: "usr_member", Role: types.OrganizationRoleMember}, nil).AnyTimes()
	mockStore.EXPECT().ListTeams(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().ListAccessGrants(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().ListProjects(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().GetEvaluationRun(gomock.Any(), "run_1").DoAndReturn(
		func(context.Context, string) (*types.EvaluationRun, error) {
			return &types.EvaluationRun{ID: "run_1", AppID: "app_shared", AppConfigSnapshot: secretBearingApp().Config}, nil
		}).AnyTimes()
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}

	run, httpErr := server.getEvaluationRun(nil, evaluationRunRequest("app_shared", "run_1", types.User{ID: "usr_member"}))
	require.Nil(t, httpErr)
	requireNoSecrets(t, run)

	run, httpErr = server.getEvaluationRun(nil, evaluationRunRequest("app_shared", "run_1", types.User{ID: "usr_author"}))
	require.Nil(t, httpErr)
	requireAllSecrets(t, run)

	_, httpErr = server.getEvaluationRun(nil, evaluationRunRequest("app_other", "run_1", types.User{ID: "usr_member"}))
	require.NotNil(t, httpErr)
	require.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

// Removing an entry must not shift a stored credential onto the entry that
// moves into its place.
func TestRestoreMaskedAgentSecrets_DeletedTriggerDoesNotShift(t *testing.T) {
	stored := types.AgentConfig{Helix: types.AgentHelixConfig{Triggers: []types.Trigger{
		{Slack: &types.SlackTrigger{AppToken: "slack-app-value", BotToken: "slack-bot-value"}},
		{Crisp: &types.CrispTrigger{Identifier: "crisp-id", Token: "crisp-token-value"}},
	}}}
	update := maskedAgentConfig(stored)
	update.Helix.Triggers = update.Helix.Triggers[1:] // the Slack trigger is removed

	restoreMaskedAgentSecrets(stored, &update)

	require.Len(t, update.Helix.Triggers, 1)
	require.Equal(t, "crisp-token-value", update.Helix.Triggers[0].Crisp.Token)
	requireNoSlackValues(t, update)
}

func requireNoSlackValues(t *testing.T, cfg types.AgentConfig) {
	t.Helper()
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "slack-bot-value")
	require.NotContains(t, string(raw), "slack-app-value")
}

// Entries that share a name cannot be told apart, so a placeholder on either
// is dropped rather than given the other's value.
func TestRestoreMaskedAgentSecrets_DuplicateNamesAreNotCrossAssigned(t *testing.T) {
	stored := types.AgentConfig{Helix: types.AgentHelixConfig{Assistants: []types.AssistantConfig{{
		Name: "main",
		APIs: []types.AssistantAPI{
			{Name: "api", Headers: map[string]string{"Authorization": "first-value"}},
			{Name: "api", Headers: map[string]string{"Authorization": "second-value"}},
		},
	}}}}
	update := maskedAgentConfig(stored)

	restoreMaskedAgentSecrets(stored, &update)

	for _, api := range update.Helix.Assistants[0].APIs {
		require.NotContains(t, api.Headers, "Authorization")
	}
}
