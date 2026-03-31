package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// =============================================================================
// Session ETag Tests
// =============================================================================

type SessionETagSuite struct {
	suite.Suite

	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	server   *HelixAPIServer

	userID    string
	sessionID string
	now       time.Time
}

func TestSessionETagSuite(t *testing.T) {
	suite.Run(t, new(SessionETagSuite))
}

func (s *SessionETagSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)
	s.server = &HelixAPIServer{
		Cfg:                   &config.ServerConfig{},
		Store:                 s.store,
		externalAgentExecutor: s.executor,
	}
	s.userID = "user_123"
	s.sessionID = "ses_etag_test"
	s.now = time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
}

func (s *SessionETagSuite) TearDownTest() {
	s.ctrl.Finish()
}

// makeRequest creates a GET request for the session endpoint with optional If-None-Match header.
func (s *SessionETagSuite) makeRequest(ifNoneMatch string) *http.Request {
	req := httptest.NewRequest("GET", "/api/v1/sessions/"+s.sessionID, http.NoBody)
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: s.userID}))
	req = mux.SetURLVars(req, map[string]string{"id": s.sessionID})
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	return req
}

// makeSession creates a test session with sensible defaults.
func (s *SessionETagSuite) makeSession() *types.Session {
	return &types.Session{
		ID:      s.sessionID,
		Owner:   s.userID,
		Updated: s.now,
	}
}

// expectCheapQueries sets up the mock expectations for the cheap ETag path:
// GetSession, GetInteractionsSummary. Does NOT set up ListInteractions.
func (s *SessionETagSuite) expectCheapQueries(session *types.Session, count int64, maxUpdated time.Time) {
	s.store.EXPECT().GetSession(gomock.Any(), s.sessionID).Return(session, nil)
	s.store.EXPECT().GetInteractionsSummary(gomock.Any(), s.sessionID, session.GenerationID).Return(count, maxUpdated, nil)
}

// expectFullLoad sets up the mock expectation for ListInteractions (cache miss path).
func (s *SessionETagSuite) expectFullLoad(session *types.Session, interactions []*types.Interaction) {
	s.store.EXPECT().ListInteractions(gomock.Any(), &types.ListInteractionsQuery{
		SessionID:    s.sessionID,
		GenerationID: session.GenerationID,
		PerPage:      1000,
	}).Return(interactions, int64(len(interactions)), nil)
}

// --- Tests ---

func (s *SessionETagSuite) TestFirstRequest_Returns200WithETag() {
	session := s.makeSession()
	interactions := []*types.Interaction{
		{ID: "int_1", SessionID: s.sessionID, PromptMessage: "hello"},
	}

	s.expectCheapQueries(session, 1, s.now)
	s.expectFullLoad(session, interactions)

	rr := httptest.NewRecorder()
	s.server.getSession(rr, s.makeRequest(""))

	s.Equal(http.StatusOK, rr.Code)
	s.NotEmpty(rr.Header().Get("ETag"), "Response must include ETag header")
	s.Equal("application/json", rr.Header().Get("Content-Type"))

	// Body should contain the session with interactions
	var result types.Session
	err := json.Unmarshal(rr.Body.Bytes(), &result)
	s.NoError(err)
	s.Equal(s.sessionID, result.ID)
	s.Len(result.Interactions, 1)
	s.Equal("hello", result.Interactions[0].PromptMessage)
}

func (s *SessionETagSuite) TestMatchingETag_Returns304NoBody() {
	session := s.makeSession()
	maxUpdated := s.now.Add(10 * time.Second)

	// First request to get the ETag
	s.expectCheapQueries(session, 3, maxUpdated)
	s.expectFullLoad(session, []*types.Interaction{
		{ID: "int_1", SessionID: s.sessionID},
		{ID: "int_2", SessionID: s.sessionID},
		{ID: "int_3", SessionID: s.sessionID},
	})

	rr1 := httptest.NewRecorder()
	s.server.getSession(rr1, s.makeRequest(""))
	s.Equal(http.StatusOK, rr1.Code)
	etag := rr1.Header().Get("ETag")
	s.NotEmpty(etag)

	// Second request with the same ETag — should get 304
	// Only cheap queries should be called, NOT ListInteractions
	s.expectCheapQueries(session, 3, maxUpdated)
	// No expectFullLoad — ListInteractions must NOT be called

	rr2 := httptest.NewRecorder()
	s.server.getSession(rr2, s.makeRequest(etag))

	s.Equal(http.StatusNotModified, rr2.Code)
	s.Empty(rr2.Body.String(), "304 response must have no body")
	s.Equal(etag, rr2.Header().Get("ETag"), "304 response should echo the ETag")
}

