package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
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

func TestRevDialKeyMayRegisterDesktop(t *testing.T) {
	for _, tc := range []struct {
		name string
		user types.User
		ok   bool
	}{
		{"session key for this session", types.User{APIKeyType: types.APIkeytypeAPI, SessionID: "ses_A"}, true},
		{"bot instance key for this session", types.User{APIKeyType: types.APIkeytypeBotInstance, SessionID: "ses_A"}, true},
		{"owner key without a session", types.User{APIKeyType: types.APIkeytypeAPI}, true},
		{"session key for another session", types.User{APIKeyType: types.APIkeytypeAPI, SessionID: "ses_B"}, false},
		{"app key", types.User{APIKeyType: types.APIkeytypeApp}, false},
		{"embed key", types.User{APIKeyType: types.APIkeytypeEmbed, SessionID: "ses_A"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := revDialKeyMayRegisterDesktop(&tc.user, "ses_A")
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want allowed=%v", err, tc.ok)
			}
		})
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

// revDialControl is a test client's control connection. closed is closed
// when the server ends the connection.
type revDialControl struct {
	*websocket.Conn
	closed chan struct{}
}

func dialRevDialControl(t *testing.T, srv *httptest.Server, runnerID string) (*revDialControl, int) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/revdial?runnerid=" + runnerID
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		if resp == nil {
			t.Fatalf("dial %s: %v", runnerID, err)
		}
		return nil, resp.StatusCode
	}
	control := &revDialControl{Conn: conn, closed: make(chan struct{})}
	// Keep reading so control frames (pings) are answered, as the real
	// listener does.
	go func() {
		defer close(control.closed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	return control, resp.StatusCode
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

func TestHandleRevDial_RefusesNonWebSocketControlConnection(t *testing.T) {
	cm := connman.New()
	t.Cleanup(cm.Stop)
	srv := revDialTestServer(t, &HelixAPIServer{connman: cm}, types.User{TokenType: types.TokenTypeRunner})

	resp, err := http.Get(srv.URL + "/api/v1/revdial?runnerid=hydra-host1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if len(cm.List()) != 0 {
		t.Fatalf("connman has registrations after a refused request: %v", cm.List())
	}
}

func TestHandleRevDial_SessionKeyCannotRegisterAnotherSessionsDesktop(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetSession(gomock.Any(), "ses_B").Return(&types.Session{ID: "ses_B", Owner: "usr_A"}, nil).AnyTimes()

	cm := connman.New()
	t.Cleanup(cm.Stop)
	srv := revDialTestServer(t, &HelixAPIServer{connman: cm, Store: st},
		types.User{ID: "usr_A", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, SessionID: "ses_A"})

	_, status := dialRevDialControl(t, srv, "desktop-ses_B")
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
	if len(cm.List()) != 0 {
		t.Fatalf("connman has registrations after a refused request: %v", cm.List())
	}
}

// A runner-token registration displaces the current tunnel for the id (last
// writer wins), so a host that reconnects always gets its id back. The
// displaced tunnel is closed so its client notices and reconnects.
func TestHandleRevDial_RunnerTokenReconnectDisplacesOldTunnel(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	// ensureSandboxRegistered: no existing host row, register a new one.
	st.EXPECT().ListSandboxInstances(gomock.Any()).Return(nil, nil).AnyTimes()
	st.EXPECT().RegisterSandboxInstance(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	cm := connman.New()
	t.Cleanup(cm.Stop)
	srv := revDialTestServer(t, &HelixAPIServer{connman: cm, Store: st, Cfg: &config.ServerConfig{}}, types.User{TokenType: types.TokenTypeRunner})

	first, status := dialRevDialControl(t, srv, "hydra-host1")
	if first == nil {
		t.Fatalf("first registration failed with status %d", status)
	}
	defer first.Close()
	waitFor(t, func() bool { return len(cm.List()) == 1 })

	second, status := dialRevDialControl(t, srv, "hydra-host1")
	if second == nil {
		t.Fatalf("reconnect failed with status %d", status)
	}
	defer second.Close()

	// The displaced tunnel is closed by the server.
	select {
	case <-first.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("displaced tunnel was not closed")
	}
	select {
	case <-second.closed:
		t.Fatal("the new tunnel was closed")
	default:
	}
	if keys := cm.List(); len(keys) != 1 || keys[0] != "hydra-host1" {
		t.Fatalf("connman keys = %v, want [hydra-host1]", keys)
	}
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
