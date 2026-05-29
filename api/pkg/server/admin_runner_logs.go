package server

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/proxy"
)

// streamAdminRunnerLogs streams a Runner's hydra-aggregated log buffer over a
// WebSocket so an admin can live-tail what the Runner is doing without SSH.
//
// Route: GET /api/v1/admin/runners/{runner_id}/logs
// Query: tail=N (default 200, max 5000), follow=bool (default true)
//
// Auth is enforced by the admin subrouter middleware. Multiple admins can open
// the same stream concurrently — hydra fans the same buffer to all
// subscribers; there is no supersede semantic here (unlike the screenshot
// stream WS which dedupes per browser tab).
//
// Pattern mirrors the screenshot stream WS proxy at
// external_agent_handlers.go: hijack, dial hydra via RevDial, do the upgrade
// dance, then hand off to proxy.NewResilientProxy for byte pumping.
func (apiServer *HelixAPIServer) streamAdminRunnerLogs(res http.ResponseWriter, req *http.Request) {
	runnerID := mux.Vars(req)["runner_id"]
	if runnerID == "" {
		http.Error(res, "runner_id required", http.StatusBadRequest)
		return
	}

	// Forward tail and follow query params through to hydra. Unknown params
	// are dropped to keep the surface area small.
	hydraQuery := url.Values{}
	if v := req.URL.Query().Get("tail"); v != "" {
		hydraQuery.Set("tail", v)
	}
	if v := req.URL.Query().Get("follow"); v != "" {
		hydraQuery.Set("follow", v)
	}
	hydraPath := "/api/v1/logs"
	if encoded := hydraQuery.Encode(); encoded != "" {
		hydraPath = hydraPath + "?" + encoded
	}

	hydraRunnerID := "hydra-" + runnerID
	wsKey := req.Header.Get("Sec-WebSocket-Key")
	if wsKey == "" {
		http.Error(res, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	hijacker, ok := res.(http.Hijacker)
	if !ok {
		http.Error(res, "server does not support connection hijacking", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		log.Error().Err(err).Msg("admin runner logs: hijack failed")
		http.Error(res, "failed to hijack connection", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	dialCtx, dialCancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer dialCancel()

	serverConn, err := apiServer.connman.Dial(dialCtx, hydraRunnerID)
	if err != nil {
		log.Warn().
			Err(err).
			Str("runner_id", runnerID).
			Msg("admin runner logs: failed to dial runner via RevDial")
		_, _ = clientConn.Write([]byte("HTTP/1.1 503 Service Unavailable\r\nContent-Type: text/plain\r\n\r\nRunner not connected"))
		return
	}
	defer serverConn.Close()

	// Send WebSocket upgrade request to hydra.
	upgradeReq := fmt.Sprintf("GET %s HTTP/1.1\r\n"+
		"Host: localhost:9876\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\n"+
		"Sec-WebSocket-Version: 13\r\n"+
		"\r\n", hydraPath, wsKey)
	if _, err := serverConn.Write([]byte(upgradeReq)); err != nil {
		log.Warn().Err(err).Msg("admin runner logs: failed to send upgrade to hydra")
		_, _ = clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain\r\n\r\nHydra upgrade write failed"))
		return
	}

	serverReader := bufio.NewReader(serverConn)
	upgradeResp, err := http.ReadResponse(serverReader, nil)
	if err != nil {
		log.Warn().Err(err).Msg("admin runner logs: failed to read upgrade response from hydra")
		_, _ = clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain\r\n\r\nHydra upgrade response read failed"))
		return
	}
	defer upgradeResp.Body.Close()

	if upgradeResp.StatusCode != http.StatusSwitchingProtocols {
		log.Warn().
			Int("status", upgradeResp.StatusCode).
			Str("runner_id", runnerID).
			Msg("admin runner logs: hydra refused WebSocket upgrade")
		_, _ = clientConn.Write([]byte(fmt.Sprintf("HTTP/1.1 %d %s\r\nContent-Type: text/plain\r\n\r\nHydra refused upgrade", upgradeResp.StatusCode, upgradeResp.Status)))
		return
	}

	// Forward the 101 to the client. We pass hydra's Sec-WebSocket-Accept
	// through verbatim — hydra computed it from the same wsKey we sent
	// (which is the client's wsKey), so the value the client expects matches.
	clientAccept := upgradeResp.Header.Get("Sec-WebSocket-Accept")
	clientUpgradeResp := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Accept: %s\r\n"+
		"\r\n", clientAccept)
	if _, err := clientConn.Write([]byte(clientUpgradeResp)); err != nil {
		log.Warn().Err(err).Msg("admin runner logs: failed to forward 101 to client")
		return
	}

	log.Info().
		Str("runner_id", runnerID).
		Str("hydra_path", hydraPath).
		Msg("admin runner logs: WebSocket established, starting proxy")

	// Pump bytes via the resilient proxy. On RevDial reconnect, the same
	// upgrade dance is replayed against the new server connection.
	dialFunc := func(ctx context.Context) (net.Conn, error) {
		return apiServer.connman.Dial(ctx, hydraRunnerID)
	}
	upgradeFunc := proxy.CreateWebSocketUpgradeFunc(hydraPath, wsKey)

	rp := proxy.NewResilientProxy(proxy.ResilientProxyConfig{
		SessionID:   "admin-runner-logs-" + runnerID,
		ClientConn:  clientConn,
		ServerConn:  serverConn,
		DialFunc:    dialFunc,
		UpgradeFunc: upgradeFunc,
	})
	defer rp.Close()
	if err := rp.Run(req.Context()); err != nil {
		log.Debug().Err(err).Str("runner_id", runnerID).Msg("admin runner logs: proxy exited with error")
	}

	log.Info().
		Str("runner_id", runnerID).
		Msg("admin runner logs: WebSocket connection closed")
}
