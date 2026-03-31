package server

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

var termUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleTerminal proxies a WebSocket PTY connection to the sandbox container.
// The client connects via WebSocket, and this handler bridges to the
// desktop-bridge's /pty endpoint through RevDial.
//
// GET /api/v1/sessions/{id}/terminal?cols=80&rows=24
func (apiServer *HelixAPIServer) handleTerminal(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	sessionID := vars["id"]

	// Verify session ownership
	session, err := apiServer.Store.GetSession(r.Context(), sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	if err := apiServer.authorizeUserToSession(r.Context(), user, session, types.ActionGet); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Upgrade client connection to WebSocket
	clientConn, err := termUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("terminal: websocket upgrade failed")
		return
	}
	defer clientConn.Close()

	// Connect to desktop container via RevDial
	runnerID := fmt.Sprintf("desktop-%s", sessionID)
	revDialConn, err := apiServer.connman.Dial(r.Context(), runnerID)
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("terminal: revdial connect failed")
		clientConn.WriteMessage(websocket.TextMessage, []byte(`{"error":"sandbox not connected"}`))
		return
	}
	defer revDialConn.Close()

	// Send WebSocket upgrade request to desktop-bridge's /pty endpoint
	cols := r.URL.Query().Get("cols")
	rows := r.URL.Query().Get("rows")
	if cols == "" {
		cols = "80"
	}
	if rows == "" {
		rows = "24"
	}

	upgradeReq := fmt.Sprintf("GET /pty?cols=%s&rows=%s HTTP/1.1\r\n"+
		"Host: localhost\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"+
		"Sec-WebSocket-Version: 13\r\n"+
		"\r\n", cols, rows)

	if _, err := revDialConn.Write([]byte(upgradeReq)); err != nil {
		log.Error().Err(err).Msg("terminal: failed to send upgrade to bridge")
		clientConn.WriteMessage(websocket.TextMessage, []byte(`{"error":"failed to connect to shell"}`))
		return
	}

	// Read the upgrade response
	resp, err := http.ReadResponse(bufio.NewReader(revDialConn), nil)
	if err != nil || resp.StatusCode != http.StatusSwitchingProtocols {
		log.Error().Err(err).Msg("terminal: bridge did not upgrade to websocket")
		clientConn.WriteMessage(websocket.TextMessage, []byte(`{"error":"shell upgrade failed"}`))
		return
	}

	log.Info().Str("session_id", sessionID).Msg("terminal: PTY session established")

	// Bidirectional proxy between client WebSocket and RevDial (raw TCP after upgrade)
	var wg sync.WaitGroup

	// Client → Bridge (RevDial)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			_, data, err := clientConn.ReadMessage()
			if err != nil {
				return
			}
			if _, err := revDialConn.Write(data); err != nil {
				return
			}
		}
	}()

	// Bridge (RevDial) → Client
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := revDialConn.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Debug().Err(err).Msg("terminal: bridge read error")
				}
				return
			}
			if err := clientConn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
	}()

	wg.Wait()
	log.Info().Str("session_id", sessionID).Msg("terminal: PTY session ended")
}
