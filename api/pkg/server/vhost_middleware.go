package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/vhost"
	"github.com/rs/zerolog/log"
)

// VHostMiddlewareConfig is parsed once at server startup.
type VHostMiddlewareConfig struct {
	// CanonicalHostnames is the set of hostnames that fall through to the
	// main API/UI mux. Populated from SERVER_URL (one entry) and any
	// configured aliases.
	CanonicalHostnames map[string]struct{}

	// BaseDomain is the DEV_SUBDOMAIN-derived base for vhost routing
	// (e.g. "dev.helix.example.com"). Empty disables vhost dispatch
	// entirely — every request falls through to the main mux.
	BaseDomain string

	// Enabled is true if BaseDomain is set; convenience flag.
	Enabled bool
}

// parseVHostConfig builds the middleware config from the existing
// DEV_SUBDOMAIN and SERVER_URL env vars. No new env vars are introduced
// for the canonical hostname or the base domain; everything reuses
// existing config.
func parseVHostConfig(devSubdomainEnv, serverURL string) *VHostMiddlewareConfig {
	cfg := &VHostMiddlewareConfig{
		CanonicalHostnames: map[string]struct{}{},
	}

	// Canonical hostname from SERVER_URL.
	if canonical := hostnameOf(serverURL); canonical != "" {
		cfg.CanonicalHostnames[canonical] = struct{}{}
	}

	// Base domain from DEV_SUBDOMAIN. Format accepted: "dev" (uses
	// SERVER_URL's domain) or a full subdomain ("dev.helix.example.com").
	if devSubdomainEnv != "" {
		if strings.Contains(devSubdomainEnv, ".") {
			cfg.BaseDomain = strings.ToLower(strings.TrimSuffix(devSubdomainEnv, "."))
		} else if base := hostnameOf(serverURL); base != "" {
			cfg.BaseDomain = strings.ToLower(devSubdomainEnv + "." + base)
		}
		cfg.Enabled = cfg.BaseDomain != ""
	}
	return cfg
}

func hostnameOf(serverURL string) string {
	if serverURL == "" {
		return ""
	}
	raw := serverURL
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// VHostMiddleware dispatches incoming requests by Host header:
//   - canonical hostname → fall through to the main mux
//   - <share-*>.<base>   → look up vhost_routes (sandbox_preview)
//   - other host with a matching vhost_routes row → project web service
//   - everything else    → fall through to the main mux (404s from there)
//
// This replaces the old SubdomainProxyMiddleware (deleted) and the
// p{port}-{session_id} URL scheme it served.
type VHostMiddleware struct {
	cfg       *VHostMiddlewareConfig
	apiServer *HelixAPIServer
	next      http.Handler
}

// NewVHostMiddleware wires the middleware around the next handler
// (typically the gorilla mux router).
func NewVHostMiddleware(cfg *VHostMiddlewareConfig, apiServer *HelixAPIServer, next http.Handler) *VHostMiddleware {
	return &VHostMiddleware{cfg: cfg, apiServer: apiServer, next: next}
}

func (m *VHostMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(stripPort(r.Host))

	// 1. Canonical hostname → main mux.
	if _, isCanonical := m.cfg.CanonicalHostnames[host]; isCanonical {
		m.next.ServeHTTP(w, r)
		return
	}

	// If vhost feature isn't configured, behave like the old null
	// middleware: everything goes to main mux.
	if !m.cfg.Enabled {
		m.next.ServeHTTP(w, r)
		return
	}

	// 2. share-* preview tokens — only the leftmost label matters here.
	// The hostname must end in the configured base domain, and the
	// leftmost label must start with the share- prefix.
	if strings.HasSuffix(host, "."+m.cfg.BaseDomain) {
		leftmost := strings.SplitN(host, ".", 2)[0]
		if strings.HasPrefix(leftmost, vhost.SharePrefix) {
			m.serveVHostLookup(w, r, host, types.VHostTargetSandboxPreview)
			return
		}
	}

	// 3. Project web services — any other hostname that has a verified row.
	m.serveVHostLookup(w, r, host, types.VHostTargetProjectWebService)
}

// serveVHostLookup resolves a hostname via vhost_routes and dispatches
// to the appropriate proxy. expectedKind is used to refuse cross-kind
// matches (e.g. a row stored as project_web_service must not be reached
// via the share-* branch and vice versa).
func (m *VHostMiddleware) serveVHostLookup(w http.ResponseWriter, r *http.Request, hostname string, expectedKind types.VHostTargetKind) {
	route, err := m.apiServer.Store.GetVHostRouteByHostname(r.Context(), hostname)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Unknown host. Fall through to main mux so the existing
			// 404 page is served.
			m.next.ServeHTTP(w, r)
			return
		}
		log.Warn().Err(err).Str("host", hostname).Msg("vhost: store lookup error")
		http.Error(w, "vhost lookup failed", http.StatusInternalServerError)
		return
	}
	if route.TargetKind != expectedKind {
		// Misrouted (likely a misconfiguration or someone trying to use
		// the wrong dispatch branch). Treat as unknown.
		m.next.ServeHTTP(w, r)
		return
	}
	if route.VerifiedAt == nil {
		http.Error(w, "domain not yet verified", http.StatusServiceUnavailable)
		return
	}

	switch route.TargetKind {
	case types.VHostTargetSandboxPreview:
		m.dispatchSandboxPreview(w, r, route)
	case types.VHostTargetProjectWebService:
		m.dispatchProjectWebService(w, r, route)
	default:
		http.Error(w, "unknown route target kind", http.StatusInternalServerError)
	}
}

