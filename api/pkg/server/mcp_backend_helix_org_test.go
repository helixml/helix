package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// /mcp/helix-org is reachable only by worker-bearing sessions; task keys stay 403.
type HelixOrgMCPBackendSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	mockStore *store.MockStore
	backend   *HelixOrgMCPBackend
}

func TestHelixOrgMCPBackendSuite(t *testing.T) {
	suite.Run(t, new(HelixOrgMCPBackendSuite))
}

func (s *HelixOrgMCPBackendSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = store.NewMockStore(s.ctrl)
	apiServer := &HelixAPIServer{Store: s.mockStore}
	s.backend = NewHelixOrgMCPBackend(apiServer, &helixOrgHandlers{
		api: http.NotFoundHandler(),
	})
}

func (s *HelixOrgMCPBackendSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HelixOrgMCPBackendSuite) serve(user *types.User) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/helix-org", nil)
	req = mux.SetURLVars(req, map[string]string{"path": ""})
	w := httptest.NewRecorder()
	s.backend.ServeHTTP(w, req, user)
	return w
}

func apiKeyUser(sessionID string) *types.User {
	return &types.User{
		ID:             "user-1",
		TokenType:      types.TokenTypeAPIKey,
		SessionID:      sessionID,
		OrganizationID: "org-1",
	}
}

func (s *HelixOrgMCPBackendSuite) TestTaskSessionIsForbidden() {
	taskSession := &types.Session{
		ID:             "ses-task",
		Owner:          "user-1",
		OrganizationID: "org-1",
		Created:        time.Now(),
	}
	taskSession.Metadata.SpecTaskID = "spt-1"
	// OrgWorkerID deliberately unset (guardrail invariant).
	s.mockStore.EXPECT().GetSession(gomock.Any(), "ses-task").Return(taskSession, nil)
	w := s.serve(apiKeyUser("ses-task"))
	s.Equal(http.StatusForbidden, w.Code)
}

func (s *HelixOrgMCPBackendSuite) TestUserKeyIsForbidden() {
	user := apiKeyUser("ses-any")
	user.TokenType = types.TokenTypeSession
	w := s.serve(user)
	s.Equal(http.StatusForbidden, w.Code)
}

func (s *HelixOrgMCPBackendSuite) TestMismatchedOrgIsForbidden() {
	orgSession := &types.Session{
		ID:             "ses-x",
		Owner:          "user-1",
		OrganizationID: "org-2",
		Created:        time.Now(),
	}
	orgSession.Metadata.OrgWorkerID = "w-1"
	s.mockStore.EXPECT().GetSession(gomock.Any(), "ses-x").Return(orgSession, nil)
	w := s.serve(apiKeyUser("ses-x"))
	s.Equal(http.StatusForbidden, w.Code)
}
