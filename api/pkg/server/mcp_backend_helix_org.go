package server

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	"github.com/helixml/helix/api/pkg/server/helixorg"
	"github.com/helixml/helix/api/pkg/types"
)

// HelixOrgMCPBackend exposes the embedded helix-org MCP server through
// the Helix MCP gateway. The gateway sits behind Helix's standard auth
// chain, so by the time we get a request here the user has already
// been authenticated by api_key; we only need to enforce the alpha
// feature flag and forward to the in-process helix-org handler.
//
// The backend honours a Worker ID embedded in the suffix path:
// `/api/v1/mcp/helix-org/workers/<id>/mcp` routes to /workers/<id>/mcp
// in helix-org. Callers that don't specify a Worker default to
// `w-owner` (the single alpha owner). The Spawner uses this to scope
// each Worker activation to its own MCP path so subscribe/publish
// land on the activating Worker, not the owner.
type HelixOrgMCPBackend struct {
	apiServer  *HelixAPIServer
	orgHandler http.Handler
	scope      *helixOrgScope
}

// NewHelixOrgMCPBackend creates a backend that proxies to the in-process
// helix-org server handler. Tenants are identified by URL prefix
// (suffix path starts with `{org}/...`) so each per-Worker MCP call
// scopes to the right helix-org.
func NewHelixOrgMCPBackend(apiServer *HelixAPIServer, orgHandlers *helixOrgHandlers) *HelixOrgMCPBackend {
	return &HelixOrgMCPBackend{
		apiServer:  apiServer,
		orgHandler: orgHandlers.api,
		scope:      orgHandlers.scope,
	}
}

// ServeHTTP implements MCPBackend. The MCP gateway has already
// authenticated the request; we add the alpha-feature gate here so a
// user without the flag can't attach this MCP to an agent and call
// org tools, even though their api_key would otherwise satisfy the
// gateway. Defence in depth — the picker is alpha-gated already, but
// agents can be shared.
func (b *HelixOrgMCPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User) {
	if !hasAlphaFeature(user, helixorg.AlphaFeature) {
		log.Warn().Str("user_id", user.ID).Msg("helix-org MCP: user lacks alpha feature flag")
		http.Error(w, "forbidden: helix-org alpha not enabled for this user", http.StatusForbidden)
		return
	}

	// Parse the org segment + worker ID from the suffix path the gateway
	// captured. Accept forms:
	//   {org}/workers/<id>/mcp     (preferred — explicit)
	//   {org}                      (defaults to w-owner inside that org)
	suffix := strings.Trim(mux.Vars(r)["path"], "/")
	if suffix == "" {
		http.Error(w, "helix-org MCP: missing org in URL", http.StatusBadRequest)
		return
	}
	parts := strings.Split(suffix, "/")
	orgSlugOrID := parts[0]
	workerID := "w-owner"
	if len(parts) >= 4 && parts[1] == "workers" && parts[3] == "mcp" {
		workerID = parts[2]
	}
	org, err := b.apiServer.lookupOrg(r.Context(), orgSlugOrID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if _, err := b.apiServer.authorizeOrgMember(r.Context(), user, org.ID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := b.scope.ensureBootstrap(r.Context(), org.ID); err != nil {
		http.Error(w, "bootstrap: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rewritten := r.Clone(helixorgserver.WithOrgID(r.Context(), org.ID))
	rewritten.URL.Path = "/orgs/" + org.ID + "/workers/" + workerID + "/mcp"
	rewritten.URL.RawPath = ""
	rewritten.RequestURI = rewritten.URL.RequestURI()
	// Forward the authenticated user's ID so helix-org tools can
	// persist it onto domain state (e.g. hire_worker → WorkerState)
	// without holding the api_key. The Spawner later asks the
	// embedded SaaS to mint a fresh api_key for this user_id at
	// activation time — no tokens stored at rest.
	rewritten.Header.Set("X-Helix-Org-User-Id", user.ID)

	log.Trace().
		Str("user_id", user.ID).
		Str("worker_id", workerID).
		Str("orig_path", r.URL.Path).
		Msg("helix-org MCP: forwarding to in-process handler")

	b.orgHandler.ServeHTTP(w, rewritten)
}

// hasAlphaFeature reports whether the user has been granted name in
// their AlphaFeatures slice. nil-safe.
func hasAlphaFeature(user *types.User, name string) bool {
	if user == nil {
		return false
	}
	for _, f := range user.AlphaFeatures {
		if strings.EqualFold(f, name) {
			return true
		}
	}
	return false
}
