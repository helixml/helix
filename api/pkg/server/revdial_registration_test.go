package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func TestRunnerTokenMayRegisterRevDial(t *testing.T) {
	for _, tc := range []struct {
		runnerID string
		want     bool
	}{
		{"hydra-host1", true},
		{"sandbox-host1", true},
		{"desktop-ses_A", false},
		{"desktop-sbx_A", false},
		{"hydra-", false},
		{"sandbox-", false},
		{"ses_A", false},
		{"", false},
	} {
		if got := runnerTokenMayRegisterRevDial(tc.runnerID); got != tc.want {
			t.Errorf("runnerTokenMayRegisterRevDial(%q) = %v, want %v", tc.runnerID, got, tc.want)
		}
	}
}

func TestRevDialLiveness(t *testing.T) {
	start := time.Now()
	l := newRevDialLiveness(start)
	if !l.alive(start.Add(revDialLivenessWindow - time.Second)) {
		t.Fatal("connection should be alive inside the window")
	}
	if l.alive(start.Add(revDialLivenessWindow + time.Second)) {
		t.Fatal("connection should not be alive after the window without a pong")
	}
	l.touch(start.Add(revDialLivenessWindow))
	if !l.alive(start.Add(revDialLivenessWindow + time.Second)) {
		t.Fatal("a pong should extend the window")
	}
}

// revDialTestServer serves handleRevDial with a fixed caller identity, as the
// auth middleware would.
func revDialTestServer(t *testing.T, apiServer *HelixAPIServer, user types.User) *httptest.Server {
	t.Helper()
	handler := apiServer.handleRevDial()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r.WithContext(setRequestUser(r.Context(), user)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dialRevDialControl(t *testing.T, srv *httptest.Server, runnerID string) (*websocket.Conn, int) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/revdial?runnerid=" + runnerID
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		if resp == nil {
			t.Fatalf("dial %s: %v", runnerID, err)
		}
		return nil, resp.StatusCode
	}
	// Keep reading so control frames (pings) are answered, as the real
	// listener does.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	return conn, resp.StatusCode
}

func TestHandleRevDial_RunnerTokenCannotRegisterDesktopTunnel(t *testing.T) {
	cm := connman.New()
	t.Cleanup(cm.Stop)
	apiServer := &HelixAPIServer{connman: cm}
	srv := revDialTestServer(t, apiServer, types.User{TokenType: types.TokenTypeRunner})

	_, status := dialRevDialControl(t, srv, "desktop-ses_victim")
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
	if len(cm.List()) != 0 {
		t.Fatalf("connman has registrations after a refused request: %v", cm.List())
	}
}

func TestHandleRevDial_LiveTunnelIsNotTakenOver(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetSession(gomock.Any(), "ses_A").Return(&types.Session{ID: "ses_A", Owner: "usr_A"}, nil).AnyTimes()

	cm := connman.New()
	t.Cleanup(cm.Stop)
	apiServer := &HelixAPIServer{connman: cm, Store: st}
	srv := revDialTestServer(t, apiServer, types.User{ID: "usr_A", TokenType: types.TokenTypeAPIKey, SessionID: "ses_A"})

	first, status := dialRevDialControl(t, srv, "desktop-ses_A")
	if first == nil {
		t.Fatalf("first registration failed with status %d", status)
	}
	waitFor(t, func() bool { return cm.HasLive("desktop-ses_A") })

	second, status := dialRevDialControl(t, srv, "desktop-ses_A")
	if second != nil {
		second.Close()
		t.Fatal("second registration of a live tunnel was accepted")
	}
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}

	// Once the first client goes away the id is free again.
	first.Close()
	waitFor(t, func() bool { return !cm.HasLive("desktop-ses_A") })

	third, status := dialRevDialControl(t, srv, "desktop-ses_A")
	if third == nil {
		t.Fatalf("reconnect after the old tunnel closed failed with status %d", status)
	}
	third.Close()
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 5s")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