func (s *SessionETagSuite) TestStaleETag_Returns200WithNewData() {
	session := s.makeSession()
	maxUpdated := s.now.Add(10 * time.Second)

	// First request
	s.expectCheapQueries(session, 1, maxUpdated)
	s.expectFullLoad(session, []*types.Interaction{
		{ID: "int_1", SessionID: s.sessionID, PromptMessage: "first"},
	})

	rr1 := httptest.NewRecorder()
	s.server.getSession(rr1, s.makeRequest(""))
	oldEtag := rr1.Header().Get("ETag")

	// Simulate new interaction added — maxUpdated and count change
	newMaxUpdated := s.now.Add(30 * time.Second)
	s.expectCheapQueries(session, 2, newMaxUpdated)
	s.expectFullLoad(session, []*types.Interaction{
		{ID: "int_1", SessionID: s.sessionID, PromptMessage: "first"},
		{ID: "int_2", SessionID: s.sessionID, PromptMessage: "second"},
	})

	rr2 := httptest.NewRecorder()
	s.server.getSession(rr2, s.makeRequest(oldEtag))

	s.Equal(http.StatusOK, rr2.Code, "Stale ETag should trigger full response")
	newEtag := rr2.Header().Get("ETag")
	s.NotEmpty(newEtag)
	s.NotEqual(oldEtag, newEtag, "ETag must change when data changes")

	var result types.Session
	s.NoError(json.Unmarshal(rr2.Body.Bytes(), &result))
	s.Len(result.Interactions, 2)
}

func (s *SessionETagSuite) TestETagChanges_WhenSessionUpdated() {
	session1 := s.makeSession()
	session1.Updated = s.now

	s.expectCheapQueries(session1, 1, s.now)
	s.expectFullLoad(session1, []*types.Interaction{{ID: "int_1", SessionID: s.sessionID}})

	rr1 := httptest.NewRecorder()
	s.server.getSession(rr1, s.makeRequest(""))
	etag1 := rr1.Header().Get("ETag")

	// Session metadata updated (e.g., name change) — Updated timestamp changes
	session2 := s.makeSession()
	session2.Updated = s.now.Add(5 * time.Second)

	s.expectCheapQueries(session2, 1, s.now)
	s.expectFullLoad(session2, []*types.Interaction{{ID: "int_1", SessionID: s.sessionID}})

	rr2 := httptest.NewRecorder()
	s.server.getSession(rr2, s.makeRequest(""))
	etag2 := rr2.Header().Get("ETag")

	s.NotEqual(etag1, etag2, "ETag must change when session.Updated changes")
}

func (s *SessionETagSuite) TestETagChanges_WhenInteractionCountChanges() {
	session := s.makeSession()

	s.expectCheapQueries(session, 1, s.now)
	s.expectFullLoad(session, []*types.Interaction{{ID: "int_1", SessionID: s.sessionID}})

	rr1 := httptest.NewRecorder()
	s.server.getSession(rr1, s.makeRequest(""))
	etag1 := rr1.Header().Get("ETag")

	// Same timestamps, but count increased
	s.expectCheapQueries(session, 2, s.now)
	s.expectFullLoad(session, []*types.Interaction{
		{ID: "int_1", SessionID: s.sessionID},
		{ID: "int_2", SessionID: s.sessionID},
	})

	rr2 := httptest.NewRecorder()
	s.server.getSession(rr2, s.makeRequest(""))
	etag2 := rr2.Header().Get("ETag")

	s.NotEqual(etag1, etag2, "ETag must change when interaction count changes")
}

