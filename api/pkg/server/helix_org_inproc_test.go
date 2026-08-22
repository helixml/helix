package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/server/helixorg"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
)

type gormBackedInProcStore struct {
	helixstore.Store
	db *gorm.DB
}

func (s *gormBackedInProcStore) GormDB() *gorm.DB {
	return s.db
}

// newInProcTestSetup builds a NewTestServer-backed HelixAPIServer + an
// inProcHelixClient with a request user. The single setup
// covers every test in this file — each test seeds whatever store
// state it needs against the returned memorystore.
//
// We deliberately leave `Cfg.Inference` / providers etc unset: the
// adapter's ProjectService methods we exercise here don't touch those
// fields (`applyProject` does, but it isn't covered in this test
// suite — that handler's full happy path is exercised separately
// via the in-Helix end-to-end tests).
func newInProcTestSetup(t *testing.T) (*HelixAPIServer, *memorystore.MemoryStore, *inProcHelixClient, *types.User, context.Context) {
	t.Helper()
	store := memorystore.New()
	server := NewTestServer(store, pubsub.NewNoop())
	user := &types.User{
		ID:        "usr_request",
		Email:     "request@helix.local",
		FullName:  "Request User",
		Type:      types.OwnerTypeUser,
		TokenType: types.TokenTypeAPIKey,
	}
	client := NewInProcHelixClient(server)
	ctx := runtimehelix.WithUser(context.Background(), user)
	return server, store, client, user, ctx
}

func TestInProcClient_DeleteLinkedAgentPreservesConfiguredProjectAndUnsetsAgentID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&types.Project{}, &types.App{}, &types.Knowledge{}, &types.KnowledgeVersion{}))
	for _, statement := range []string{
		`CREATE TABLE org_bot_runtime_state (org_id TEXT, bot_id TEXT)`,
		`CREATE TABLE org_subscriptions (org_id TEXT, bot_id TEXT)`,
		`CREATE TABLE org_bots (org_id TEXT, id TEXT, agent_app_id TEXT)`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}

	app := &types.App{ID: "app-agent"}
	project := &types.Project{
		ID:                "project-configured",
		Name:              "Configured Project",
		Description:       "Project state must outlive its org agent",
		UserID:            "user-owner",
		OrganizationID:    "org-test",
		Status:            "active",
		DefaultRepoID:     "repo-configured",
		DefaultHelixAppID: app.ID,
		Technologies:      []string{"go", "typescript"},
	}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Create(project).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO org_bots (org_id, id, agent_app_id) VALUES (?, ?, ?)`,
		"org-test", "b-agent", app.ID,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO org_bot_runtime_state (org_id, bot_id) VALUES (?, ?)`,
		"org-test", "b-agent",
	).Error)

	client := NewInProcHelixClient(&HelixAPIServer{Store: &gormBackedInProcStore{db: db}})
	require.NoError(t, client.DeleteLinkedAgent(context.Background(), "org-test", "b-agent", app.ID, ""))

	var preserved types.Project
	require.NoError(t, db.First(&preserved, "id = ?", project.ID).Error)
	require.Empty(t, preserved.DefaultHelixAppID)
	require.Equal(t, project.Name, preserved.Name)
	require.Equal(t, project.Description, preserved.Description)
	require.Equal(t, project.UserID, preserved.UserID)
	require.Equal(t, project.OrganizationID, preserved.OrganizationID)
	require.Equal(t, project.Status, preserved.Status)
	require.Equal(t, project.DefaultRepoID, preserved.DefaultRepoID)
	require.Equal(t, project.Technologies, preserved.Technologies)
	var appCount, botCount int64
	require.NoError(t, db.Model(&types.App{}).Where("id = ?", app.ID).Count(&appCount).Error)
	require.NoError(t, db.Table("org_bots").Where("org_id = ? AND id = ?", "org-test", "b-agent").Count(&botCount).Error)
	require.Zero(t, appCount)
	require.Zero(t, botCount)

	replacement := &types.App{ID: "app-replacement"}
	require.NoError(t, db.Create(replacement).Error)
	require.NoError(t, db.Model(&preserved).Update("default_helix_app_id", replacement.ID).Error)
	require.NoError(t, db.First(&preserved, "id = ?", project.ID).Error)
	require.Equal(t, replacement.ID, preserved.DefaultHelixAppID)
}

