package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/rs/zerolog/log"
)

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

	proxy.ErrorHandler = func(rw http.ResponseWriter, _ *http.Request, err error) {
		log.Warn().Err(err).
			Str("sandbox_id", sandboxID).
			Str("hydra_container_id", hydraContainerID).
			Int("port", port).
			Msg("vhost proxy error")
		http.Error(rw, fmt.Sprintf("upstream error: %s", err), http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}

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

func (t *revdialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	hydraClient := hydra.NewRevDialClient(t.apiServer.connman, "hydra-"+t.sandboxID)
	ctx, cancel := context.WithTimeout(req.Context(), t.dialTimeout)
	defer cancel()

	conn, err := t.apiServer.connman.Dial(ctx, hydraClient.DeviceID())
	if err != nil {
		return nil, fmt.Errorf("dial hydra: %w", err)
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
