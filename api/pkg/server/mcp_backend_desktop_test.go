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
// It also records the request path so tests can verify which path the proxy used.
func (s *DesktopMCPBackendSuite) startFakeDesktopBridge(responseBody string) *string {
	clientConn, serverConn := net.Pipe()
	s.dialer.conn = clientConn
	var requestPath string

	go func() {
		defer serverConn.Close()
		// Read the forwarded HTTP request
		req, err := http.ReadRequest(bufio.NewReader(serverConn))
		if err != nil {
			return
		}
		requestPath = req.URL.Path

		// Write an HTTP response
		resp := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s",
			len(responseBody), responseBody)
		serverConn.Write([]byte(resp))
	}()
	return &requestPath
}

// startFakeDesktopBridgeWithRoutes simulates the real desktop-bridge HTTP
// server on port 9876 which serves specific routes (screenshot, health,
// mcp, etc). Requests to unknown paths return 404.
func (s *DesktopMCPBackendSuite) startFakeDesktopBridgeWithRoutes() *string {
	clientConn, serverConn := net.Pipe()
	s.dialer.conn = clientConn
	var requestPath string

	go func() {
		defer serverConn.Close()
		req, err := http.ReadRequest(bufio.NewReader(serverConn))
		if err != nil {
			return
		}
		requestPath = req.URL.Path

		// The real desktop server (port 9876) serves /screenshot, /health,
		// /clipboard, /mcp etc. If the proxy sends to the wrong path, it 404s.
		knownRoutes := map[string]bool{
			"/screenshot": true,
			"/health":     true,
			"/clipboard":  true,
			"/mcp":        true,
		}

		if !knownRoutes[req.URL.Path] {
			resp := "HTTP/1.1 404 Not Found\r\nContent-Length: 9\r\n\r\nnot found"
			serverConn.Write([]byte(resp))
			return
		}

		body := `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`
		resp := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s",
			len(body), body)
		serverConn.Write([]byte(resp))
	}()
	return &requestPath
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

// TestServeHTTP_ProxiesToCorrectPath verifies the proxy sends to /mcp on the
// desktop HTTP server (port 9876, reached via RevDial).
func (s *DesktopMCPBackendSuite) TestServeHTTP_ProxiesToCorrectPath() {
	session := &types.Session{
		ID:    "ses-123",
		Owner: "user-1",
	}

	s.mockStore.EXPECT().
		GetSession(gomock.Any(), "ses-123").
		Return(session, nil)

	// Simulate the real desktop-bridge: known routes return 200, unknown 404.
	requestPath := s.startFakeDesktopBridgeWithRoutes()

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/desktop?session_id=ses-123", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	user := &types.User{ID: "user-1"}
	s.backend.ServeHTTP(w, req, user)

	// The proxy must target /mcp (served by desktop HTTP server on port 9876)
	s.Equal("/mcp", *requestPath, "proxy should target /mcp on port 9876, not some other path")
	s.Equal(http.StatusOK, w.Code, "should get 200 from desktop server's /mcp route")
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
