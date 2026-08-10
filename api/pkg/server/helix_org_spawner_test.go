package server

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	helixorgconfig "github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/server/helixorg"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// TestBuildHelixOrgSpawnerConfig_WiresProjectService is the regression
// test for the AI-worker click crash.
//
// Repro before the fix: hire an AI worker into a position, click the
// worker chip in the chart. The lazy spawner cached a SpawnerConfig
// whose ProjectService field was never populated (the builder
// constructed the struct field-by-field but forgot it), and the
// inner Spawner closure's ensureProject fast-path nil-derefed at
// project.go:156 the moment it tried to verify the per-Worker Helix
// project still existed.
//
// This test pins the builder down: pass it a non-nil
// runtimehelix.ProjectService (the same inProcHelixClient production
// uses) and the returned cfg MUST contain it. If a future refactor
// drops the assignment, this fires.
func TestBuildHelixOrgSpawnerConfig_WiresProjectService(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	orgStore := orggorm.GetOrgTestDB(t)
	reg := helixorgconfig.New(orgStore.Configs)
	helixorg.RegisterConfigSpecs(reg)

	const orgID = "org-test"
	require.NoError(t, reg.Set(ctx, orgID, "helix.api_key", `"hlx-test-key"`))
	require.NoError(t, reg.Set(ctx, orgID, "helix.url", `"http://helix.test"`))
	store := helixstore.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetOrganization(gomock.Any(), &helixstore.GetOrganizationQuery{ID: orgID}).
		Return(&types.Organization{ID: orgID, Owner: "usr-owner"}, nil).Times(2)
	store.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: "hlx-test-key"}).
		Return(&types.ApiKey{Key: "hlx-test-key", Owner: "usr-owner"}, nil)

	_, _, projectSvc, _, _ := newInProcTestSetup(t)
	hub := wakebus.New(pubsub.NewNoop())
	logger := slog.Default()

	cfg, err := buildHelixOrgSpawnerConfig(ctx, orgID, spawnerDeps{
		Cfg:        reg,
		HelixStore: store,
		ProjectSvc: projectSvc, // the field we're pinning
		OrgStore:   orgStore,
		Hub:        hub,
		PubSub:     pubsub.NewNoop(), // required: spawner.bridge.run calls SubscribeSessionUpdates
		Logger:     logger,
		NewID:      func() string { return "id" },
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
	})
	require.NoError(t, err)
	require.NotNil(t, cfg.ProjectService, "ProjectService must be wired — its absence used to nil-deref WorkerProject.Ensure at project.go:156")
	// Same pointer round-tripped — confirms the builder copies the
	// host-provided service, not some other one constructed inside.
	require.Same(t, projectSvc, cfg.ProjectService.(*inProcHelixClient))
}

func TestBuildHelixOrgSpawnerConfig_ReplacesStaleServiceKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const orgID = "org-test"
	const ownerID = "usr-owner"

	orgStore := orggorm.GetOrgTestDB(t)
	reg := helixorgconfig.New(orgStore.Configs)
	helixorg.RegisterConfigSpecs(reg)
	require.NoError(t, reg.Set(ctx, orgID, "helix.api_key", `"hlx-stale"`))
	require.NoError(t, reg.Set(ctx, orgID, "helix.url", `"http://helix.test"`))

	store := helixstore.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetOrganization(gomock.Any(), &helixstore.GetOrganizationQuery{ID: orgID}).
		Return(&types.Organization{ID: orgID, Owner: ownerID}, nil).Times(2)
	store.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: "hlx-stale"}).
		Return(nil, helixstore.ErrNotFound)
	store.EXPECT().GetUser(gomock.Any(), &helixstore.GetUserQuery{ID: ownerID}).
		Return(&types.User{ID: ownerID}, nil)
	store.EXPECT().CreateAPIKey(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, key *types.ApiKey) (*types.ApiKey, error) { return key, nil })

	_, _, projectSvc, _, _ := newInProcTestSetup(t)
	cfg, err := buildHelixOrgSpawnerConfig(ctx, orgID, spawnerDeps{
		Cfg:        reg,
		HelixStore: store,
		ProjectSvc: projectSvc,
		OrgStore:   orgStore,
		Hub:        wakebus.New(pubsub.NewNoop()),
		PubSub:     pubsub.NewNoop(),
		Logger:     slog.Default(),
		NewID:      func() string { return "id" },
		Now:        func() time.Time { return time.Unix(0, 0).UTC() },
	})
	require.NoError(t, err)
	require.NotEmpty(t, cfg.MCPAuthBearer)
	require.NotEqual(t, "hlx-stale", cfg.MCPAuthBearer)
	stored, err := reg.GetString(ctx, orgID, "helix.api_key")
	require.NoError(t, err)
	require.Equal(t, cfg.MCPAuthBearer, stored)
}

