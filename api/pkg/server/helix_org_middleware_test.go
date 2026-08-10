package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/org/application/assets"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/asset"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
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
	return &types.User{ID: "user-owner"}, nil
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
	role          types.OrganizationRole
	membershipErr error
	session       *types.Session
}

func (s *helixOrgRouteTestStore) GetSession(_ context.Context, id string) (*types.Session, error) {
	if s.session == nil || s.session.ID != id {
		return nil, helixstore.ErrNotFound
	}
	return s.session, nil
}

type bootstrapActivationDispatcher struct {
	ids           []orgchart.NodeID
	activationIDs []activation.ID
	defaults      *repairDefaultApplier
	outOfOrder    bool
}

func (d *bootstrapActivationDispatcher) DispatchHire(_ context.Context, _ string, id orgchart.NodeID, activationID activation.ID) {
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
	if s.membershipErr != nil {
		return nil, s.membershipErr
	}
	role := s.role
	if role == "" {
		role = types.OrganizationRoleMember
	}
	return &types.OrganizationMembership{
		OrganizationID: query.OrganizationID,
		UserID:         query.UserID,
		Role:           role,
	}, nil
}

func newAssetRBACIntegrationHandler(t *testing.T, routeStore *helixOrgRouteTestStore) http.Handler {
	t.Helper()
	st := orgmemory.New()
	service, err := assets.New(assets.Deps{
		Assets: st.Assets, Links: st.AssetLinks, Nodes: st.Nodes,
		GenerateKey: func() (string, string, error) { return "private-key", "ssh-ed25519 public-key", nil },
		Encrypt:     func(value []byte) (string, error) { return "encrypted:" + string(value), nil },
		Now:         func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC) },
		NewID:       func() string { return "asset-test" },
	})
	if err != nil {
		t.Fatalf("new assets service: %v", err)
	}
	downstream := orgapi.Handler(orgapi.Deps{Assets: service})
	server := &HelixAPIServer{Store: routeStore}
	return server.withHelixOrgIdentity(stripOrgScopedPrefix(downstream))
}

