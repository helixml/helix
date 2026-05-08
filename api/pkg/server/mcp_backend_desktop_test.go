package server

import (
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
	if s.dialer.server != nil {
		s.dialer.server.Close()
	}
}

// fakeDialer implements SandboxDialer by TCP-dialing an httptest.Server.
type fakeDialer struct {
	server *httptest.Server
	err    error
}

func (d *fakeDialer) Dial(_ context.Context, _ string) (net.Conn, error) {
	if d.err != nil {
		return nil, d.err
	}
	addr := strings.TrimPrefix(d.server.URL, "http://")
	return net.Dial("tcp", addr)
}

// session returns a test session owned by user-1.
func (s *DesktopMCPBackendSuite) session() *types.Session {
	return &types.Session{ID: "ses-123", Owner: "user-1"}
}

// mcpRequest builds a POST to the desktop MCP gateway with the given session.
func (s *DesktopMCPBackendSuite) mcpRequest(sessionID string) (*httptest.ResponseRecorder, *http.Request) {
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/desktop?session_id="+sessionID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return httptest.NewRecorder(), req
}

// =============================================================================
// Tests
// =============================================================================

func (s *DesktopMCPBackendSuite) TestProxy_ForwardsToMCPPath() {
	s.mockStore.EXPECT().GetSession(gomock.Any(), "ses-123").Return(s.session(), nil)

	var receivedPath string
	s.dialer.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`)
	}))

	w, req := s.mcpRequest("ses-123")
	s.backend.ServeHTTP(w, req, &types.User{ID: "user-1"})

	s.Equal("/mcp", receivedPath)
	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Body.String(), `"tools":[]`)
}

func (s *DesktopMCPBackendSuite) TestMissingSessionID() {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/desktop", strings.NewReader("{}"))
	w := httptest.NewRecorder()

	s.backend.ServeHTTP(w, req, &types.User{ID: "user-1"})

	s.Equal(http.StatusBadRequest, w.Code)
	s.Contains(w.Body.String(), "session_id")
}

func (s *DesktopMCPBackendSuite) TestSessionNotFound() {
	s.mockStore.EXPECT().GetSession(gomock.Any(), "ses-missing").Return(nil, fmt.Errorf("not found"))

	w, req := s.mcpRequest("ses-missing")
	s.backend.ServeHTTP(w, req, &types.User{ID: "user-1"})

	s.Equal(http.StatusNotFound, w.Code)
}

func (s *DesktopMCPBackendSuite) TestForbiddenWhenNotOwner() {
	other := &types.Session{ID: "ses-123", Owner: "user-other"}
	s.mockStore.EXPECT().GetSession(gomock.Any(), "ses-123").Return(other, nil)

	w, req := s.mcpRequest("ses-123")
	s.backend.ServeHTTP(w, req, &types.User{ID: "user-1"})

	s.Equal(http.StatusForbidden, w.Code)
}

func (s *DesktopMCPBackendSuite) TestSandboxNotConnected() {
	s.mockStore.EXPECT().GetSession(gomock.Any(), "ses-123").Return(s.session(), nil)
	s.dialer.err = fmt.Errorf("no connection")

	w, req := s.mcpRequest("ses-123")
	s.backend.ServeHTTP(w, req, &types.User{ID: "user-1"})

	s.Equal(http.StatusServiceUnavailable, w.Code)
	s.Contains(w.Body.String(), "not connected")
}
