package server

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	helixorgconfig "github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/streamhub"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/pubsub"
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
	registerHelixOrgConfigSpecs(reg)

	const orgID = "org-test"
	require.NoError(t, reg.Set(ctx, orgID, "helix.api_key", `"hlx-test-key"`, orgchart.WorkerID("")))
	require.NoError(t, reg.Set(ctx, orgID, "helix.url", `"http://helix.test"`, orgchart.WorkerID("")))

	_, _, projectSvc, _ := newInProcTestSetup(t)
	hub := streamhub.New(pubsub.NewNoop())
	logger := slog.Default()

	cfg, err := buildHelixOrgSpawnerConfig(
		ctx, orgID, reg, nil,
		nil,        // spawnerClient — not exercised here
		projectSvc, // projectSvc — the field we're pinning
		orgStore,
		hub,
		pubsub.NewNoop(), // PubSub — required since spawner.bridge.run calls SubscribeSessionUpdates
		logger,
		nil, // gitHubTokenResolver — not exercised here
		func() string { return "id" },
		func() time.Time { return time.Unix(0, 0).UTC() },
	)
	require.NoError(t, err)
	require.NotNil(t, cfg.ProjectService, "ProjectService must be wired — its absence used to nil-deref WorkerProject.Ensure at project.go:156")
	// Same pointer round-tripped — confirms the builder copies the
	// host-provided service, not some other one constructed inside.
	require.Same(t, projectSvc, cfg.ProjectService.(*inProcHelixClient))
}

// TestBuildHelixOrgSpawnerConfig_WiresGitHubTokenResolver pins the
// GitHub-OAuth → spawner wiring. The host (helix_org.go) constructs
// a resolver via newGitHubOAuthResolver and passes it through to the
// spawner config. The spawner's per-activation injectGitHubToken
// path then calls it to refresh `GH_TOKEN` on every activation.
//
// Without this assertion the host could silently drop the resolver
// at the boundary — workers would land in the desktop with `gh`
// unauthenticated and no clear error pointing at the wiring gap.
func TestBuildHelixOrgSpawnerConfig_WiresGitHubTokenResolver(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	orgStore := orggorm.GetOrgTestDB(t)
	reg := helixorgconfig.New(orgStore.Configs)
	registerHelixOrgConfigSpecs(reg)

	const orgID = "org-test"
	require.NoError(t, reg.Set(ctx, orgID, "helix.api_key", `"hlx-test-key"`, orgchart.WorkerID("")))
	require.NoError(t, reg.Set(ctx, orgID, "helix.url", `"http://helix.test"`, orgchart.WorkerID("")))

	_, _, projectSvc, _ := newInProcTestSetup(t)
	hub := streamhub.New(pubsub.NewNoop())

	var called int
	resolver := func(_ context.Context, gotOrg string) (string, error) {
		called++
		require.Equal(t, orgID, gotOrg)
		return "gho_test", nil
	}

	cfg, err := buildHelixOrgSpawnerConfig(
		ctx, orgID, reg, nil,
		nil,
		projectSvc,
		orgStore,
		hub,
		pubsub.NewNoop(),
		slog.Default(),
		resolver,
		func() string { return "id" },
		func() time.Time { return time.Unix(0, 0).UTC() },
	)
	require.NoError(t, err)
	require.NotNil(t, cfg.GitHubTokenResolver, "GitHubTokenResolver must be wired — without it the worker desktop has no GH_TOKEN")
	// Round-trip the resolver to confirm it's the host-provided one, not
	// some intermediate wrapper that drops the value.
	tok, rerr := cfg.GitHubTokenResolver(ctx, orgID)
	require.NoError(t, rerr)
	require.Equal(t, "gho_test", tok)
	require.Equal(t, 1, called)
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
	registerHelixOrgConfigSpecs(reg)

	const orgID = "org-test"
	require.NoError(t, reg.Set(ctx, orgID, "helix.api_key", `"hlx-test-key"`, orgchart.WorkerID("")))
	require.NoError(t, reg.Set(ctx, orgID, "helix.url", `"http://helix.test"`, orgchart.WorkerID("")))

	_, err := buildHelixOrgSpawnerConfig(
		ctx, orgID, reg, nil,
		nil, // spawnerClient
		nil, // projectSvc — explicitly nil
		orgStore, streamhub.New(pubsub.NewNoop()),
		pubsub.NewNoop(),
		slog.Default(),
		nil, // gitHubTokenResolver
		func() string { return "id" },
		func() time.Time { return time.Unix(0, 0).UTC() },
	)
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
