package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/rs/zerolog/log"
)

// proxyToContainer forwards an HTTP request to a port inside a sandbox
// container via the hydra dev-container proxy, reached over RevDial.
//
// sandboxID is the runner-side sandbox ID (used to dial hydra), and
// hydraContainerID is the key hydra uses to look up the container (for
// session-backed dev containers this is the session ID; for
// sandbox-API-backed containers this is the sandbox ID itself).
//
// Extracted from the deleted proxyToSessionPort so both preview-token
// dispatch and project-web-service dispatch can share one
// implementation.
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
	if targetPath == "" {
		targetPath = "/"
	}

	hydraPath := fmt.Sprintf("/api/v1/dev-containers/%s/proxy/%d%s", hydraContainerID, port, targetPath)
	if r.URL.RawQuery != "" {
		hydraPath += "?" + r.URL.RawQuery
	}

	hydraClient := hydra.NewRevDialClient(apiServer.connman, "hydra-"+sandboxID)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	conn, err := apiServer.connman.Dial(ctx, hydraClient.DeviceID())
	if err != nil {
		log.Warn().Err(err).
			Str("sandbox_id", sandboxID).
			Str("hydra_container_id", hydraContainerID).
			Int("port", port).
			Msg("vhost proxy: failed to dial hydra via RevDial")
		http.Error(w, fmt.Sprintf("failed to connect to sandbox: %s", err), http.StatusBadGateway)
		return
	}
	defer conn.Close()

	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, "http://hydra"+hydraPath, r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create proxy request: %s", err), http.StatusInternalServerError)
		return
	}

	for key, values := range r.Header {
		switch strings.ToLower(key) {
		case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
			"te", "trailers", "transfer-encoding", "upgrade":
			continue
		}
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}
	proxyReq.Header.Set("X-Forwarded-For", r.RemoteAddr)
	proxyReq.Header.Set("X-Forwarded-Host", r.Host)
	if r.TLS != nil {
		proxyReq.Header.Set("X-Forwarded-Proto", "https")
	} else {
		proxyReq.Header.Set("X-Forwarded-Proto", "http")
	}

	if err := proxyReq.Write(conn); err != nil {
		log.Warn().Err(err).Msg("vhost proxy: failed to write request to hydra")
		http.Error(w, fmt.Sprintf("failed to send request: %s", err), http.StatusBadGateway)
		return
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), proxyReq)
	if err != nil {
		log.Warn().Err(err).Msg("vhost proxy: failed to read response from hydra")
		http.Error(w, fmt.Sprintf("failed to read response: %s", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Debug().Err(err).Msg("vhost proxy: error streaming response")
	}
}
