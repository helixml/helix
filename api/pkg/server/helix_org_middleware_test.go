package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type bootstrapHelixStore struct {
	helixstore.Store
}

func (s *bootstrapHelixStore) GetOrganization(_ context.Context, _ *helixstore.GetOrganizationQuery) (*types.Organization, error) {
	return &types.Organization{ID: "org-race", Owner: "user-owner"}, nil
}

func (s *bootstrapHelixStore) GetUser(_ context.Context, _ *helixstore.GetUserQuery) (*types.User, error) {
	return &types.User{ID: "user-owner", AlphaFeatures: []string{"helix-org"}}, nil
}

func (s *bootstrapHelixStore) CreateAPIKey(_ context.Context, key *types.ApiKey) (*types.ApiKey, error) {
	return key, nil
}

type failingBootstrapHelixStore struct {
	helixstore.Store
}

func (s *failingBootstrapHelixStore) GetOrganization(context.Context, *helixstore.GetOrganizationQuery) (*types.Organization, error) {
	return nil, errors.New("organization unavailable")
}

type helixOrgRouteTestStore struct {
	helixstore.Store
}

type bootstrapActivationDispatcher struct {
	ids           []orgchart.BotID
	activationIDs []activation.ID
	defaults      *repairDefaultApplier
	outOfOrder    bool
}

func (d *bootstrapActivationDispatcher) DispatchHire(_ context.Context, _ string, id orgchart.BotID, activationID activation.ID) {
	if d.defaults != nil && !d.defaults.applied {
		d.outOfOrder = true
	}
	d.ids = append(d.ids, id)
	d.activationIDs = append(d.activationIDs, activationID)
}

type repairDefaultApplier struct {
	applied bool
	appID   string
	config  types.AssistantConfig
}

func (a *repairDefaultApplier) ApplyAgentDefaults(_ context.Context, appID string, defaults types.AssistantConfig) error {
	a.applied = true
	a.appID = appID
	a.config = defaults
	return nil
}

func (s *helixOrgRouteTestStore) GetOrganization(_ context.Context, query *helixstore.GetOrganizationQuery) (*types.Organization, error) {
	if query.Name != "acme" {
		return nil, errors.New("unexpected organization")
	}
	return &types.Organization{ID: "org_acme", Name: "acme"}, nil
}

func (s *helixOrgRouteTestStore) GetOrganizationMembership(_ context.Context, query *helixstore.GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
	return &types.OrganizationMembership{
		OrganizationID: query.OrganizationID,
		UserID:         query.UserID,
		Role:           types.OrganizationRoleMember,
	}, nil
}

func newHelixOrgRouteTestHandler(t *testing.T) (http.Handler, *helixOrgScope) {
	t.Helper()
	scope := &helixOrgScope{bootstrapped: map[string]bool{}}
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if orgID := helixorgserver.OrgIDFromContext(r.Context()); orgID != "org_acme" {
			t.Errorf("org ID context = %q, want org_acme", orgID)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	server := &HelixAPIServer{Store: &helixOrgRouteTestStore{}}
	root := mux.NewRouter()
	router := root.PathPrefix(APIPrefix).Subrouter()
	server.registerHelixOrgAuthenticatedRoutes(router, &helixOrgHandlers{api: api, scope: scope})
	return root, scope
}

func helixOrgRouteRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user-1"}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHelixOrgSettingsRoutesDoNotRequireFeature(t *testing.T) {
	handler, _ := newHelixOrgRouteTestHandler(t)
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/orgs/acme/settings"},
		{http.MethodPut, "/api/v1/orgs/acme/settings/agent.default"},
		{http.MethodDelete, "/api/v1/orgs/acme/settings/agent.default"},
		{http.MethodGet, "/api/v1/orgs/acme/github/app-installation"},
		{http.MethodPost, "/api/v1/orgs/acme/github/app-manifest"},
	}
	for _, route := range routes {
		rec := helixOrgRouteRequest(handler, route.method, route.path)
		if rec.Code != http.StatusNoContent {
			t.Errorf("%s %s status = %d, want %d", route.method, route.path, rec.Code, http.StatusNoContent)
		}
	}
}

