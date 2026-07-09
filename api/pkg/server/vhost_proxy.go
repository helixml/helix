package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/webservice"
	"github.com/rs/zerolog/log"
)

// errUpstreamUnavailable signals that hydra reached us but could not reach the
// app container (app down / still starting). ModifyResponse raises it so the
// ErrorHandler serves the branded holding page instead of leaking hydra's 502
// body (which carries the internal container IP) to the public.
var errUpstreamUnavailable = errors.New("web service upstream unavailable")

// proxyToContainer forwards an HTTP request to a port inside a sandbox
// container via the hydra dev-container proxy, reached over RevDial.
//
// sandboxID identifies the runner host for RevDial (hydra-<sandboxID> is
// the device key on connman). hydraContainerID is the key hydra uses to
// look up the container — for session-backed dev containers this is the
// session ID; for sandbox-API-backed containers this is the sandbox ID
// itself.
//
// Implementation: httputil.ReverseProxy with a custom RoundTripper that
// dials RevDial. Using the stdlib reverse proxy gives us correct
// hop-by-hop header handling, HTTP/2 upgrade support, trailer
// propagation, and — critically — CodeQL recognises it as a sanctioned
// reverse-proxy pattern (not a hand-rolled XSS vector).
func (apiServer *HelixAPIServer) proxyToContainer(
	w http.ResponseWriter,
	r *http.Request,
	sandboxID, hydraContainerID string,
	port int,
	targetPath string,
	// projectID identifies the web-service project so the holding page can be
	// state-aware ("starting up" vs "temporarily unavailable"). Empty for
	// sandbox previews, which always get the optimistic starting-up page.
	projectID string,
) {
	if sandboxID == "" {
		http.Error(w, "no sandbox associated with route", http.StatusServiceUnavailable)
		return
	}

	// hydra needs the original path encoded under its dev-container
	// proxy URL. Build it as a parsed URL so reverse-proxy's Director
	// can splice query/path correctly.
	hydraPath := fmt.Sprintf("/api/v1/dev-containers/%s/proxy/%d%s", hydraContainerID, port, targetPath)
	target := &url.URL{
		Scheme: "http",
		Host:   "hydra", // pseudo-host; the RoundTripper ignores it
		Path:   hydraPath,
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		// httputil rewrites Host to the target ("hydra"); restore the
		// client's Host as X-Forwarded-Host so the upstream sees the
		// original request hostname.
		req.Header.Set("X-Forwarded-Host", r.Host)
		req.URL.Path = hydraPath
		req.URL.RawPath = ""
		req.URL.RawQuery = r.URL.RawQuery
	}

	proxy.Transport = &revdialTransport{
		apiServer:  apiServer,
		sandboxID:  sandboxID,
		dialTimeout: 60 * time.Second,
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		// hydra reached us but couldn't reach the app container. Its 502 body
		// names the internal container IP — never surface that on a customer's
		// site. Signal the ErrorHandler to serve the branded holding page.
		if resp.Header.Get("X-Helix-Upstream-Unavailable") != "" {
			_ = resp.Body.Close()
			return errUpstreamUnavailable
		}
		return nil
	}

	proxy.ErrorHandler = func(rw http.ResponseWriter, _ *http.Request, err error) {
		// Log the detail; NEVER echo the raw Go error to the client (it read
		// "upstream error: dial hydra: ..." — a stack-trace string on a
		// customer's site). Serve a branded, auto-refreshing "starting up"
		// page so a transient blip or an in-flight recovery looks like a
		// spinner, not a crash.
		log.Warn().Err(err).
			Str("sandbox_id", sandboxID).
			Str("hydra_container_id", hydraContainerID).
			Int("port", port).
			Msg("vhost proxy error")
		apiServer.serveHoldingPage(r.Context(), rw, projectID)
	}

	proxy.ServeHTTP(w, r)
}

// writeStartingUpPage serves a branded, auto-refreshing holding page when the
// upstream web-service container is briefly unreachable (transient blip or an
// in-flight auto-recovery). Retry-After + meta-refresh mean the browser retries
// on its own, so a recovering service shows a spinner instead of an error.
func writeStartingUpPage(rw http.ResponseWriter) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Retry-After", "3")
	rw.Header().Set("Cache-Control", "no-store")
	rw.WriteHeader(http.StatusServiceUnavailable)
	_, _ = io.WriteString(rw, startingUpHTML)
}

const startingUpHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="3">
<title>Starting up…</title>
<style>
  html,body{height:100%;margin:0}
  body{display:flex;align-items:center;justify-content:center;
    font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;
    background:#0b0d12;color:#e6e9ef}
  .card{text-align:center;max-width:32rem;padding:2rem}
  .spinner{width:42px;height:42px;margin:0 auto 1.25rem;border:4px solid rgba(255,255,255,.12);
    border-top-color:#5b8cff;border-radius:50%;animation:spin 1s linear infinite}
  @keyframes spin{to{transform:rotate(360deg)}}
  h1{font-size:1.25rem;font-weight:600;margin:0 0 .5rem}
  p{margin:0;color:#9aa4b2;font-size:.95rem;line-height:1.5}
</style>
</head>
<body>
  <div class="card">
    <div class="spinner"></div>
    <h1>This service is starting up</h1>
    <p>It’ll be ready in a moment — this page refreshes automatically.</p>
  </div>
</body>
</html>`

// serveHoldingPage picks the public holding page for an unreachable web service.
// "Starting up" (optimistic, spinner, fast refresh) is only honest while a deploy
// is genuinely in flight; a failed/crashed service that isn't being redeployed
// gets "temporarily unavailable" instead. Sandbox previews (projectID == "")
// always get the starting-up page since they are booting.
func (apiServer *HelixAPIServer) serveHoldingPage(ctx context.Context, rw http.ResponseWriter, projectID string) {
	if projectID != "" && !apiServer.webServiceDeployInFlight(ctx, projectID) {
		writeUnavailablePage(rw)
		return
	}
	writeStartingUpPage(rw)
}

// webServiceDeployInFlight reports whether the project's latest deploy is still
// pending/building within the build window. Mirrors Controller.Health's
// "deploying" state but without the live probe (we're already on the error path,
// so the container is known-unreachable).
func (apiServer *HelixAPIServer) webServiceDeployInFlight(ctx context.Context, projectID string) bool {
	deploys, err := apiServer.Store.ListWebServiceDeploys(ctx, projectID, 1)
	if err != nil || len(deploys) == 0 {
		return false
	}
	d := deploys[0]
	inflight := d.Status == types.WebServiceDeployStatusPending || d.Status == types.WebServiceDeployStatusBuilding
	return inflight && time.Since(d.StartedAt) < webservice.DeployBuildTimeout
}

// writeUnavailablePage serves a branded holding page for a web service that is
// down and NOT currently deploying. It still auto-refreshes (slower) so the page
// recovers on its own once the owner redeploys, but it does not falsely claim the
// service is "starting up". No failure detail is exposed — that stays private to
// the project's Web Service tab.
func writeUnavailablePage(rw http.ResponseWriter) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Retry-After", "30")
	rw.Header().Set("Cache-Control", "no-store")
	rw.WriteHeader(http.StatusServiceUnavailable)
	_, _ = io.WriteString(rw, unavailableHTML)
}

const unavailableHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="30">
<title>Temporarily unavailable</title>
<style>
  html,body{height:100%;margin:0}
  body{display:flex;align-items:center;justify-content:center;
    font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;
    background:#0b0d12;color:#e6e9ef}
  .card{text-align:center;max-width:32rem;padding:2rem}
  h1{font-size:1.25rem;font-weight:600;margin:0 0 .5rem}
  p{margin:0;color:#9aa4b2;font-size:.95rem;line-height:1.5}
</style>
</head>
<body>
  <div class="card">
    <h1>This service is temporarily unavailable</h1>
    <p>Please check back shortly.</p>
  </div>
</body>
</html>`

// revdialTransport is a RoundTripper that dials hydra over RevDial,
// writes the request, and reads the response. One TCP conn per request
// (matches the previous hand-rolled flow). HTTP-keepalive pooling is a
// future optimisation — the existing dial cost is low because connman
// keeps the per-device control connection warm.
type revdialTransport struct {
	apiServer   *HelixAPIServer
	sandboxID   string
	dialTimeout time.Duration
}

// dialRetryWindow bounds how long RoundTrip retries a failing dial before
// giving up to the branded holding page. Short enough that a genuinely-down
// service shows the auto-refreshing page quickly, long enough to ride through
// the sub-second gap when a container restarts in place or a revdial control
// connection re-establishes.
const dialRetryWindow = 6 * time.Second

func (t *revdialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	hydraClient := hydra.NewRevDialClient(t.apiServer.connman, "hydra-"+t.sandboxID)

	// Retry ONLY the dial — nothing has been written yet, so this can't
	// double-submit a non-idempotent request. This absorbs transient revdial
	// races and the moment a container is coming back up during recovery.
	var conn net.Conn
	var err error
	deadline := time.Now().Add(dialRetryWindow)
	for {
		dctx, cancel := context.WithTimeout(req.Context(), t.dialTimeout)
		conn, err = t.apiServer.connman.Dial(dctx, hydraClient.DeviceID())
		cancel()
		if err == nil {
			break
		}
		if req.Context().Err() != nil || time.Now().After(deadline) {
			return nil, fmt.Errorf("dial hydra: %w", err)
		}
		time.Sleep(250 * time.Millisecond)
	}

	if err := req.Write(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("write request: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read response: %w", err)
	}
	// Body wraps both the response and the underlying conn so we
	// release the dial as soon as the client finishes reading.
	resp.Body = &connReleasingBody{ReadCloser: resp.Body, conn: conn}
	return resp, nil
}

// connReleasingBody closes the RevDial conn once the response body is
// drained / closed by the reverse proxy.
type connReleasingBody struct {
	io.ReadCloser
	conn net.Conn
}

func (b *connReleasingBody) Close() error {
	bodyErr := b.ReadCloser.Close()
	connErr := b.conn.Close()
	if bodyErr != nil {
		return bodyErr
	}
	return connErr
}
