package server

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// DesktopMCPBackendSuite tests the Desktop MCP backend
type DesktopMCPBackendSuite struct {
	suite.Suite
	ctx       context.Context
	ctrl      *gomock.Controller
	mockStore *store.MockStore
	dialer    *fakeDialer
	backend   *DesktopMCPBackend
}

func TestDesktopMCPBackendSuite(t *testing.T) {
	suite.Run(t, new(DesktopMCPBackendSuite))
}

func (s *DesktopMCPBackendSuite) SetupTest() {
	s.ctx = context.Background()
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = store.NewMockStore(s.ctrl)
	s.dialer = &fakeDialer{}
	s.backend = NewDesktopMCPBackend(s.mockStore, s.dialer)
}

func (s *DesktopMCPBackendSuite) TearDownTest() {
	s.ctrl.Finish()
}

// =============================================================================
// Fake Dialer — implements SandboxDialer using net.Pipe
// =============================================================================

type fakeDialer struct {
	// conn is the server-side of the pipe, set by test to simulate desktop-bridge
	conn net.Conn
	err  error
}

func (d *fakeDialer) Dial(_ context.Context, _ string) (net.Conn, error) {
	if d.err != nil {
		return nil, d.err
	}
	return d.conn, nil
}

// startFakeDesktopBridge starts a goroutine that reads an HTTP request from
// the server-side pipe and writes back a canned HTTP response.
func (s *DesktopMCPBackendSuite) startFakeDesktopBridge(responseBody string) {
	clientConn, serverConn := net.Pipe()
	s.dialer.conn = clientConn

	go func() {
		defer serverConn.Close()
		// Read the forwarded HTTP request
		_, err := http.ReadRequest(bufio.NewReader(serverConn))
		if err != nil {
			return
		}

		// Write an HTTP response
		resp := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s",
			len(responseBody), responseBody)
		serverConn.Write([]byte(resp))
	}()
}

// =============================================================================
// Tests
// =============================================================================

func (s *DesktopMCPBackendSuite) TestServeHTTP_ProxiesToDesktopBridge() {
	session := &types.Session{
		ID:    "ses-123",
		Owner: "user-1",
	}

	s.mockStore.EXPECT().
		GetSession(gomock.Any(), "ses-123").
		Return(session, nil)

	mcpResponse := `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`
	s.startFakeDesktopBridge(mcpResponse)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/desktop?session_id=ses-123", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	user := &types.User{ID: "user-1"}
	s.backend.ServeHTTP(w, req, user)

	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Body.String(), `"tools":[]`)
}

func (s *DesktopMCPBackendSuite) TestServeHTTP_MissingSessionID() {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/desktop", strings.NewReader("{}"))
	w := httptest.NewRecorder()

	user := &types.User{ID: "user-1"}
	s.backend.ServeHTTP(w, req, user)

	s.Equal(http.StatusBadRequest, w.Code)
	s.Contains(w.Body.String(), "session_id")
}

func (s *DesktopMCPBackendSuite) TestServeHTTP_SessionNotFound() {
	s.mockStore.EXPECT().
		GetSession(gomock.Any(), "ses-missing").
		Return(nil, fmt.Errorf("not found"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/desktop?session_id=ses-missing", strings.NewReader("{}"))
	w := httptest.NewRecorder()

	user := &types.User{ID: "user-1"}
	s.backend.ServeHTTP(w, req, user)

	s.Equal(http.StatusNotFound, w.Code)
}

func (s *DesktopMCPBackendSuite) TestServeHTTP_UserDoesNotOwnSession() {
	session := &types.Session{
		ID:    "ses-123",
		Owner: "user-other",
	}

	s.mockStore.EXPECT().
		GetSession(gomock.Any(), "ses-123").
		Return(session, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/desktop?session_id=ses-123", strings.NewReader("{}"))
	w := httptest.NewRecorder()

	user := &types.User{ID: "user-1"}
	s.backend.ServeHTTP(w, req, user)

	s.Equal(http.StatusForbidden, w.Code)
}

func (s *DesktopMCPBackendSuite) TestServeHTTP_SandboxNotConnected() {
	session := &types.Session{
		ID:    "ses-123",
		Owner: "user-1",
	}

	s.mockStore.EXPECT().
		GetSession(gomock.Any(), "ses-123").
		Return(session, nil)

	s.dialer.err = fmt.Errorf("no connection")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/desktop?session_id=ses-123", strings.NewReader("{}"))
	w := httptest.NewRecorder()

	user := &types.User{ID: "user-1"}
	s.backend.ServeHTTP(w, req, user)

	s.Equal(http.StatusServiceUnavailable, w.Code)
	s.Contains(w.Body.String(), "not connected")
}