func assetRBACRequest(t *testing.T, handler http.Handler, user *types.User, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &payload)
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"org": "acme"})
	if user != nil {
		req = req.WithContext(setRequestUser(req.Context(), *user))
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
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

func TestHelixOrgChartRouteDoesNotRequireFeature(t *testing.T) {
	handler, scope := newHelixOrgRouteTestHandler(t)
	scope.bootstrapped["org_acme"] = true
	rec := helixOrgRouteRequest(handler, http.MethodGet, "/api/v1/orgs/acme/chart/positions")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestHelixOrgMCPBackendDoesNotRequireFeature(t *testing.T) {
	scope := &helixOrgScope{bootstrapped: map[string]bool{"org_acme": true}}
	downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if orgID := helixorgserver.OrgIDFromContext(r.Context()); orgID != "org_acme" {
			t.Errorf("org ID context = %q, want org_acme", orgID)
		}
		if r.URL.Path != "/orgs/org_acme/workers/b-worker/mcp" {
			t.Errorf("path = %q, want worker-bound MCP path", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	server := &HelixAPIServer{Store: &helixOrgRouteTestStore{session: &types.Session{
		ID: "ses-worker", Owner: "user-1", OrganizationID: "org_acme",
		Metadata: types.SessionMetadata{OrgWorkerID: "b-worker"},
	}}}
	backend := NewHelixOrgMCPBackend(server, &helixOrgHandlers{api: downstream, scope: scope})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/helix-org", nil)
	req = mux.SetURLVars(req, map[string]string{"path": ""})
	rec := httptest.NewRecorder()

	backend.ServeHTTP(rec, req, &types.User{ID: "user-1", TokenType: types.TokenTypeAPIKey, SessionID: "ses-worker", OrganizationID: "org_acme"})

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestHelixOrgMCPBackendRejectsInvalidSessionScope(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		user       *types.User
		session    *types.Session
		wantStatus int
	}{
		{name: "rejects suffix", path: "acme/workers/b-worker/mcp", wantStatus: http.StatusBadRequest},
		{name: "requires session key", user: &types.User{ID: "user-1"}, wantStatus: http.StatusForbidden},
		{name: "key org mismatch", user: &types.User{ID: "user-1", TokenType: types.TokenTypeAPIKey, SessionID: "ses-worker", OrganizationID: "org_other"}, session: &types.Session{ID: "ses-worker", Owner: "user-1", OrganizationID: "org_acme", Metadata: types.SessionMetadata{OrgWorkerID: "b-worker"}}, wantStatus: http.StatusForbidden},
		{name: "session owner mismatch", user: &types.User{ID: "user-1", TokenType: types.TokenTypeAPIKey, SessionID: "ses-worker", OrganizationID: "org_acme"}, session: &types.Session{ID: "ses-worker", Owner: "user-other", OrganizationID: "org_acme", Metadata: types.SessionMetadata{OrgWorkerID: "b-worker"}}, wantStatus: http.StatusForbidden},
		{name: "requires session org", user: &types.User{ID: "user-1", TokenType: types.TokenTypeAPIKey, SessionID: "ses-worker"}, session: &types.Session{ID: "ses-worker", Owner: "user-1", Metadata: types.SessionMetadata{OrgWorkerID: "b-worker"}}, wantStatus: http.StatusForbidden},
		{name: "requires session worker", user: &types.User{ID: "user-1", TokenType: types.TokenTypeAPIKey, SessionID: "ses-worker", OrganizationID: "org_acme"}, session: &types.Session{ID: "ses-worker", Owner: "user-1", OrganizationID: "org_acme"}, wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			scope := &helixOrgScope{bootstrapped: map[string]bool{"org_acme": true}}
			server := &HelixAPIServer{Store: &helixOrgRouteTestStore{session: tt.session}}
			backend := NewHelixOrgMCPBackend(server, &helixOrgHandlers{api: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }), scope: scope})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/helix-org", nil)
			req = mux.SetURLVars(req, map[string]string{"path": tt.path})
			rec := httptest.NewRecorder()

			backend.ServeHTTP(rec, req, tt.user)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if called {
				t.Fatal("mismatched session reached helix-org handler")
			}
		})
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

func TestHelixOrgAssetRBAC(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		role       types.OrganizationRole
		admin      bool
		wantStatus int
	}{
		{name: "member can list", method: http.MethodGet, path: "/api/v1/orgs/acme/assets", role: types.OrganizationRoleMember, wantStatus: http.StatusNoContent},
		{name: "member can view", method: http.MethodGet, path: "/api/v1/orgs/acme/assets/a-server", role: types.OrganizationRoleMember, wantStatus: http.StatusNoContent},
		{name: "member cannot create", method: http.MethodPost, path: "/api/v1/orgs/acme/assets", role: types.OrganizationRoleMember, wantStatus: http.StatusForbidden},
		{name: "member cannot update", method: http.MethodPatch, path: "/api/v1/orgs/acme/assets/a-server", role: types.OrganizationRoleMember, wantStatus: http.StatusForbidden},
		{name: "member cannot delete", method: http.MethodDelete, path: "/api/v1/orgs/acme/assets/a-server", role: types.OrganizationRoleMember, wantStatus: http.StatusForbidden},
		{name: "owner can create", method: http.MethodPost, path: "/api/v1/orgs/acme/assets", role: types.OrganizationRoleOwner, wantStatus: http.StatusNoContent},
		{name: "admin can update", method: http.MethodPatch, path: "/api/v1/orgs/acme/assets/a-server", role: types.OrganizationRoleMember, admin: true, wantStatus: http.StatusNoContent},
		{name: "unrelated route keeps member mutation access", method: http.MethodPost, path: "/api/v1/orgs/acme/workers", role: types.OrganizationRoleMember, wantStatus: http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &helixOrgRouteTestStore{role: tt.role}
			server := &HelixAPIServer{Store: store}
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
			handler := server.withHelixOrgIdentity(next)
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req = mux.SetURLVars(req, map[string]string{"org": "acme"})
			req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user-1", Admin: tt.admin}))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestHelixOrgAssetsAPIIntegrationRBAC(t *testing.T) {
	member := types.User{ID: "user-member"}
	owner := types.User{ID: "user-owner"}

	memberHandler := newAssetRBACIntegrationHandler(t, &helixOrgRouteTestStore{role: types.OrganizationRoleMember})
	response := assetRBACRequest(t, memberHandler, &member, http.MethodGet, "/api/v1/orgs/acme/assets", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("member list status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	response = assetRBACRequest(t, memberHandler, &member, http.MethodPost, "/api/v1/orgs/acme/assets", orgapi.CreateAssetRequest{
		Name: "production", Kind: asset.KindServer,
		Server: &orgapi.ServerAssetWriteRequest{Address: "10.0.0.8", User: "ubuntu", AuthType: asset.AuthSSHKey},
	})
	if response.Code != http.StatusForbidden {
		t.Fatalf("member create status = %d, want %d; body=%s", response.Code, http.StatusForbidden, response.Body.String())
	}

	ownerStore := &helixOrgRouteTestStore{role: types.OrganizationRoleOwner}
	ownerHandler := newAssetRBACIntegrationHandler(t, ownerStore)
	response = assetRBACRequest(t, ownerHandler, &owner, http.MethodPost, "/api/v1/orgs/acme/assets", orgapi.CreateAssetRequest{
		Name: "production", NotesForAgents: "Deploy only after draining traffic.", Kind: asset.KindServer,
		Server: &orgapi.ServerAssetWriteRequest{Address: "10.0.0.8", User: "ubuntu", AuthType: asset.AuthSSHKey},
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("owner create status = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	var created orgapi.AssetDTO
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("decode created asset: %v", err)
	}
	if created.Server == nil || created.Server.PublicKey == "" || strings.Contains(response.Body.String(), "private-key") {
		t.Fatalf("create response did not expose only the server public key: %s", response.Body.String())
	}

	ownerStore.role = types.OrganizationRoleMember
	response = assetRBACRequest(t, ownerHandler, &member, http.MethodGet, "/api/v1/orgs/acme/assets/"+created.ID, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("member view status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}

	description := "updated"
	response = assetRBACRequest(t, ownerHandler, &member, http.MethodPatch, "/api/v1/orgs/acme/assets/"+created.ID, orgapi.UpdateAssetRequest{Description: &description})
	if response.Code != http.StatusForbidden {
		t.Fatalf("member update status = %d, want %d; body=%s", response.Code, http.StatusForbidden, response.Body.String())
	}
	ownerStore.role = types.OrganizationRoleOwner
	response = assetRBACRequest(t, ownerHandler, &owner, http.MethodPatch, "/api/v1/orgs/acme/assets/"+created.ID, orgapi.UpdateAssetRequest{Description: &description})
	if response.Code != http.StatusOK {
		t.Fatalf("owner update status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}

	nonMemberHandler := newAssetRBACIntegrationHandler(t, &helixOrgRouteTestStore{membershipErr: errors.New("not a member")})
	response = assetRBACRequest(t, nonMemberHandler, &member, http.MethodGet, "/api/v1/orgs/acme/assets", nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-member list status = %d, want %d; body=%s", response.Code, http.StatusForbidden, response.Body.String())
	}
	response = assetRBACRequest(t, ownerHandler, nil, http.MethodGet, "/api/v1/orgs/acme/assets", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list status = %d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
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
	for _, b := range []orgchart.Node{
		mustBot(t, "bot-legacy-a", "", now).WithAgentID("app-legacy-a"),
		mustBot(t, "bot-legacy-b", "", now),
		mustBot(t, "bot-created", "", now),
		mustBot(t, "human-owner", orgchart.NodeKindHuman, now),
	} {
		if err := st.Nodes.Create(ctx, b); err != nil {
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

func mustBot(t *testing.T, id string, kind orgchart.NodeKind, now time.Time) orgchart.Node {
	t.Helper()
	b, err := orgchart.NewNode(orgchart.NodeID(id), "test bot", nil, now, "org-test")
	if err != nil {
		t.Fatal(err)
	}
	return b.WithKind(kind)
}
