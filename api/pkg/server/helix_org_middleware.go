package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/helixevents"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/application/slackrouting"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	"github.com/helixml/helix/api/pkg/server/helixorg"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
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

	// helixEvents ensures the org's single "Helix events" topic exists on
	// first request. nil when not wired.
	helixEvents *helixevents.Reconciler

	// humanReconcile makes the org's human nodes match its membership on
	// first request (the correctness backstop for the inline membership
	// hooks — see org_graph_seed.go). nil when helix-org / the seeder isn't
	// wired.
	humanReconcile func(ctx context.Context, orgID string) error
	botRepair      func(ctx context.Context, orgID, serviceKey string) error
	botTools       *nodes.Nodes

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
func newHelixOrgScope(configs *configregistry.Registry, orgStore *helixorgstore.Store, hs helixstore.Store, mirror *runtimehelix.Mirror, slackRoutes *slackrouting.Reconciler, helixEvents *helixevents.Reconciler) *helixOrgScope {
	return &helixOrgScope{
		configs:      configs,
		orgStore:     orgStore,
		helixStore:   hs,
		mirror:       mirror,
		slackRoutes:  slackRoutes,
		helixEvents:  helixEvents,
		bootstrapped: map[string]bool{},
	}
}

// ensureBootstrap runs the per-org first-request setup: provision the
// helix.api_key into the org's config registry, converge any existing
// graph, and start the transcript mirror. Runs once per org per process
// (guarded by bootstrapped).
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

		// Provision a per-org Helix service api_key for the organization owner.
		serviceKey, err := helixorg.NewHelixAPIKeys(s.helixStore, s.configs).Service(ctx, orgID)
		if err != nil {
			return nil, fmt.Errorf("provision helix-org service api key: %w", err)
		}

		if s.botRepair != nil {
			if err := s.botRepair(ctx, orgID, serviceKey); err != nil {
				return nil, fmt.Errorf("repair helix-org bots: %w", err)
			}
		}

		// Converge the full topology for this org. Best-effort: a
		// failure is logged but does not break the request — the org
		// is still accessible and future hire/reparent/fire mutations
		// will re-run Reconcile on the affected Workers. This catches
		// Workers hired before the topology reconciler was wired
		// (e.g. orgs upgraded from an older server version that
		// lacked team-channel auto-creation).
		rec := reconcile.New(reconcile.Deps{
			Nodes:          s.orgStore.Nodes,
			ReportingLines: s.orgStore.ReportingLines,
			Triggers:       s.orgStore.Triggers,
			Attachments:    s.orgStore.WorkerAttachments,
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

		// Ensure the org's single "Helix events" topic exists. Best-effort
		// like the reconciles above: a failure logs and continues (the
		// attention publisher also ensures it defensively on first event).
		if s.helixEvents != nil {
			if err := s.helixEvents.Reconcile(ctx, orgID); err != nil {
				log.Warn().Err(err).Str("org_id", orgID).Msg("helix-org helix events topic reconcile failed")
			}
		}

		// Backfill the universal read baseline on every Role in this
		// org. Catches Roles created before BaseReadTools existed —
		// e.g. an `r-qa-engineer` whose creator forgot `managers` and
		// `reports` (issue #2546). Best-effort like the topology
		// reconcile above: a failure logs and continues so a transient
		// DB error doesn't lock users out of the org.
		botsSvc := s.botTools
		if botsSvc == nil {
			botsSvc = nodes.New(nodes.Deps{Nodes: s.orgStore.Nodes, BaseTools: mcptools.BaseReadTools})
		}
		if err := botsSvc.Reconcile(ctx, orgID); err != nil {
			log.Warn().Err(err).Str("org_id", orgID).Msg("helix-org role reconcile failed")
		}

		// Converge human nodes against org membership: create a node for any
		// member missing one (covers OIDC joins + members added before this
		// feature) and remove orphans. Best-effort like the reconciles above.
		if s.humanReconcile != nil {
			if err := s.humanReconcile(ctx, orgID); err != nil {
				log.Warn().Err(err).Str("org_id", orgID).Msg("helix-org human-node reconcile failed")
			}
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

type repairAgentDefaultApplier interface {
	ApplyAgentDefaults(ctx context.Context, appID string, defaults types.AssistantConfig) error
}

func repairNeverActivatedBots(ctx context.Context, orgID string, st *helixorgstore.Store, dispatcher lifecycle.CreateDispatcher, configs *configregistry.Registry, applier repairAgentDefaultApplier) error {
	if st == nil || dispatcher == nil || configs == nil || !configs.IsDefaultAgentConfigComplete(ctx, orgID) {
		return nil
	}
	defaults, err := configs.GetDefaultAgentConfig(ctx, orgID)
	if err != nil {
		return fmt.Errorf("read default agent config: %w", err)
	}
	bs, err := st.Nodes.List(ctx, orgID)
	if err != nil {
		return err
	}
	for _, b := range bs {
		if b.IsHuman() {
			continue
		}
		acts, err := st.Activations.ListForWorker(ctx, orgID, b.ID, 1)
		if err != nil {
			return fmt.Errorf("list activations for bot %s: %w", b.ID, err)
		}
		if len(acts) > 0 {
			continue
		}
		if b.AgentID != "" {
			if applier == nil {
				return fmt.Errorf("apply defaults to never-activated bot %s: applier is not wired", b.ID)
			}
			if err := applier.ApplyAgentDefaults(ctx, b.AgentID, defaults); err != nil {
				return fmt.Errorf("apply defaults to never-activated bot %s: %w", b.ID, err)
			}
		}
		actID := activation.ID("a-repair-" + string(b.ID))
		act, err := activation.New(actID, b.ID, []activation.Trigger{{Kind: activation.TriggerHire}}, time.Now().UTC(), orgID)
		if err != nil {
			return fmt.Errorf("build repair activation for bot %s: %w", b.ID, err)
		}
		if err := st.Activations.Create(ctx, act); err != nil {
			if _, getErr := st.Activations.Get(ctx, orgID, actID); getErr == nil {
				continue
			}
			return fmt.Errorf("persist repair activation for bot %s: %w", b.ID, err)
		}
		dispatcher.DispatchHire(ctx, orgID, b.ID, actID)
	}
	return nil
}

// withHelixOrgScope adds org-graph bootstrap to the shared authenticated
// organization context.
func (s *HelixAPIServer) withHelixOrgScope(scope *helixOrgScope, next http.Handler) http.Handler {
	return s.withHelixOrgIdentity(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID := helixorgserver.OrgIDFromContext(r.Context())
		if err := scope.ensureBootstrap(r.Context(), orgID); err != nil {
			http.Error(w, "bootstrap: "+err.Error(), http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// withHelixOrgIdentity resolves the organization, checks membership, and
// stores the canonical organization ID and authenticated user on context.
func (s *HelixAPIServer) withHelixOrgIdentity(next http.Handler) http.Handler {
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
		membership, err := s.authorizeOrgMember(r.Context(), user, org.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if isHelixOrgPrivilegedMutation(r) && !isAdmin(user) && membership.Role != types.OrganizationRoleOwner {
			http.Error(w, "only organization owners and administrators can modify this resource", http.StatusForbidden)
			return
		}
		ctx := helixorgserver.WithOrgID(r.Context(), org.ID)
		// Bridge the authenticated caller into the runtime-helix context
		// so lifecycle.Create persists them as the Bot's hiring user
		// (SaveHiringUser reads runtimehelix.UserIDFromContext). Without
		// this every Bot's session falls back to the org-service identity
		// (organization owner), which cross-attributes another user's key into
		// the tenant org's API-keys list.
		ctx = runtimehelix.WithUserID(ctx, user.ID)
		// Strip the /orgs/{org}/helix-org prefix so the downstream
		// helix-org handler sees the same flat path it served from
		// /api/v1/org/* before — keeps the org-graph server unaware
		// of its mount point.
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

func isHelixOrgPrivilegedMutation(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	orgSegment := mux.Vars(r)["org"]
	assetsPath := strings.TrimRight(APIPrefix, "/") + "/orgs/" + orgSegment + "/assets"
	if r.URL.Path == assetsPath || strings.HasPrefix(r.URL.Path, assetsPath+"/") {
		return true
	}
	agentsPath := strings.TrimRight(APIPrefix, "/") + "/orgs/" + orgSegment + "/agents/"
	if !strings.HasPrefix(r.URL.Path, agentsPath) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, agentsPath), "/")
	return len(parts) >= 3 && parts[0] != "" && parts[1] == "secrets" && parts[2] != ""
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
