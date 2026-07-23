package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// missingOrganizationHelixStore makes service-key provisioning fail;
// bootstrap must remain best-effort.
type missingOrganizationHelixStore struct {
	helixstore.Store
}

func (s *missingOrganizationHelixStore) GetOrganization(_ context.Context, _ *helixstore.GetOrganizationQuery) (*types.Organization, error) {
	return nil, errors.New("organization not found")
}

type helixOrgRouteTestStore struct {
	helixstore.Store
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
	scope := newHelixOrgScope(
		configregistry.New(orgStore.Configs),
		orgStore,
		&missingOrganizationHelixStore{},
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
