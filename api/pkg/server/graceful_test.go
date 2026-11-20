package server

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
)

// TestDrainClosesHijackedConnectionsAndFinishesRequests is the SIGTERM path
// of a blue/green deploy: websockets/tunnels must be cut at once so their
// clients reconnect to the new colour, while an ordinary request in flight
// still completes.
func TestDrainClosesHijackedConnectionsAndFinishesRequests(t *testing.T) {
	d := newDrainer()
	slowStarted := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) {
		close(slowStarted)
		time.Sleep(500 * time.Millisecond)
		_, _ = io.WriteString(w, "done")
	})
	mux.HandleFunc("/upgrade", func(w http.ResponseWriter, _ *http.Request) {
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: upgrade\r\nUpgrade: test\r\n\r\n")
		_ = rw.Flush()
		// Hold the connection like a websocket handler until it is closed.
		_, _ = io.Copy(io.Discard, conn)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	served := make(chan error, 1)
	go func() { served <- d.serve(srv, ln, false) }()
	addr := ln.Addr().String()

	upgraded, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer func() { _ = upgraded.Close() }()
	_, err = fmt.Fprintf(upgraded, "GET /upgrade HTTP/1.1\r\nHost: x\r\n\r\n")
	require.NoError(t, err)
	status, err := bufio.NewReader(upgraded).ReadString('\n')
	require.NoError(t, err)
	require.Contains(t, status, "101")

	slow := make(chan string, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/slow")
		if err != nil {
			slow <- "error: " + err.Error()
			return
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		slow <- string(body)
	}()
	<-slowStarted

	drained := make(chan struct{})
	go func() { d.drain(); close(drained) }()

	// The hijacked connection is closed straight away, well before the
	// in-flight request (500ms) lets Shutdown return.
	require.NoError(t, upgraded.SetReadDeadline(time.Now().Add(200*time.Millisecond)))
	_, err = upgraded.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF, "hijacked connection should be closed by the drain")

	// The in-flight request still finishes, then the server stops.
	require.Equal(t, "done", <-slow)
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("drain did not return after the in-flight request finished")
	}
	require.NoError(t, <-served)
	_, err = net.DialTimeout("tcp", addr, time.Second)
	require.Error(t, err, "listener should be closed after the drain")
}

func TestReadyzReportsDraining(t *testing.T) {
	ctrl, s := newReadyzServer(t)
	defer ctrl.Finish()

	rec := httptest.NewRecorder()
	s.readyz(rec, httptest.NewRequest(http.MethodGet, "/api/v1/readyz", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "blue", rec.Header().Get("X-Helix-Instance"))

	s.drainer.draining.Store(true)
	rec = httptest.NewRecorder()
	s.readyz(rec, httptest.NewRequest(http.MethodGet, "/api/v1/readyz", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func newReadyzServer(t *testing.T) (*gomock.Controller, *HelixAPIServer) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().Ping(gomock.Any()).Return(nil).AnyTimes()
	cfg := &config.ServerConfig{}
	cfg.InstanceID = "blue"
	return ctrl, &HelixAPIServer{Cfg: cfg, Store: st, drainer: newDrainer()}
}
