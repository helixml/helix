package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/proxy"
)

const (
	// logsTailMaxFromAPI mirrors hydra's max-tail clamp so we reject
	// nonsense before opening a RevDial connection. Hydra also clamps,
	// this is defence in depth.
	logsTailMaxFromAPI = 5000
)

// streamAdminRunnerLogs streams a Runner's hydra-aggregated log buffer over a
// WebSocket so an admin can live-tail what the Runner is doing without SSH.
//
// Route: GET /api/v1/admin/runners/{runner_id}/logs
// Query: tail=N (default 200, max 5000, 0 = no history), follow=bool (default true)
//
// Auth is enforced by the admin subrouter middleware. Multiple admins can open
// the same stream concurrently — hydra fans the same buffer to all
// subscribers; there is no supersede semantic here (unlike the screenshot
// stream WS which dedupes per browser tab).
//
// Pattern mirrors the screenshot stream WS proxy at
// external_agent_handlers.go: hijack, dial hydra via RevDial, do the upgrade
// dance, then hand off to proxy.NewResilientProxy for byte pumping.
//
// Caveat about the URL var `runner_id`: this matches the existing
// /admin/runners/{runner_id}/... family which treats the value as a
// sandbox_instances row ID. The same value is the RevDial registration key
// (`hydra-<id>`). Hydra serves the *aggregated* buffer for the whole runner
// process — every inner desktop container, plus hydra's own zerolog output.
// This endpoint does not filter to a single inner container.
//
// Auth scope: Runners are global infrastructure (no `OrganizationID` field
// on the SandboxInstance row), so any admin can view any Runner's logs.
// This matches the rest of the /admin/runners/... family. If Runners ever
// gain org-scoping, this handler will need to call s.lookupOrg() and check
// membership, matching the wallet_handlers.go pattern. Documented openly so
// the next reader doesn't assume the scoping that doesn't exist.
//
// Content disclosure: streamed `docker logs` output may contain whatever the
// inner desktop containers printed to stdout/stderr — including any API
// keys, OAuth tokens, or environment variable dumps that the agent or its
// children emitted. Treat this endpoint as carrying the same trust as
// reading every running container's stdout directly. "Admin" should be a
// trusted operator role.
func (apiServer *HelixAPIServer) streamAdminRunnerLogs(res http.ResponseWriter, req *http.Request) {
	runnerID := mux.Vars(req)["runner_id"]
	if runnerID == "" {
		http.Error(res, "runner_id required", http.StatusBadRequest)
		return
	}

	// Origin check (CSRF defence). The same-site WS hijack is otherwise
	// reachable by an authenticated admin's browser session from any
	// origin, which is the classic WS-CSRF shape. Allow only requests
	// whose Origin matches the server's configured public URL, or is empty
	// (curl / wscat clients, fine because they have to forge auth too).
	if origin := req.Header.Get("Origin"); origin != "" {
		serverURL := apiServer.Cfg.WebServer.URL
		if serverURL == "" || !originAllowed(origin, serverURL) {
			log.Warn().
				Str("origin", origin).
				Str("server_url", serverURL).
				Msg("admin runner logs: rejecting WS upgrade from disallowed Origin")
			http.Error(res, "forbidden origin", http.StatusForbidden)
			return
		}
	}

	// Validate + clamp `tail` before opening a RevDial connection so an
	// admin requesting an absurd value gets a clean 400, not a stalled
	// upgrade.
	hydraQuery := url.Values{}
	if v := req.URL.Query().Get("tail"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			http.Error(res, "invalid tail", http.StatusBadRequest)
			return
		}
		if n < 0 {
			n = 0
		}
		if n > logsTailMaxFromAPI {
			n = logsTailMaxFromAPI
		}
		hydraQuery.Set("tail", strconv.Itoa(n))
	}
	if v := req.URL.Query().Get("follow"); v != "" {
		// Accept only literal "true" / "false" so we don't forward
		// arbitrary user input verbatim.
		if v != "true" && v != "false" {
			http.Error(res, "invalid follow", http.StatusBadRequest)
			return
		}
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

	// Per-request session ID so two concurrent admin tabs against the same
	// runner are distinguishable in logs.
	sessionID := fmt.Sprintf("admin-runner-logs-%s-%s", runnerID, randHex(6))

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
			Str("session_id", sessionID).
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
		log.Warn().Err(err).Str("session_id", sessionID).Msg("admin runner logs: failed to send upgrade to hydra")
		_, _ = clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain\r\n\r\nHydra upgrade write failed"))
		return
	}

	// CRITICAL: bufio reader can buffer bytes past the 101 response into
	// memory. With this PR's snapshot-on-connect, hydra writes the 101
	// then immediately follows with up to 5000 snapshot WS frames; many of
	// those will land in the bufio reader's buffer during ReadResponse.
	// Once we hand serverConn to ResilientProxy below, the proxy reads
	// from the raw conn and any bytes still in the bufio reader are lost.
	// We drain `serverReader.Buffered()` to the client right after the
	// upgrade dance to avoid silent snapshot truncation.
	serverReader := bufio.NewReader(serverConn)
	upgradeResp, err := http.ReadResponse(serverReader, nil)
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("admin runner logs: failed to read upgrade response from hydra")
		_, _ = clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain\r\n\r\nHydra upgrade response read failed"))
		return
	}
	defer upgradeResp.Body.Close()

	if upgradeResp.StatusCode != http.StatusSwitchingProtocols {
		log.Warn().
			Int("status", upgradeResp.StatusCode).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
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
		log.Warn().Err(err).Str("session_id", sessionID).Msg("admin runner logs: failed to forward 101 to client")
		return
	}

	// Drain any post-101 bytes already buffered by the http.ReadResponse
	// call and forward them to the client before ResilientProxy takes
	// over. Without this, the entire initial snapshot is silently dropped.
	if buffered := serverReader.Buffered(); buffered > 0 {
		buf := make([]byte, buffered)
		if _, err := serverReader.Read(buf); err == nil {
			if _, werr := clientConn.Write(buf); werr != nil {
				log.Warn().Err(werr).Str("session_id", sessionID).Msg("admin runner logs: failed to flush buffered upgrade tail to client")
				return
			}
		}
	}

	log.Info().
		Str("runner_id", runnerID).
		Str("session_id", sessionID).
		Str("hydra_path", hydraPath).
		Msg("admin runner logs: WebSocket established, starting proxy")

	// Pump bytes via the resilient proxy. On RevDial reconnect, the
	// upgrade is replayed — but with tail=0 so the client doesn't receive
	// a duplicate snapshot every reconnect. The follow flag is preserved.
	dialFunc := func(ctx context.Context) (net.Conn, error) {
		return apiServer.connman.Dial(ctx, hydraRunnerID)
	}
	reconnectQuery := url.Values{}
	reconnectQuery.Set("tail", "0")
	if v := hydraQuery.Get("follow"); v != "" {
		reconnectQuery.Set("follow", v)
	}
	reconnectPath := "/api/v1/logs?" + reconnectQuery.Encode()
	upgradeFunc := proxy.CreateWebSocketUpgradeFunc(reconnectPath, wsKey)

	rp := proxy.NewResilientProxy(proxy.ResilientProxyConfig{
		SessionID:   sessionID,
		ClientConn:  clientConn,
		ServerConn:  serverConn,
		DialFunc:    dialFunc,
		UpgradeFunc: upgradeFunc,
	})
	defer rp.Close()
	if err := rp.Run(req.Context()); err != nil {
		log.Debug().Err(err).Str("runner_id", runnerID).Str("session_id", sessionID).Msg("admin runner logs: proxy exited with error")
	}

	log.Info().
		Str("runner_id", runnerID).
		Str("session_id", sessionID).
		Msg("admin runner logs: WebSocket connection closed")
}

// originAllowed returns true when the request Origin matches the scheme+host
// of the configured public server URL. Simple exact-match on origin string;
// callers running multiple frontends behind the same control plane should
// extend this once that requirement actually exists.
func originAllowed(origin, serverURL string) bool {
	serverParsed, err := url.Parse(serverURL)
	if err != nil || serverParsed.Host == "" {
		return false
	}
	originParsed, err := url.Parse(origin)
	if err != nil || originParsed.Host == "" {
		return false
	}
	return strings.EqualFold(originParsed.Host, serverParsed.Host) &&
		strings.EqualFold(originParsed.Scheme, serverParsed.Scheme)
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fallback to a timestamp-derived suffix; uniqueness still good
		// enough for log correlation.
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}
