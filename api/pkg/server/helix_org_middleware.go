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

	"github.com/helixml/helix/api/pkg/org/application/bootstrap"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
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

	mu           sync.Mutex
	bootstrapped map[string]bool
}

// newHelixOrgScope wires the data the middleware needs. configs and
// orgStore are the same instances handed to the helix-org handler;
// envsRoot is the parent directory under which `<orgID>/w-owner/`
// will land at bootstrap time.
func newHelixOrgScope(configs *configregistry.Registry, orgStore *helixorgstore.Store, envsRoot string, hs helixstore.Store) *helixOrgScope {
	return &helixOrgScope{
		configs:      configs,
		orgStore:     orgStore,
		envsRoot:     envsRoot,
		helixStore:   hs,
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

	envsDir := filepath.Join(s.envsRoot, orgID)
	ownerEnvPath := filepath.Join(envsDir, "w-owner")
	if err := osMkdirAll(ownerEnvPath); err != nil {
		return err
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
		return err
	}

	// Provision a per-org Helix service api_key. Tied to the first
	// admin user found — see ensureHelixOrgServiceAPIKey for the
	// idempotency story.
	if _, err := ensureHelixOrgServiceAPIKey(ctx, orgID, s.helixStore, s.configs); err != nil {
		log.Warn().Err(err).Str("org_id", orgID).Msg("helix-org service api key not provisioned")
	}

	s.mu.Lock()
	s.bootstrapped[orgID] = true
	s.mu.Unlock()
	return nil
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