func TestBuildHelixOrgProjectApplier_ReplacesStaleServiceKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const orgID = "org-test"
	const ownerID = "usr-owner"

	orgStore := orggorm.GetOrgTestDB(t)
	reg := helixorgconfig.New(orgStore.Configs)
	helixorg.RegisterConfigSpecs(reg)
	require.NoError(t, reg.Set(ctx, orgID, "helix.api_key", `"hlx-stale"`))
	require.NoError(t, reg.Set(ctx, orgID, "helix.url", `"http://helix.test"`))

	store := helixstore.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetOrganization(gomock.Any(), &helixstore.GetOrganizationQuery{ID: orgID}).
		Return(&types.Organization{ID: orgID, Owner: ownerID}, nil)
	store.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: "hlx-stale"}).
		Return(nil, helixstore.ErrNotFound)
	store.EXPECT().GetUser(gomock.Any(), &helixstore.GetUserQuery{ID: ownerID}).
		Return(&types.User{ID: ownerID}, nil)
	store.EXPECT().CreateAPIKey(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, key *types.ApiKey) (*types.ApiKey, error) { return key, nil })

	_, bearer, err := buildHelixOrgProjectApplier(ctx, orgID, reg, store, nil, orgStore, slog.Default())
	require.NoError(t, err)
	require.NotEmpty(t, bearer)
	require.NotEqual(t, "hlx-stale", bearer)
}

type mcpRepairProjectService struct {
	runtimehelix.ProjectService
	config      types.AppConfig
	badApp      string
	badAppError error
	updated     types.AppConfig
	updatedIDs  []string
}

func (s *mcpRepairProjectService) GetAppConfig(_ context.Context, appID string) (types.AppConfig, error) {
	if appID == s.badApp {
		return types.AppConfig{}, s.badAppError
	}
	return s.config, nil
}

func (s *mcpRepairProjectService) UpdateAppConfig(_ context.Context, appID string, config types.AppConfig) error {
	s.updated = config
	s.updatedIDs = append(s.updatedIDs, appID)
	return nil
}

func TestRepairHelixOrgMCPServiceKeys_UpdatesLinkedApps(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const orgID = "org-test"

	orgStore := orggorm.GetOrgTestDB(t)
	worker, err := orgchart.NewNode("b-worker", "worker", nil, time.Now().UTC(), orgID)
	require.NoError(t, err)
	worker = worker.WithAgentID("app-worker")
	require.NoError(t, orgStore.Nodes.Create(ctx, worker))

	reg := helixorgconfig.New(orgStore.Configs)
	helixorg.RegisterConfigSpecs(reg)
	require.NoError(t, reg.Set(ctx, orgID, "helix.url", `"http://helix.test"`))
	svc := &mcpRepairProjectService{config: types.AppConfig{Helix: types.AppHelixConfig{
		Assistants: []types.AssistantConfig{{MCPs: []types.AssistantMCP{{
			Name: "helix", Headers: map[string]string{"Authorization": "Bearer hlx-stale"},
		}}}},
	}}}

	require.NoError(t, repairHelixOrgMCPServiceKeys(ctx, orgID, "hlx-fresh", reg, orgStore, svc))
	require.Equal(t, "Bearer hlx-fresh", svc.updated.Helix.Assistants[0].MCPs[0].Headers["Authorization"])
}