// A failure to stop the bot's desktop session must not abort the delete
// cascade. stopExternalAgentSession 404s on an already-gone session, 400s on a
// non-zed_external session and 500s when hydra is down; treating any of those
// as fatal left the bot permanently undeletable.
func TestInProcClient_DeleteLinkedAgentContinuesWhenSessionStopFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&types.Project{}, &types.App{}, &types.Knowledge{}, &types.KnowledgeVersion{}))
	for _, statement := range []string{
		`CREATE TABLE org_bot_runtime_state (org_id TEXT, bot_id TEXT)`,
		`CREATE TABLE org_subscriptions (org_id TEXT, bot_id TEXT)`,
		`CREATE TABLE org_bots (org_id TEXT, id TEXT, agent_app_id TEXT)`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	app := &types.App{ID: "app-agent"}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO org_bots (org_id, id, agent_app_id) VALUES (?, ?, ?)`,
		"org-test", "b-agent", app.ID,
	).Error)

	// memorystore has no session seeded, so StopExternalAgent resolves to a 404.
	store := &gormBackedInProcStore{Store: memorystore.New(), db: db}
	client := NewInProcHelixClient(&HelixAPIServer{Store: store})
	ctx := runtimehelix.WithUser(context.Background(), &types.User{ID: "usr_request"})

	require.NoError(t, client.DeleteLinkedAgent(ctx, "org-test", "b-agent", app.ID, "ses_missing"))

	var appCount, botCount int64
	require.NoError(t, db.Model(&types.App{}).Where("id = ?", app.ID).Count(&appCount).Error)
	require.NoError(t, db.Table("org_bots").Where("org_id = ? AND id = ?", "org-test", "b-agent").Count(&botCount).Error)
	require.Zero(t, appCount)
	require.Zero(t, botCount)
}

// The org runtime — not the shared apply handler — is what classifies a bot's
// agent app as org_agent. applyProject leaves the app as a coding agent so the
// public apply endpoint stays usable for spec tasks.
func TestInProcClient_MarkAgentAppAsOrgKind(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := helixstore.NewMockStore(ctrl)
	existing := &types.App{ID: "app_bot", AgentKind: types.AgentKindCoding}
	st.EXPECT().GetApp(gomock.Any(), "app_bot").Return(existing, nil)
	st.EXPECT().UpdateApp(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, app *types.App) (*types.App, error) {
			require.Equal(t, types.AgentKindOrg, app.AgentKind)
			return app, nil
		},
	)

	client := NewInProcHelixClient(&HelixAPIServer{Store: st})
	require.NoError(t, client.markAgentAppAsOrgKind(context.Background(), "app_bot"))

	// Already org_agent → no write.
	already := &types.App{ID: "app_bot2", AgentKind: types.AgentKindOrg}
	st.EXPECT().GetApp(gomock.Any(), "app_bot2").Return(already, nil)
	require.NoError(t, client.markAgentAppAsOrgKind(context.Background(), "app_bot2"))
}

func TestInProcClient_UpdateAppConfigPersistsDirectlyAndPreservesApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := helixstore.NewMockStore(ctrl)
	created := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	existing := &types.App{
		ID:             "app_bot",
		Created:        created,
		Updated:        created.Add(time.Hour),
		OrganizationID: "org_test",
		Owner:          "usr_owner",
		OwnerType:      types.OwnerTypeUser,
		Global:         true,
		AgentKind:      types.AgentKindOrg,
		Config:         types.AppConfig{Helix: types.AppHelixConfig{Name: "old"}},
		User:           types.User{ID: "usr_owner", Email: "owner@helix.local"},
	}
	wantConfig := types.AppConfig{Helix: types.AppHelixConfig{Name: "updated"}}
	wantApp := *existing
	wantApp.Config = wantConfig

	st.EXPECT().GetApp(gomock.Any(), existing.ID).Return(existing, nil)
	st.EXPECT().UpdateApp(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, app *types.App) (*types.App, error) {
			require.Same(t, existing, app)
			require.Equal(t, &wantApp, app)
			return app, nil
		},
	)

	client := NewInProcHelixClient(&HelixAPIServer{Store: st})
	require.NoError(t, client.UpdateAppConfig(context.Background(), existing.ID, wantConfig))
}

func TestInProcClient_ResolvesOrganizationOwnerWithoutAdmin(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := helixstore.NewMockStore(ctrl)
	owner := &types.User{ID: "usr_owner", Admin: false}
	st.EXPECT().GetOrganization(gomock.Any(), &helixstore.GetOrganizationQuery{ID: "org_test"}).
		Return(&types.Organization{ID: "org_test", Owner: owner.ID}, nil)
	st.EXPECT().GetUser(gomock.Any(), &helixstore.GetUserQuery{ID: owner.ID}).Return(owner, nil)

	client := NewInProcHelixClient(&HelixAPIServer{Store: st})
	ctx := runtimehelix.WithHelixIdentity(context.Background(), runtimehelix.HelixIdentity{OrganizationID: "org_test"})
	got, err := client.resolveUser(ctx)

	require.NoError(t, err)
	require.Same(t, owner, got)
}

func TestInProcClient_CreateAgentUsesOrganizationOwnerWithoutRequestUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := helixstore.NewMockStore(ctrl)
	owner := &types.User{ID: "usr_owner"}
	st.EXPECT().GetOrganization(gomock.Any(), &helixstore.GetOrganizationQuery{ID: "org_test"}).
		Return(&types.Organization{ID: "org_test", Owner: owner.ID}, nil)
	st.EXPECT().GetUser(gomock.Any(), &helixstore.GetUserQuery{ID: owner.ID}).Return(owner, nil)
	st.EXPECT().CreateApp(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, app *types.App) (*types.App, error) {
			require.Equal(t, owner.ID, app.Owner)
			require.Equal(t, "org_test", app.OrganizationID)
			require.Equal(t, types.AgentKindOrg, app.AgentKind)
			app.ID = "app_test"
			return app, nil
		},
	)

	client := NewInProcHelixClient(&HelixAPIServer{Store: st})
	appID, err := client.CreateAgent(context.Background(), "org_test", "Chief of Staff", "Lead", lifecycle.AgentConfig{})

	require.NoError(t, err)
	require.Equal(t, "app_test", appID)
}

func TestInProcClient_CreateAgentUsesConfiguredOrgDefaults(t *testing.T) {
	ctx := context.Background()
	reg := configregistry.New(orggorm.GetOrgTestDB(t).Configs)
	helixorg.RegisterConfigSpecs(reg)
	err := reg.Set(ctx, "org-test", configregistry.DefaultAgentConfigKey,
		`{"code_agent_runtime":"codex_cli","code_agent_credential_type":"subscription","model":"gpt-5.6","reasoning_effort":"high"}`)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	st := helixstore.NewMockStore(ctrl)
	owner := &types.User{ID: "usr_owner"}
	st.EXPECT().GetOrganization(gomock.Any(), &helixstore.GetOrganizationQuery{Name: "org-test"}).
		Return(&types.Organization{ID: "org-test", Owner: owner.ID}, nil)
	st.EXPECT().GetUser(gomock.Any(), &helixstore.GetUserQuery{ID: owner.ID}).Return(owner, nil)
	st.EXPECT().CreateApp(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, app *types.App) (*types.App, error) {
			assistant := app.Config.Helix.Assistants[0]
			require.Equal(t, types.CodeAgentRuntimeCodexCLI, assistant.CodeAgentRuntime)
			require.Equal(t, types.CodeAgentCredentialTypeSubscription, assistant.CodeAgentCredentialType)
			require.Equal(t, "gpt-5.6", assistant.Model)
			require.Equal(t, "high", assistant.ReasoningEffort)
			app.ID = "app_test"
			return app, nil
		},
	)

	client := NewInProcHelixClient(&HelixAPIServer{Store: st}, reg)
	_, err = client.CreateAgent(ctx, "org-test", "Engineer", "Build", lifecycle.AgentConfig{})
	require.NoError(t, err)
}

func TestInProcClient_CreateAgentUsesExplicitConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := helixstore.NewMockStore(ctrl)
	ctx := runtimehelix.WithUser(context.Background(), &types.User{ID: "usr-owner"})
	st.EXPECT().CreateApp(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, app *types.App) (*types.App, error) {
			assistant := app.Config.Helix.Assistants[0]
			require.Equal(t, types.CodeAgentRuntimeClaudeCode, assistant.CodeAgentRuntime)
			require.Equal(t, types.CodeAgentCredentialTypeAPIKey, assistant.CodeAgentCredentialType)
			require.Equal(t, "anthropic", assistant.Provider)
			require.Equal(t, "claude-opus-4-6", assistant.Model)
			require.Equal(t, "high", assistant.ReasoningEffort)
			app.ID = "app-test"
			return app, nil
		},
	)
	client := NewInProcHelixClient(&HelixAPIServer{Store: st})

	_, err := client.CreateAgent(ctx, "org-test", "Engineer", "Build", lifecycle.AgentConfig{
		CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
		CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
		Provider:                "anthropic",
		Model:                   "claude-opus-4-6",
		ReasoningEffort:         "high",
	})
	require.NoError(t, err)
}

func TestInProcClient_CreateAgentRejectsIncompatibleHarnessModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := helixstore.NewMockStore(ctrl)
	ctx := runtimehelix.WithUser(context.Background(), &types.User{ID: "usr-owner"})
	client := NewInProcHelixClient(&HelixAPIServer{Store: st})

	_, err := client.CreateAgent(ctx, "org-test", "Engineer", "Build", lifecycle.AgentConfig{
		CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
		CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
		Provider:                "openai",
		Model:                   "gpt-5.6-sol",
	})

	require.ErrorContains(t, err, "claude_code requires an Anthropic Claude model")
}

func TestInProcClient_DeferredDefaultsApplyOnlyToUntouchedScaffold(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := helixstore.NewMockStore(ctrl)
	ctx := context.Background()
	app := &types.App{
		ID:    "app-deferred",
		Owner: "usr-owner",
		Config: types.AppConfig{Helix: types.AppHelixConfig{
			Assistants: []types.AssistantConfig{{
				Name: "Agent", AgentType: types.AgentTypeZedExternal,
				CodeAgentRuntime: types.CodeAgentRuntimeZedAgent,
				ReasoningEffort:  types.ReasoningEffortNone,
			}},
		}},
	}
	st.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)
	st.EXPECT().UpdateApp(gomock.Any(), app).Return(app, nil)
	st.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)
	client := NewInProcHelixClient(&HelixAPIServer{Store: st})
	defaults := types.AssistantConfig{
		CodeAgentRuntime:        types.CodeAgentRuntimeZedAgent,
		CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
		Provider:                "anthropic",
		Model:                   "claude-opus-4-6",
		ReasoningEffort:         "high",
	}

	require.NoError(t, client.ApplyAgentDefaults(ctx, app.ID, defaults))
	require.Equal(t, "anthropic", app.Config.Helix.Assistants[0].Provider)
	require.Equal(t, "claude-opus-4-6", app.Config.Helix.Assistants[0].Model)

	app.Config.Helix.Assistants[0].CodeAgentRuntime = types.CodeAgentRuntimeCodexCLI
	app.Config.Helix.Assistants[0].Model = "user-selected"
	require.NoError(t, client.ApplyAgentDefaults(ctx, app.ID, defaults))
	require.Equal(t, types.CodeAgentRuntimeCodexCLI, app.Config.Helix.Assistants[0].CodeAgentRuntime)
	require.Equal(t, "user-selected", app.Config.Helix.Assistants[0].Model)
}

func TestInProcClient_DeleteProjectNormalizesGormNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := helixstore.NewMockStore(ctrl)
	st.EXPECT().GetProject(gomock.Any(), "prj_deleted").Return(nil, gorm.ErrRecordNotFound)
	client := NewInProcHelixClient(&HelixAPIServer{Store: st})

	err := client.DeleteProject(context.Background(), "prj_deleted")

	require.ErrorIs(t, err, runtimehelix.ErrProjectNotFound)
}

func TestInProcClient_UpdateAgentRestoresAppAfterPostSaveFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := helixstore.NewMockStore(ctrl)
	user := &types.User{ID: "usr_owner", Type: types.OwnerTypeUser}
	existing := &types.App{
		ID:        "app_test",
		Owner:     user.ID,
		OwnerType: types.OwnerTypeUser,
		Config: types.AppConfig{Helix: types.AppHelixConfig{
			Name: "Agent",
			Assistants: []types.AssistantConfig{{
				Name:         "Agent",
				SystemPrompt: "old instructions",
			}},
		}},
	}
	handlerExisting := *existing
	handlerExisting.Config.Helix.Assistants = append([]types.AssistantConfig(nil), existing.Config.Helix.Assistants...)
	gomock.InOrder(
		st.EXPECT().GetApp(gomock.Any(), existing.ID).Return(existing, nil),
		st.EXPECT().GetApp(gomock.Any(), existing.ID).Return(&handlerExisting, nil),
	)
	var savedPrompt, restoredPrompt string
	st.EXPECT().UpdateApp(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, app *types.App) (*types.App, error) {
			savedPrompt = app.Config.Helix.Assistants[0].SystemPrompt
			return app, nil
		},
	)
	st.EXPECT().ListKnowledge(gomock.Any(), &helixstore.ListKnowledgeQuery{AppID: existing.ID}).
		Return([]*types.Knowledge{}, nil)
	st.EXPECT().ListTriggerConfigurations(gomock.Any(), &helixstore.ListTriggerConfigurationsQuery{AppID: existing.ID}).
		Return(nil, errors.New("trigger reconciliation failed"))
	st.EXPECT().UpdateApp(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, app *types.App) (*types.App, error) {
			restoredPrompt = app.Config.Helix.Assistants[0].SystemPrompt
			return app, nil
		},
	)

	client := NewInProcHelixClient(&HelixAPIServer{
		Store:      st,
		Cfg:        &config.ServerConfig{},
		Controller: &controller.Controller{},
	})
	ctx := runtimehelix.WithUser(context.Background(), user)
	instructions := "new instructions"

	err := client.UpdateAgent(ctx, existing.ID, orgapi.AgentConfigPatch{}, nil, &instructions)

	require.ErrorContains(t, err, "trigger reconciliation failed")
	require.Equal(t, instructions, savedPrompt)
	require.Equal(t, "old instructions", restoredPrompt)
}

// TestInProcProjectService_GetProject_NotFound_ReturnsErrProjectNotFound
// exercises the 404 → ErrProjectNotFound mapping that WorkerProject.Ensure's
// stale-pointer recovery path depends on (Ensure → GetProject errors.Is(…,
// ErrProjectNotFound) → ClearProject + re-apply).
func TestInProcProjectService_GetProject_NotFound_ReturnsErrProjectNotFound(t *testing.T) {
	_, _, client, _, ctx := newInProcTestSetup(t)

	_, err := client.GetProject(ctx, "proj_missing")
	require.Error(t, err)
	require.True(t, errors.Is(err, runtimehelix.ErrProjectNotFound),
		"expected ErrProjectNotFound, got %v", err)
}

// TestInProcProjectService_GetProject_Found_ReturnsProject verifies the
// happy path — a seeded project row round-trips through the inProc
// adapter back to types.Project. Authorization passes because the
// adapter resolves the request user as the project owner.
func TestInProcProjectService_GetProject_Found_ReturnsProject(t *testing.T) {
	_, store, client, user, ctx := newInProcTestSetup(t)

	store.SeedProject(&types.Project{
		ID:     "proj_test_1",
		Name:   "test-project",
		UserID: user.ID,
		Status: "active",
	})

	got, err := client.GetProject(ctx, "proj_test_1")
	require.NoError(t, err)
	require.Equal(t, "proj_test_1", got.ID)
	require.Equal(t, "test-project", got.Name)
}

// TestInProcSpawnerClient_GetOutput_ReturnsStoreData seeds a session
// + interactions and asserts GetOutput returns the data the handler
// would have served via /api/v1/sessions/{id}/output.
func TestInProcSpawnerClient_GetOutput_ReturnsStoreData(t *testing.T) {
	_, store, client, user, ctx := newInProcTestSetup(t)

	session, err := store.CreateSession(context.Background(), types.Session{
		ID:    "ses_test_1",
		Owner: user.ID,
	})
	require.NoError(t, err)

	_, err = store.CreateInteraction(context.Background(), &types.Interaction{
		ID:              "int_test_1",
		SessionID:       session.ID,
		State:           types.InteractionStateComplete,
		ResponseMessage: "hello from the test",
	})
	require.NoError(t, err)

	out, err := client.GetOutput(ctx, "ses_test_1")
	require.NoError(t, err)
	require.Equal(t, "ses_test_1", out.SessionID)
	// Status comes from the last interaction's State (set above).
	require.Equal(t, string(types.InteractionStateComplete), out.Status)
}

// TestInProcProjectService_GetAppConfig_RoundTrips seeds an app row and
// asserts GetAppConfig returns its embedded AppConfig verbatim.
// Validates the JSON shape Helix's handler emits matches what the
// runtimehelix port expects (types.AppConfig directly, not raw JSON).
func TestInProcProjectService_GetAppConfig_RoundTrips(t *testing.T) {
	_, store, client, user, ctx := newInProcTestSetup(t)

	want := types.AppConfig{
		AllowedDomains: []string{"example.test"},
		Secrets:        map[string]string{"FOO": "bar"},
		Helix: types.AppHelixConfig{
			Name: "test-app",
		},
	}
	store.SeedApp(&types.App{
		ID:     "app_test_1",
		Owner:  user.ID,
		Config: want,
	})

	got, err := client.GetAppConfig(ctx, "app_test_1")
	require.NoError(t, err)
	require.Equal(t, want.AllowedDomains, got.AllowedDomains)
	require.Equal(t, want.Secrets, got.Secrets)
	require.Equal(t, want.Helix.Name, got.Helix.Name)
}

func TestInProcProjectService_GetAppConfig_PreservesNotFound(t *testing.T) {
	_, _, client, _, ctx := newInProcTestSetup(t)
	_, err := client.GetAppConfig(ctx, "app-missing")
	require.ErrorIs(t, err, helixstore.ErrNotFound)
}

// TestInProcSpawnerClient_StopExternalAgent_NoSession_ReturnsError
// confirms a missing session ID surfaces as an error (the underlying
// handler returns 404; the adapter wraps it as a generic error — no
// sentinel needed since SpawnerClient.StopExternalAgent has no
// sentinel-Is contract).
func TestInProcSpawnerClient_StopExternalAgent_NoSession_ReturnsError(t *testing.T) {
	_, _, client, _, ctx := newInProcTestSetup(t)

	err := client.StopExternalAgent(ctx, "ses_does_not_exist")
	require.Error(t, err)
}

// TestInProcSpawnerClient_ClearSession_RemovesInteractionsKeepsSession
// pins the wiring the spawner relies on before every re-activation:
// ClearSession routes through clearSessionHandler (so authz matches the
// SendMessage path), wipes the session's interactions, and preserves the
// session row itself. An internal (non-Zed) session has a no-op runtime
// backend, so this exercises the shared DB clear in isolation.
func TestInProcSpawnerClient_ClearSession_RemovesInteractionsKeepsSession(t *testing.T) {
	_, store, client, user, ctx := newInProcTestSetup(t)

	session, err := store.CreateSession(context.Background(), types.Session{
		ID:    "ses_clear_1",
		Owner: user.ID,
	})
	require.NoError(t, err)
	_, err = store.CreateInteraction(context.Background(), &types.Interaction{
		ID:              "int_clear_1",
		SessionID:       session.ID,
		State:           types.InteractionStateComplete,
		ResponseMessage: "prior context",
	})
	require.NoError(t, err)

	require.NoError(t, client.ClearSession(ctx, "ses_clear_1"))

	// Session row preserved.
	got, err := store.GetSession(context.Background(), "ses_clear_1")
	require.NoError(t, err)
	require.NotNil(t, got)
	// Interactions gone — the next turn starts on an empty context.
	ints, _, err := store.ListInteractions(context.Background(), &types.ListInteractionsQuery{
		SessionID:    "ses_clear_1",
		GenerationID: -1,
	})
	require.NoError(t, err)
	require.Empty(t, ints, "ClearSession must remove the session's interactions")
}

// TestInProcSpawnerClient_ClearSession_NoSession_ReturnsError confirms a
// missing session surfaces as an error rather than silently succeeding,
// so a stale persisted session pointer fails the activation loudly.
func TestInProcSpawnerClient_ClearSession_NoSession_ReturnsError(t *testing.T) {
	_, _, client, _, ctx := newInProcTestSetup(t)

	err := client.ClearSession(ctx, "ses_does_not_exist")
	require.Error(t, err)
}

func TestInProcSpawnerClient_SyncAgentProfileRenamesStoppedSession(t *testing.T) {
	_, store, client, _, ctx := newInProcTestSetup(t)
	_, err := store.CreateSession(ctx, types.Session{ID: "ses_profile", Name: "You are Bot old"})
	require.NoError(t, err)

	err = client.SyncAgentProfile(ctx, "ses_profile", "Build Engineer", "w-build", "instructions")
	require.ErrorContains(t, err, "external agent executor is not configured")

	got, err := store.GetSession(ctx, "ses_profile")
	require.NoError(t, err)
	require.Equal(t, "Build Engineer", got.Name)
	require.Equal(t, "w-build", got.Metadata.OrgWorkerID)
	require.Equal(t, "instructions", got.Metadata.RuntimeInstructions)
}

// TestParseEnvVarsToMap pins the KEY=value split that backs
// ListProjectSecrets / list_secrets. A value containing `=` (base64,
// tokens, URL query strings) must survive intact — Cut on the FIRST `=`
// — and a malformed entry must never produce a "" key.
func TestParseEnvVarsToMap(t *testing.T) {
	got := parseEnvVarsToMap([]string{
		"DRONE_TOKEN=abc",
		"B64=a=b=c==",           // value with `=` — keep everything after the first
		"URL=https://x?a=1&b=2", // query string with `=`
		"EMPTY=",                // empty value is a valid secret
		"=orphan",               // empty name — dropped
		"NOEQUALS",              // no `=` — dropped
	})
	require.Equal(t, map[string]string{
		"DRONE_TOKEN": "abc",
		"B64":         "a=b=c==",
		"URL":         "https://x?a=1&b=2",
		"EMPTY":       "",
	}, got)
}

// TODO: tests for StartSession / SendMessage. StartSession routes to
// StartExternalAgentSession (starts a real dev container) and
// SendMessage to sendSessionMessage (needs a connected external-agent
// WS), both non-trivial to satisfy from memorystore in isolation. These
// adapters are the same shared primitives the cron trigger and spec
// tasks use, and are exercised end-to-end by the helix-org alpha sandbox
// flow in the inner Helix; focused unit tests belong in a follow-up that
// stubs the executor + WS manager.
