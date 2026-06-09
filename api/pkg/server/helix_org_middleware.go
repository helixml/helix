package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"

	"github.com/helixml/helix/api/pkg/org/application/bootstrap"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/topology"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	helixstore "github.com/helixml/helix/api/pkg/store"
)

// helixOrgScope bundles the per-org state the middleware needs to
// pass into bootstrap and into the handlers. EnvsDir is the per-org
// working-directory root (envs land under <root>/<orgID>/) so two
// orgs can both have a `w-owner` Worker without clashing on disk.
type helixOrgScope struct {
	configs    *configregistry.Registry
	orgStore   *helixorgstore.Store
	envsRoot   string
	helixStore helixstore.Store

	// mirror's EnsureAll runs after bootstrap so pre-existing /
	// inline-chat-only workers are mirrored without an activation first.
	mirror *runtimehelix.Mirror

	mu           sync.Mutex
	bootstrapped map[string]bool
	// bootstrapFlight dedupes concurrent first-load races on the same
	// org. The HelixOrgChart page fires several React Query hooks in
	// parallel (/chart, /workers, /roles, /streams, …) and every one
	// of those handlers funnels through ensureBootstrap. Without
	// singleflight the per-org mutex only guarded the `bootstrapped`
	// map flag, so multiple goroutines could enter bootstrap.Run at
	// once — the winner created r-owner / w-owner, every loser
	// returned "create owner role: already exists" (HTTP 500). The
	// browser then served the 500 on the first paint and only worked
	// after a refresh (when `bootstrapped[orgID]` was already true).
	bootstrapFlight singleflight.Group
}

// newHelixOrgScope wires the data the middleware needs. configs and
// orgStore are the same instances handed to the helix-org handler;
// envsRoot is the parent directory under which `<orgID>/w-owner/`
// will land at bootstrap time.
func newHelixOrgScope(configs *configregistry.Registry, orgStore *helixorgstore.Store, envsRoot string, hs helixstore.Store, mirror *runtimehelix.Mirror) *helixOrgScope {
	return &helixOrgScope{
		configs:      configs,
		orgStore:     orgStore,
		envsRoot:     envsRoot,
		helixStore:   hs,
		mirror:       mirror,
		bootstrapped: map[string]bool{},
	}
}