func (s *SessionETagSuite) TestETagChanges_WhenInteractionUpdated() {
	session := s.makeSession()

	s.expectCheapQueries(session, 1, s.now)
	s.expectFullLoad(session, []*types.Interaction{{ID: "int_1", SessionID: s.sessionID}})

	rr1 := httptest.NewRecorder()
	s.server.getSession(rr1, s.makeRequest(""))
	etag1 := rr1.Header().Get("ETag")

	// Same count, but max updated changed (interaction was modified)
	s.expectCheapQueries(session, 1, s.now.Add(time.Minute))
	s.expectFullLoad(session, []*types.Interaction{{ID: "int_1", SessionID: s.sessionID, ResponseMessage: "updated"}})

	rr2 := httptest.NewRecorder()
	s.server.getSession(rr2, s.makeRequest(""))
	etag2 := rr2.Header().Get("ETag")

	s.NotEqual(etag1, etag2, "ETag must change when interaction updated timestamp changes")
}

func (s *SessionETagSuite) TestETagChanges_WhenExternalAgentStatusChanges() {
	session := s.makeSession()
	session.Metadata.ContainerName = "container_123"

	// First: agent is running
	s.executor.EXPECT().GetSession(s.sessionID).Return(&external_agent.ZedSession{}, nil)
	s.expectCheapQueries(session, 1, s.now)
	s.expectFullLoad(session, []*types.Interaction{{ID: "int_1", SessionID: s.sessionID}})

	rr1 := httptest.NewRecorder()
	s.server.getSession(rr1, s.makeRequest(""))
	etag1 := rr1.Header().Get("ETag")

	// Second: agent is stopped (GetSession returns error)
	s.executor.EXPECT().GetSession(s.sessionID).Return(nil, fmt.Errorf("not found"))
	s.expectCheapQueries(session, 1, s.now)
	s.expectFullLoad(session, []*types.Interaction{{ID: "int_1", SessionID: s.sessionID}})

	rr2 := httptest.NewRecorder()
	s.server.getSession(rr2, s.makeRequest(""))
	etag2 := rr2.Header().Get("ETag")

	s.NotEqual(etag1, etag2, "ETag must change when external agent status changes")
}

func (s *SessionETagSuite) TestETagStable_WhenNothingChanges() {
	session := s.makeSession()

	// Two identical requests with no changes
	for i := 0; i < 2; i++ {
		s.expectCheapQueries(session, 2, s.now)
		s.expectFullLoad(session, []*types.Interaction{
			{ID: "int_1", SessionID: s.sessionID},
			{ID: "int_2", SessionID: s.sessionID},
		})
	}

	rr1 := httptest.NewRecorder()
	s.server.getSession(rr1, s.makeRequest(""))
	etag1 := rr1.Header().Get("ETag")

	rr2 := httptest.NewRecorder()
	s.server.getSession(rr2, s.makeRequest(""))
	etag2 := rr2.Header().Get("ETag")

	s.Equal(etag1, etag2, "ETag must be stable when nothing changes")
}

func (s *SessionETagSuite) TestNoContainerName_AgentStatusEmpty() {
	// Session without ContainerName — executor should NOT be called
	session := s.makeSession()
	session.Metadata.ContainerName = "" // no container

	s.expectCheapQueries(session, 0, time.Time{})
	s.expectFullLoad(session, []*types.Interaction{})
	// No executor expectations — it must not be called

	rr := httptest.NewRecorder()
	s.server.getSession(rr, s.makeRequest(""))

	s.Equal(http.StatusOK, rr.Code)

	var result types.Session
	s.NoError(json.Unmarshal(rr.Body.Bytes(), &result))
	s.Empty(result.Metadata.ExternalAgentStatus, "Status should be empty when no container")
}

func (s *SessionETagSuite) TestExternalAgentRunning_SetsStatusInResponse() {
	session := s.makeSession()
	session.Metadata.ContainerName = "container_123"

	s.executor.EXPECT().GetSession(s.sessionID).Return(&external_agent.ZedSession{}, nil)
	s.expectCheapQueries(session, 0, time.Time{})
	s.expectFullLoad(session, []*types.Interaction{})

	rr := httptest.NewRecorder()
	s.server.getSession(rr, s.makeRequest(""))

	var result types.Session
	s.NoError(json.Unmarshal(rr.Body.Bytes(), &result))
	s.Equal("running", result.Metadata.ExternalAgentStatus)
}

