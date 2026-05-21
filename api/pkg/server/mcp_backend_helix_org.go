package server

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

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
	// orgHandler is the http.Handler returned by helixorgserver.Server.Handler().
	// It mounts /workers/{id}/mcp; we forward there after extracting the
	// worker ID from the gateway suffix.
	orgHandler http.Handler
}

// NewHelixOrgMCPBackend creates a backend that proxies to the in-process
// helix-org server handler. ownerWorkerID identifies which Worker's
// MCP scope is exposed (currently always "w-owner").
func NewHelixOrgMCPBackend(orgHandler http.Handler) *HelixOrgMCPBackend {
	return &HelixOrgMCPBackend{orgHandler: orgHandler}
}

// ServeHTTP implements MCPBackend. The MCP gateway has already
// authenticated the request; we add the alpha-feature gate here so a
// user without the flag can't attach this MCP to an agent and call
// org tools, even though their api_key would otherwise satisfy the
// gateway. Defence in depth — the picker is alpha-gated already, but
// agents can be shared.
func (b *HelixOrgMCPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User) {
	if !hasAlphaFeature(user, alphaFeatureHelixOrg) {
		log.Warn().Str("user_id", user.ID).Msg("helix-org MCP: user lacks alpha feature flag")
		http.Error(w, "forbidden: helix-org alpha not enabled for this user", http.StatusForbidden)
		return
	}

	// Parse the worker ID from the suffix path the gorilla mux
	// captured. Accept forms:
	//   workers/<id>/mcp           (preferred — explicit)
	//   <id>                       (legacy shorthand, treated as worker id)
	//   ""                         (default to w-owner)
	suffix := strings.Trim(mux.Vars(r)["path"], "/")
	workerID := "w-owner"
	if suffix != "" {
		parts := strings.Split(suffix, "/")
		if len(parts) >= 3 && parts[0] == "workers" && parts[2] == "mcp" {
			workerID = parts[1]
		} else if len(parts) == 1 {
			workerID = parts[0]
		}
	}

	rewritten := r.Clone(r.Context())
	rewritten.URL.Path = "/workers/" + workerID + "/mcp"
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
