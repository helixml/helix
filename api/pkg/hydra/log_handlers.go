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
	logsPongWait    = 90 * time.Second
)

// handleLogs streams hydra-aggregated runner logs (including streamed inner
// container output) over a WebSocket. Query params:
//
//	tail    int   trailing line count to emit on connect.
//	              default 200, max 5000, 0 means "no history, live tail only",
//	              negatives are clamped to 0.
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

	// Read deadline + pong handler: a half-open TCP (laptop sleep, NAT
	// timeout) is otherwise only detected when our next ping write fails,
	// which can be 30+ seconds. With ReadDeadline + a pong-bumps-deadline
	// pattern, the read loop unblocks with an error after logsPongWait and
	// the subscriber tears down cleanly. SetPongHandler returns nil to
	// signal "ping/pong handled, bump the deadline."
	_ = conn.SetReadDeadline(time.Now().Add(logsPongWait))
	conn.SetReadLimit(512)
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(logsPongWait))
	})

	// Subscribe BEFORE the snapshot so the live-tail buffer is live during
	// snapshot delivery — if the snapshot send is slow (large tail, slow
	// client), incoming lines accumulate in the per-subscriber channel
	// rather than being lost. SnapshotAndSubscribe holds the buffer's lock
	// across both, so no producer race.
	snapshot, sub, cancel, err := s.logBuffer.SnapshotAndSubscribe(tail)
	if err != nil {
		// Subscriber cap exceeded. Send a clean WS close with the standard
		// "try again later" code so the client can render the cap message
		// instead of a generic disconnect.
		log.Warn().Err(err).Msg("logs ws: rejecting new subscriber, cap reached")
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseTryAgainLater, err.Error()),
			time.Now().Add(logsWriteWait),
		)
		return
	}
	defer cancel()

	// Background goroutine to detect client disconnect; runs immediately so
	// snapshot delivery to a dead client unblocks via the write-deadline
	// rather than blocking the full snapshot before we notice.
	clientGone := make(chan struct{})
	go func() {
		defer close(clientGone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	if !sendSnapshot(conn, snapshot, clientGone) {
		return
	}

	if !follow {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "tail complete"),
			time.Now().Add(logsWriteWait),
		)
		return
	}

	pingTicker := time.NewTicker(logsPingPeriod)
	defer pingTicker.Stop()

	for {
		select {
		case <-clientGone:
			return
		case <-r.Context().Done():
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

// sendSnapshot writes the snapshot to the client, bailing early if the client
// has already disconnected. Returns false if delivery failed (caller should
// stop). Without this guard, a closed-client snapshot would write each line
// up to its 10s deadline before noticing the connection is dead.
func sendSnapshot(conn *websocket.Conn, snapshot []LogLine, clientGone <-chan struct{}) bool {
	for _, line := range snapshot {
		select {
		case <-clientGone:
			return false
		default:
		}
		if err := writeLogLine(conn, line); err != nil {
			return false
		}
	}
	return true
}

func writeLogLine(conn *websocket.Conn, line LogLine) error {
	payload, err := json.Marshal(line)
	if err != nil {
		// json.Marshal cannot fail for our {time.Time, string} shape, but
		// if the contract ever drifts, surface the error so the connection
		// closes rather than silently dropping lines forever.
		return err
	}
	if err := conn.SetWriteDeadline(time.Now().Add(logsWriteWait)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, payload)
}