func (s *SessionETagSuite) TestExternalAgentStopped_SetsStatusInResponse() {
	session := s.makeSession()
	session.Metadata.ContainerName = "container_123"

	s.executor.EXPECT().GetSession(s.sessionID).Return(nil, fmt.Errorf("not running"))
	s.expectCheapQueries(session, 0, time.Time{})
	s.expectFullLoad(session, []*types.Interaction{})

	rr := httptest.NewRecorder()
	s.server.getSession(rr, s.makeRequest(""))

	var result types.Session
	s.NoError(json.Unmarshal(rr.Body.Bytes(), &result))
	s.Equal("stopped", result.Metadata.ExternalAgentStatus)
}

func (s *SessionETagSuite) TestCacheHit_DoesNotLoadInteractions() {
	session := s.makeSession()

	// First request — loads everything
	s.expectCheapQueries(session, 1, s.now)
	s.expectFullLoad(session, []*types.Interaction{{ID: "int_1", SessionID: s.sessionID}})

	rr1 := httptest.NewRecorder()
	s.server.getSession(rr1, s.makeRequest(""))
	etag := rr1.Header().Get("ETag")

	// Second request with matching ETag — ListInteractions must NOT be called
	s.expectCheapQueries(session, 1, s.now)
	// Intentionally NOT calling expectFullLoad — gomock will fail if ListInteractions is called

	rr2 := httptest.NewRecorder()
	s.server.getSession(rr2, s.makeRequest(etag))

	s.Equal(http.StatusNotModified, rr2.Code)
}

func (s *SessionETagSuite) TestWrongETag_LoadsInteractions() {
	session := s.makeSession()

	s.expectCheapQueries(session, 1, s.now)
	s.expectFullLoad(session, []*types.Interaction{{ID: "int_1", SessionID: s.sessionID}})

	rr := httptest.NewRecorder()
	s.server.getSession(rr, s.makeRequest(`"stale-etag-value"`))

	s.Equal(http.StatusOK, rr.Code, "Wrong ETag must trigger full response")
	s.NotEmpty(rr.Body.String())
}

func (s *SessionETagSuite) TestSessionNotFound_Returns500() {
	s.store.EXPECT().GetSession(gomock.Any(), s.sessionID).Return(nil, fmt.Errorf("not found"))

	rr := httptest.NewRecorder()
	s.server.getSession(rr, s.makeRequest(""))

	s.Equal(http.StatusInternalServerError, rr.Code)
}

func (s *SessionETagSuite) TestUnauthorized_Returns403() {
	session := s.makeSession()
	session.Owner = "other_user"           // Different owner
	session.OrganizationID = "org_private" // In an org

	s.store.EXPECT().GetSession(gomock.Any(), s.sessionID).Return(session, nil)
	// Auth check will look up org membership and fail
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("not a member"))

	rr := httptest.NewRecorder()
	s.server.getSession(rr, s.makeRequest(""))

	s.Equal(http.StatusForbidden, rr.Code)
}

func (s *SessionETagSuite) TestETagChanges_WhenGenerationIDChanges() {
	session1 := s.makeSession()
	session1.GenerationID = 1

	s.store.EXPECT().GetSession(gomock.Any(), s.sessionID).Return(session1, nil)
	s.store.EXPECT().GetInteractionsSummary(gomock.Any(), s.sessionID, 1).Return(int64(1), s.now, nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), &types.ListInteractionsQuery{
		SessionID:    s.sessionID,
		GenerationID: 1,
		PerPage:      1000,
	}).Return([]*types.Interaction{{ID: "int_1", SessionID: s.sessionID}}, int64(1), nil)

	rr1 := httptest.NewRecorder()
	s.server.getSession(rr1, s.makeRequest(""))
	etag1 := rr1.Header().Get("ETag")

	// Regeneration — GenerationID bumped
	session2 := s.makeSession()
	session2.GenerationID = 2

	s.store.EXPECT().GetSession(gomock.Any(), s.sessionID).Return(session2, nil)
	s.store.EXPECT().GetInteractionsSummary(gomock.Any(), s.sessionID, 2).Return(int64(1), s.now, nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), &types.ListInteractionsQuery{
		SessionID:    s.sessionID,
		GenerationID: 2,
		PerPage:      1000,
	}).Return([]*types.Interaction{{ID: "int_1", SessionID: s.sessionID}}, int64(1), nil)

	rr2 := httptest.NewRecorder()
	s.server.getSession(rr2, s.makeRequest(""))
	etag2 := rr2.Header().Get("ETag")

	s.NotEqual(etag1, etag2, "ETag must change when GenerationID changes")
}
