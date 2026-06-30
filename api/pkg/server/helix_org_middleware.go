package server

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/application/roles"
	"github.com/helixml/helix/api/pkg/org/application/slackrouting"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	"github.com/helixml/helix/api/pkg/server/helixorg"
	helixstore "github.com/helixml/helix/api/pkg/store"
)

// helixOrgScope bundles the per-org state the middleware needs to pass
// into the per-org setup and into the handlers.
type helixOrgScope struct {
	configs    *configregistry.Registry
	orgStore   *helixorgstore.Store
	helixStore helixstore.Store

	// mirror's EnsureAll runs on first request so pre-existing /
	// inline-chat-only workers are mirrored without an activation first.
	mirror *runtimehelix.Mirror

	// slackRoutes converges Slack auto-router routes on first request,
	// catching Workers hired while the server was down. nil when Slack
	// routing isn't wired.
	slackRoutes *slackrouting.Reconciler

	mu           sync.Mutex
	bootstrapped map[string]bool
	// bootstrapFlight dedupes concurrent first-load races on the same
	// org. The HelixOrgChart page fires several React Query hooks in
	// parallel (/chart, /workers, /roles, /streams, …) and every one of
	// those handlers funnels through ensureBootstrap; the singleflight
	// collapses them into a single per-org setup run.
	bootstrapFlight singleflight.Group
}

// newHelixOrgScope wires the data the middleware needs. configs and
// orgStore are the same instances handed to the helix-org handler.
func newHelixOrgScope(configs *configregistry.Registry, orgStore *helixorgstore.Store, hs helixstore.Store, mirror *runtimehelix.Mirror, slackRoutes *slackrouting.Reconciler) *helixOrgScope {
	return &helixOrgScope{
		configs:      configs,
		orgStore:     orgStore,
		helixStore:   hs,
		mirror:       mirror,
		slackRoutes:  slackRoutes,
		bootstrapped: map[string]bool{},
	}
}

// ensureBootstrap runs the per-org first-request setup: provision the
// helix.api_key into the org's config registry, converge any existing
// graph, and start the transcript mirror. No owner is seeded — orgs
// start empty. Runs once per org per process (guarded by bootstrapped).
func (s *helixOrgScope) ensureBootstrap(ctx context.Context, orgID string) error {
	s.mu.Lock()
	if s.bootstrapped[orgID] {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	// singleflight collapses concurrent first-load callers for the
	// same orgID into a single setup run; losers wait for the winner and
	// inherit its (err) result. This prevents duplicate-key races on the
	// per-org setup below.
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

		// No owner is seeded — orgs start empty. The human creates the
		// first Role + Worker from the chart UI. This first-request hook
		// only provisions the per-org service api_key and converges any
		// graph that already exists (a no-op on a brand-new empty org).

		// Provision a per-org Helix service api_key. Tied to the
		// first admin user found — see helixorg.HelixAPIKeys.Service for the
		// idempotency story.
		if _, err := helixorg.NewHelixAPIKeys(s.helixStore, s.configs).Service(ctx, orgID); err != nil {
			log.Warn().Err(err).Str("org_id", orgID).Msg("helix-org service api key not provisioned")
		}

		// Converge the full topology for this org. Best-effort: a
		// failure is logged but does not break the request — the org
		// is still accessible and future hire/reparent/fire mutations
		// will re-run Reconcile on the affected Workers. This catches
		// Workers hired before the topology reconciler was wired
		// (e.g. orgs upgraded from an older server version that
		// lacked team-stream auto-creation).
		rec := reconcile.New(reconcile.Deps{
			Workers:        s.orgStore.Workers,
			ReportingLines: s.orgStore.ReportingLines,
			Topics:         s.orgStore.Topics,
			Subscriptions:  s.orgStore.Subscriptions,
		})
		if err := rec.ReconcileAll(ctx, orgID); err != nil {
			log.Warn().Err(err).Str("org_id", orgID).Msg("helix-org topology reconcile-all failed")
		}

		// Converge Slack auto-router routes for this org too: catches Workers
		// hired while the server was down (or before this feature shipped).
		// No-op for orgs without an Automated router. Reuses the
		// composition-root reconciler so it has the real id-generator.
		if s.slackRoutes != nil {
			if err := s.slackRoutes.Reconcile(ctx, orgID); err != nil {
				log.Warn().Err(err).Str("org_id", orgID).Msg("helix-org slack route reconcile failed")
			}
		}

		// Backfill the universal read baseline on every Role in this
		// org. Catches Roles created before BaseReadTools existed —
		// e.g. an `r-qa-engineer` whose creator forgot `managers` and
		// `reports` (issue #2546). Best-effort like the topology
		// reconcile above: a failure logs and continues so a transient
		// DB error doesn't lock users out of the org.
		rolesSvc := roles.New(roles.Deps{Roles: s.orgStore.Roles, BaseTools: mcptools.BaseReadTools})
		if err := rolesSvc.Reconcile(ctx, orgID); err != nil {
			log.Warn().Err(err).Str("org_id", orgID).Msg("helix-org role reconcile failed")
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