// ensureBootstrap materialises the per-org owner Worker + structural
// grants on first request for an org. Subsequent calls fast-path on
// ErrAlreadyInitialised. Also provisions the helix.api_key into the
// org's config registry so the spawner can use it.
//
// The envsDir under <envsRoot>/<orgID>/ is created on demand.
func (s *helixOrgScope) ensureBootstrap(ctx context.Context, orgID string) error {
	s.mu.Lock()
	if s.bootstrapped[orgID] {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	// singleflight collapses concurrent first-load callers for the
	// same orgID into a single bootstrap.Run; losers wait for the
	// winner and inherit its (err) result. This prevents the
	// duplicate-key race described on bootstrapFlight.
	_, err, _ := s.bootstrapFlight.Do(orgID, func() (any, error) {
		// Re-check under the flight: if a prior flight already
		// finished and flipped the flag, return immediately so we
		// don't repeat the work after the singleflight forgot the
		// key.
		s.mu.Lock()
		done := s.bootstrapped[orgID]
		s.mu.Unlock()
		if done {
			return nil, nil
		}

		envsDir := filepath.Join(s.envsRoot, orgID)
		ownerEnvPath := filepath.Join(envsDir, "w-owner")
		if err := osMkdirAll(ownerEnvPath); err != nil {
			return nil, err
		}

		switch result, err := bootstrap.Run(ctx, s.orgStore, bootstrap.Params{
			EnvironmentPath: ownerEnvPath,
			OrganizationID:  orgID,
		}); {
		case err == nil:
			log.Info().
				Str("org_id", orgID).
				Str("worker_id", string(result.WorkerID)).
				Msg("helix-org bootstrap created owner")
		case errors.Is(err, bootstrap.ErrAlreadyInitialised):
			// expected on subsequent boots after a previous bootstrap
		default:
			return nil, err
		}

		// Provision a per-org Helix service api_key. Tied to the
		// first admin user found — see ensureHelixOrgServiceAPIKey
		// for the idempotency story.
		if _, err := ensureHelixOrgServiceAPIKey(ctx, orgID, s.helixStore, s.configs); err != nil {
			log.Warn().Err(err).Str("org_id", orgID).Msg("helix-org service api key not provisioned")
		}

		// Converge the full topology for this org. Best-effort: a
		// failure is logged but does not break the request — the org
		// is still accessible and future hire/reparent/fire mutations
		// will re-run Reconcile on the affected Workers. This catches
		// Workers hired before the topology reconciler was wired
		// (e.g. orgs upgraded from an older server version that
		// lacked team-stream auto-creation).
		rec := &topology.Reconciler{Store: s.orgStore}
		if err := rec.ReconcileAll(ctx, orgID); err != nil {
			log.Warn().Err(err).Str("org_id", orgID).Msg("helix-org topology reconcile-all failed")
		}

		// Mirror pre-existing workers (once per org per process).
		s.mirror.EnsureAll(ctx, orgID)

		s.mu.Lock()
		s.bootstrapped[orgID] = true
		s.mu.Unlock()
		return nil, nil
	})
	return err
}

// osMkdirAll is a tiny indirection so tests can stub if needed; for
// now it just calls os.MkdirAll with the canonical mode.
func osMkdirAll(path string) error {
	return os.MkdirAll(path, 0o750)
}

// withHelixOrgScope wraps the helix-org handler chain. It resolves
// the `{org}` URL segment (slug or org_id) via lookupOrg, authorises
// the caller is a member of that org, ensures the bootstrap has run
// for the org, and stashes the resolved orgID on the request context.
func (s *HelixAPIServer) withHelixOrgScope(scope *helixOrgScope, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgSlugOrID := mux.Vars(r)["org"]
		if orgSlugOrID == "" {
			http.Error(w, "missing org", http.StatusBadRequest)
			return
		}
		org, err := s.lookupOrg(r.Context(), orgSlugOrID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		user := getRequestUser(r)
		if user == nil {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		if _, err := s.authorizeOrgMember(r.Context(), user, org.ID); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if err := scope.ensureBootstrap(r.Context(), org.ID); err != nil {
			http.Error(w, "bootstrap: "+err.Error(), http.StatusInternalServerError)
			return
		}
		ctx := helixorgserver.WithOrgID(r.Context(), org.ID)
		// Strip the /orgs/{org}/helix-org prefix so the downstream
		// helix-org handler sees the same flat path it served from
		// /api/v1/org/* before — keeps the org-graph server unaware
		// of its mount point.
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

// stripOrgScopedPrefix strips "/api/v1/orgs/{org}" off the request
// URL before forwarding to next, so the downstream helix-org handler
// sees the same flat paths it serves from the standalone server's
// own mux (/chart, /workers, /roles, …). The {org} segment is
// captured by gorilla mux and stitched back into the prefix here.
func stripOrgScopedPrefix(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgSeg := mux.Vars(r)["org"]
		fullPrefix := strings.TrimRight(APIPrefix, "/") + "/orgs/" + orgSeg
		if !strings.HasPrefix(r.URL.Path, fullPrefix) {
			http.NotFound(w, r)
			return
		}
		stripped := r.Clone(r.Context())
		stripped.URL.Path = strings.TrimPrefix(r.URL.Path, fullPrefix)
		if stripped.URL.Path == "" {
			stripped.URL.Path = "/"
		}
		stripped.URL.RawPath = ""
		stripped.RequestURI = stripped.URL.RequestURI()
		next.ServeHTTP(w, stripped)
	})
}