func TestHelixOrgChartRouteStillRequiresFeature(t *testing.T) {
	handler, _ := newHelixOrgRouteTestHandler(t)
	rec := helixOrgRouteRequest(handler, http.MethodGet, "/api/v1/orgs/acme/chart/positions")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHelixOrgSettingsRoutesDoNotBootstrap(t *testing.T) {
	handler, scope := newHelixOrgRouteTestHandler(t)
	rec := helixOrgRouteRequest(handler, http.MethodGet, "/api/v1/orgs/acme/settings")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if scope.bootstrapped["org_acme"] {
		t.Fatal("settings request bootstrapped org graph")
	}
}

// TestEnsureBootstrapConcurrentCallsAllSucceed pins the regression
// behind the 500 on first load of /orgs/<org>/helix-org/chart: the
// HelixOrgChart page renders and fires several React Query hooks in
// parallel (/chart, /workers, /roles, /streams, …) — every one of
// those endpoints lives under withHelixOrgScope and so every one
// calls ensureBootstrap with the same orgID concurrently.
//
// Before the fix, the per-org mutex only guarded the
// bootstrapped[orgID] map flag. Two requests could both read false,
// both enter bootstrap.Run, and only one would create the owner Role
// — the loser failed with "create owner role: %w" wrapping a
// duplicate-key error, returning 500 to the browser. Refreshing
// worked because by then bootstrapped[orgID] was true and the second
// request short-circuited.
//
// This test fires N goroutines through a single helixOrgScope and
// asserts that every one returns nil. Once the mutex covers the
// entire bootstrap.Run call (via singleflight or a per-org lock
// held across the work), losers will block until the winner finishes
// and then short-circuit on the true flag — no duplicate-key error
// will surface.
func TestEnsureBootstrapConcurrentCallsAllSucceed(t *testing.T) {
	t.Parallel()
	orgStore := orgmemory.New()
	configs := configregistry.New(orgStore.Configs)
	configs.Register(configregistry.Spec{Key: "helix.api_key", Type: configregistry.TypeString})
	scope := newHelixOrgScope(
		configs,
		orgStore,
		&bootstrapHelixStore{},
		nil, // mirror — nil is a safe no-op for this bootstrap-race test
		nil, // slackRoutes — nil is a safe no-op
		nil, // helixEvents — nil is a safe no-op
	)

	const N = 8
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	wg.Add(N)
	start := make(chan struct{})
	for range N {
		go func() {
			defer wg.Done()
			<-start
			if err := scope.ensureBootstrap(context.Background(), "org-race"); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("concurrent ensureBootstrap returned %d errors; first: %v", len(errs), errs[0])
	}
}

func TestEnsureBootstrapProvisionsServiceKeyBeforeGraphSeed(t *testing.T) {
	t.Parallel()
	orgStore := orgmemory.New()
	configs := configregistry.New(orgStore.Configs)
	configs.Register(configregistry.Spec{Key: "helix.api_key", Type: configregistry.TypeString})
	scope := newHelixOrgScope(configs, orgStore, &bootstrapHelixStore{}, nil, nil, nil)
	seeded := false
	scope.humanReconcile = func(ctx context.Context, orgID string) error {
		key, err := configs.GetString(ctx, orgID, "helix.api_key")
		if err != nil {
			return err
		}
		if key == "" {
			return errors.New("graph seeded before service key")
		}
		seeded = true
		return nil
	}

	if err := scope.ensureBootstrap(context.Background(), "org-race"); err != nil {
		t.Fatalf("ensure bootstrap: %v", err)
	}
	if !seeded {
		t.Fatal("graph seed hook was not called")
	}
}

func TestEnsureBootstrapRetriesAfterServiceKeyFailure(t *testing.T) {
	t.Parallel()
	orgStore := orgmemory.New()
	scope := newHelixOrgScope(configregistry.New(orgStore.Configs), orgStore, &failingBootstrapHelixStore{}, nil, nil, nil)
	for range 2 {
		if err := scope.ensureBootstrap(context.Background(), "org-retry"); err == nil {
			t.Fatal("ensureBootstrap should fail while the service key cannot be provisioned")
		}
	}
	if scope.bootstrapped["org-retry"] {
		t.Fatal("failed bootstrap must remain retryable")
	}
}

func TestRepairNeverActivatedBotsSkipsHumansAndActivatedBots(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orgmemory.New()
	configs := configregistry.New(st.Configs)
	configs.Register(configregistry.Spec{Key: configregistry.DefaultAgentConfigKey, Type: configregistry.TypeObject})
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	for _, b := range []orgchart.Bot{
		mustBot(t, "bot-legacy-a", "", now).WithAgentAppID("app-legacy-a"),
		mustBot(t, "bot-legacy-b", "", now),
		mustBot(t, "bot-created", "", now),
		mustBot(t, "human-owner", orgchart.BotKindHuman, now),
	} {
		if err := st.Bots.Create(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	createdActivation, err := activation.New("a-created", "bot-created", []activation.Trigger{{Kind: activation.TriggerHire}}, now, "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Activations.Create(ctx, createdActivation); err != nil {
		t.Fatal(err)
	}

	dispatcher := &bootstrapActivationDispatcher{}
	if err := repairNeverActivatedBots(ctx, "org-test", st, dispatcher, configs, nil); err != nil {
		t.Fatalf("repair without config: %v", err)
	}
	if len(dispatcher.ids) != 0 {
		t.Fatalf("repair before config dispatched bots = %v", dispatcher.ids)
	}
	if err := configs.Set(ctx, "org-test", configregistry.DefaultAgentConfigKey,
		`{"code_agent_runtime":"zed_agent","code_agent_credential_type":"api_key"}`); err != nil {
		t.Fatalf("set incomplete default agent config: %v", err)
	}
	if err := repairNeverActivatedBots(ctx, "org-test", st, dispatcher, configs, nil); err != nil {
		t.Fatalf("repair with incomplete config: %v", err)
	}
	if len(dispatcher.ids) != 0 {
		t.Fatalf("repair before complete config dispatched bots = %v", dispatcher.ids)
	}
	if err := configs.Set(ctx, "org-test", configregistry.DefaultAgentConfigKey,
		`{"code_agent_runtime":"zed_agent","code_agent_credential_type":"api_key","provider":"anthropic","model":"claude-opus-4-6"}`); err != nil {
		t.Fatalf("set default agent config: %v", err)
	}
	applier := &repairDefaultApplier{}
	dispatcher.defaults = applier
	if err := repairNeverActivatedBots(ctx, "org-test", st, dispatcher, configs, applier); err != nil {
		t.Fatalf("repair never-activated bots: %v", err)
	}
	if !applier.applied || applier.appID != "app-legacy-a" ||
		applier.config.Provider != "anthropic" || applier.config.Model != "claude-opus-4-6" {
		t.Fatalf("applied defaults = %+v", applier)
	}
	if dispatcher.outOfOrder {
		t.Fatal("repair dispatched before applying Agent defaults")
	}
	if len(dispatcher.ids) != 2 || dispatcher.ids[0] != "bot-legacy-a" || dispatcher.ids[1] != "bot-legacy-b" {
		t.Fatalf("activated bots = %v, want [bot-legacy-a bot-legacy-b]", dispatcher.ids)
	}
	if len(dispatcher.activationIDs) != 2 || dispatcher.activationIDs[0] == "" || dispatcher.activationIDs[1] == "" {
		t.Fatalf("repair activation IDs = %v, want two durable IDs", dispatcher.activationIDs)
	}
	for i, id := range dispatcher.ids {
		rows, err := st.Activations.ListForWorker(ctx, "org-test", id, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].ID != dispatcher.activationIDs[i] {
			t.Fatalf("activation for %s = %v, want %s", id, rows, dispatcher.activationIDs[i])
		}
	}

	secondDispatcher := &bootstrapActivationDispatcher{}
	if err := repairNeverActivatedBots(ctx, "org-test", st, secondDispatcher, configs, applier); err != nil {
		t.Fatalf("repeat repair: %v", err)
	}
	if len(secondDispatcher.ids) != 0 {
		t.Fatalf("repeat repair dispatched bots = %v, want none", secondDispatcher.ids)
	}
}

func mustBot(t *testing.T, id string, kind orgchart.BotKind, now time.Time) orgchart.Bot {
	t.Helper()
	b, err := orgchart.NewBot(orgchart.BotID(id), "test bot", nil, now, "org-test")
	if err != nil {
		t.Fatal(err)
	}
	return b.WithKind(kind)
}