func TestRepairHelixOrgMCPServiceKeys_ContinuesPastMissingApp(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const orgID = "org-test"

	orgStore := orggorm.GetOrgTestDB(t)
	for _, worker := range []struct{ id, appID string }{{"b-bad", "app-bad"}, {"b-good", "app-good"}} {
		node, err := orgchart.NewNode(worker.id, "worker", nil, time.Now().UTC(), orgID)
		require.NoError(t, err)
		require.NoError(t, orgStore.Nodes.Create(ctx, node.WithAgentID(worker.appID)))
	}
	reg := helixorgconfig.New(orgStore.Configs)
	helixorg.RegisterConfigSpecs(reg)
	require.NoError(t, reg.Set(ctx, orgID, "helix.url", `"http://helix.test"`))
	svc := &mcpRepairProjectService{
		badApp:      "app-bad",
		badAppError: helixstore.ErrNotFound,
		config:      types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{}}}},
	}

	require.NoError(t, repairHelixOrgMCPServiceKeys(ctx, orgID, "hlx-fresh", reg, orgStore, svc))
	require.Equal(t, []string{"app-good"}, svc.updatedIDs)
}

func TestRepairHelixOrgMCPServiceKeys_RetriesAfterTransientAppFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const orgID = "org-test"

	orgStore := orggorm.GetOrgTestDB(t)
	for _, worker := range []struct{ id, appID string }{{"b-bad", "app-bad"}, {"b-good", "app-good"}} {
		node, err := orgchart.NewNode(worker.id, "worker", nil, time.Now().UTC(), orgID)
		require.NoError(t, err)
		require.NoError(t, orgStore.Nodes.Create(ctx, node.WithAgentID(worker.appID)))
	}
	reg := helixorgconfig.New(orgStore.Configs)
	helixorg.RegisterConfigSpecs(reg)
	require.NoError(t, reg.Set(ctx, orgID, "helix.url", `"http://helix.test"`))
	svc := &mcpRepairProjectService{
		badApp:      "app-bad",
		badAppError: errors.New("database unavailable"),
		config:      types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{}}}},
	}

	err := repairHelixOrgMCPServiceKeys(ctx, orgID, "hlx-fresh", reg, orgStore, svc)
	require.ErrorContains(t, err, "database unavailable")
	require.Equal(t, []string{"app-good"}, svc.updatedIDs)
}

// TestBuildHelixOrgSpawnerConfig_RejectsNilProjectService pins the
// second half of the defence: passing nil should produce a clear
// error from the builder, not silently produce a config that will
// crash later. Catches "I forgot to update the caller" mistakes at
// the boundary instead of at activation time.
func TestBuildHelixOrgSpawnerConfig_RejectsNilProjectService(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	orgStore := orggorm.GetOrgTestDB(t)
	reg := helixorgconfig.New(orgStore.Configs)
	helixorg.RegisterConfigSpecs(reg)

	const orgID = "org-test"
	require.NoError(t, reg.Set(ctx, orgID, "helix.api_key", `"hlx-test-key"`))
	require.NoError(t, reg.Set(ctx, orgID, "helix.url", `"http://helix.test"`))

	_, err := buildHelixOrgSpawnerConfig(ctx, orgID, spawnerDeps{
		Cfg: reg,
		// ProjectSvc explicitly nil — the case under test.
		OrgStore: orgStore,
		Hub:      wakebus.New(pubsub.NewNoop()),
		PubSub:   pubsub.NewNoop(),
		Logger:   slog.Default(),
		NewID:    func() string { return "id" },
		Now:      func() time.Time { return time.Unix(0, 0).UTC() },
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "ProjectService is required")
}

// TestWorkerProjectEnsure_NilService_ReturnsError pins the defensive
// guard inside WorkerProject.Ensure itself. Even if a host wires a
// SpawnerConfig with a nil ProjectService (e.g. by skipping the
// builder), the activation path should surface an error instead of
// crashing the API.
func TestWorkerProjectEnsure_NilService_ReturnsError(t *testing.T) {
	t.Parallel()
	a := &runtimehelix.WorkerProject{
		// Service intentionally left nil.
	}
	_, _, _, err := a.Ensure(context.Background(), "org", "w-x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Service")
}