// dispatchSandboxPreview proxies a preview-token request to the
// underlying session or sandbox container. For sessions, the session's
// SandboxID is looked up to find the host device for RevDial.
func (m *VHostMiddleware) dispatchSandboxPreview(w http.ResponseWriter, r *http.Request, route *types.VHostRoute) {
	targetID := route.TargetID
	if strings.HasPrefix(targetID, "ses_") {
		sess, err := m.apiServer.Store.GetSession(r.Context(), targetID)
		if err != nil {
			http.Error(w, fmt.Sprintf("preview target session not found: %s", err), http.StatusNotFound)
			return
		}
		if sess.SandboxID == "" {
			http.Error(w, "preview target session has no sandbox", http.StatusServiceUnavailable)
			return
		}
		sb, err := m.apiServer.Store.GetSandbox(r.Context(), sess.SandboxID)
		if err != nil {
			http.Error(w, fmt.Sprintf("preview target sandbox not found: %s", err), http.StatusNotFound)
			return
		}
		m.apiServer.proxyToContainer(w, r, sb.HostDeviceID, targetID, route.Port, r.URL.Path)
		return
	}
	if strings.HasPrefix(targetID, "sbx_") {
		sb, err := m.apiServer.Store.GetSandbox(r.Context(), targetID)
		if err != nil {
			http.Error(w, fmt.Sprintf("preview target sandbox not found: %s", err), http.StatusNotFound)
			return
		}
		// Sandbox-API containers are registered in hydra under
		// req.SessionID = sandbox.ID (see sandbox/controller_provision.go).
		m.apiServer.proxyToContainer(w, r, sb.HostDeviceID, sb.ID, route.Port, r.URL.Path)
		return
	}
	http.Error(w, "unrecognised preview target id format", http.StatusBadRequest)
}

// dispatchProjectWebService looks up the project's active web-service
// sandbox and proxies to it. Returns 503 if the project has no active
// sandbox yet (e.g. the first deploy is still pending).
func (m *VHostMiddleware) dispatchProjectWebService(w http.ResponseWriter, r *http.Request, route *types.VHostRoute) {
	state, err := m.apiServer.Store.GetProjectWebServiceState(r.Context(), route.TargetID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "project web service not configured", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, fmt.Sprintf("web service state lookup: %s", err), http.StatusInternalServerError)
		return
	}
	if !state.Enabled {
		http.Error(w, "project web service is disabled", http.StatusServiceUnavailable)
		return
	}
	if state.ActiveSandboxID == "" {
		http.Error(w, "project web service has no active deployment", http.StatusServiceUnavailable)
		return
	}
	// Web service sandboxes are registered with hydra under their
	// sandbox ID (sandbox/controller_provision.go SessionID=sandbox.ID).
	// The RevDial device key, however, is hydra-<HostDeviceID> — that's
	// the runner-host pool sandboxes get assigned to, not the sandbox
	// itself.
	sb, err := m.apiServer.Store.GetSandbox(r.Context(), state.ActiveSandboxID)
	if err != nil {
		http.Error(w, fmt.Sprintf("active sandbox not found: %s", err), http.StatusBadGateway)
		return
	}
	m.apiServer.proxyToContainer(w, r, sb.HostDeviceID, sb.ID, route.Port, r.URL.Path)
}

// stripPort removes a trailing :port from a Host header value, taking
// care to leave IPv6 brackets intact.
func stripPort(host string) string {
	if i := strings.LastIndex(host, ":"); i >= 0 {
		if !strings.Contains(host, "]") || i > strings.LastIndex(host, "]") {
			return host[:i]
		}
	}
	return host
}

// vhostContextKey is kept here for future use (e.g. exposing the matched
// route to downstream handlers). Currently unused.
type vhostContextKey struct{}

func vhostRouteFromContext(ctx context.Context) *types.VHostRoute {
	if v, ok := ctx.Value(vhostContextKey{}).(*types.VHostRoute); ok {
		return v
	}
	return nil
}

var _ = vhostRouteFromContext // silence unused-warning until used
