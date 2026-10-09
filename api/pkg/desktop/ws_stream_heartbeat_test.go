//go:build cgo && linux

package desktop

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// The client tears down a socket after 10s without a message. While the
// pipeline is starting the server must keep talking — a StreamStatus every
// videoStartingInterval — and stop once video flows.
func TestHeartbeatReportsVideoStartingUntilFirstFrame(t *testing.T) {
	serverConn := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		serverConn <- c
		<-r.Context().Done()
	}))
	defer srv.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ws := <-serverConn
	defer ws.Close()

	v := NewVideoStreamer(48, 0, 0, StreamConfig{}, ws, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go v.heartbeat(ctx)

	readStatus := func(within time.Duration) (string, bool) {
		t.Helper()
		_ = client.SetReadDeadline(time.Now().Add(within))
		for {
			mt, msg, err := client.ReadMessage()
			if err != nil {
				return "", false
			}
			if mt != websocket.TextMessage {
				continue
			}
			var m streamStatusMsg
			if json.Unmarshal(msg, &m) == nil && m.StreamStatus.State != "" {
				return m.StreamStatus.State, true
			}
		}
	}

	// One immediately, and another within the interval: well inside the
	// client's 10s stale window.
	for i := 0; i < 2; i++ {
		state, ok := readStatus(videoStartingInterval + time.Second)
		if !ok || state != "starting_video" {
			t.Fatalf("status #%d = %q (ok=%v), want starting_video", i, state, ok)
		}
	}

	v.videoFlowing.Store(true)
	// At most one already-due status may still be in flight; after that, none.
	// (A gorilla read that times out poisons the conn, so count in one pass.)
	statuses := 0
	_ = client.SetReadDeadline(time.Now().Add(3 * videoStartingInterval))
	for {
		mt, msg, err := client.ReadMessage()
		if err != nil {
			break
		}
		if mt == websocket.TextMessage && strings.Contains(string(msg), "StreamStatus") {
			statuses++
		}
	}
	if statuses > 1 {
		t.Fatalf("got %d statuses after video started flowing, want at most 1", statuses)
	}
}
