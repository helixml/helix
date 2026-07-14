package server

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
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
