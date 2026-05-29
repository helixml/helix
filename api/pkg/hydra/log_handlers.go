package hydra

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// logsUpgrader is the websocket upgrader for the /logs endpoint. Reachable
// only via RevDial from the Helix API server, so origin checks are not
// meaningful here — auth is enforced one hop up.
var logsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 16384,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

const (
	logsDefaultTail = 200
	logsMaxTail     = 5000
	logsWriteWait   = 10 * time.Second
	logsPingPeriod  = 30 * time.Second
)

// handleLogs streams hydra-aggregated runner logs (including streamed inner
// container output) over a WebSocket. Query params:
//
//	tail    int   trailing line count to emit on connect (default 200, max 5000)
//	follow  bool  stay subscribed after the tail (default true)
//
// Each emitted message is a JSON-encoded LogLine.
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if s.logBuffer == nil {
		http.Error(w, "log buffer not configured", http.StatusServiceUnavailable)
		return
	}

	tail := logsDefaultTail
	if v := r.URL.Query().Get("tail"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			tail = parsed
		}
	}
	if tail < 0 {
		tail = 0
	}
	if tail > logsMaxTail {
		tail = logsMaxTail
	}

	follow := true
	if v := r.URL.Query().Get("follow"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			follow = parsed
		}
	}

	conn, err := logsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Warn().Err(err).Msg("logs ws: upgrade failed")
		return
	}
	defer conn.Close()

	// Atomically snapshot the trailing N lines and subscribe to live lines.
	// SnapshotAndSubscribe guarantees no lines are missed between the two
	// (a plain Snapshot then Subscribe has a race window).
	snapshot, sub, cancel := s.logBuffer.SnapshotAndSubscribe(tail)
	defer cancel()

	for _, line := range snapshot {
		if err := writeLogLine(conn, line); err != nil {
			return
		}
	}

	if !follow {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "tail complete"),
			time.Now().Add(logsWriteWait),
		)
		return
	}

	// Background goroutine to detect client disconnect and stop us.
	clientGone := make(chan struct{})
	go func() {
		defer close(clientGone)
		conn.SetReadLimit(512)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	pingTicker := time.NewTicker(logsPingPeriod)
	defer pingTicker.Stop()

	for {
		select {
		case <-clientGone:
			return
		case line, ok := <-sub:
			if !ok {
				return
			}
			if err := writeLogLine(conn, line); err != nil {
				return
			}
		case <-pingTicker.C:
			if err := conn.WriteControl(
				websocket.PingMessage,
				nil,
				time.Now().Add(logsWriteWait),
			); err != nil {
				return
			}
		}
	}
}

func writeLogLine(conn *websocket.Conn, line LogLine) error {
	payload, err := json.Marshal(line)
	if err != nil {
		// Shouldn't happen for our shape, but don't kill the stream.
		return nil
	}
	if err := conn.SetWriteDeadline(time.Now().Add(logsWriteWait)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, payload)
}
