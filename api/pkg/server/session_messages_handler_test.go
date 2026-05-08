package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// SessionMessagesHandlerSuite tests POST /api/v1/sessions/{id}/messages.
type SessionMessagesHandlerSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestSessionMessagesHandlerSuite(t *testing.T) {
	suite.Run(t, new(SessionMessagesHandlerSuite))
}

func (s *SessionMessagesHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)

	// TouchSession is fire-and-forget inside sendChatMessageToExternalAgent.
	s.store.EXPECT().TouchSession(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	s.server = &HelixAPIServer{
		Cfg: &config.ServerConfig{
			WebServer: config.WebServer{URL: "http://localhost:0", Host: "localhost", Port: 0, RunnerToken: "test"},
		},
		Store:  s.store,
		pubsub: pubsub.NewNoop(),
		Controller: &controller.Controller{
			Options: controller.Options{Store: s.store, PubSub: pubsub.NewNoop()},
		},
		externalAgentWSManager:      NewExternalAgentWSManager(),
		externalAgentRunnerManager:  NewExternalAgentRunnerManager(),
		contextMappings:             make(map[string]string),
		requestToSessionMapping:     make(map[string]string),
		requestToInteractionMapping: make(map[string]string),
		externalAgentSessionMapping: make(map[string]string),
		externalAgentUserMapping:    make(map[string]string),
		sessionCommentTimeout:       make(map[string]*time.Timer),
		requestToCommenterMapping:   make(map[string]string),
		sessionToCommenterMapping:   make(map[string]string),
		streamingContexts:           make(map[string]*streamingContext),
		streamingRateLimiter:        make(map[string]time.Time),
	}
}

func (s *SessionMessagesHandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

// callHandler invokes sendSessionMessage via system.Wrapper so the test
// exercises the same response shape an HTTP client would see.
func (s *SessionMessagesHandlerSuite) callHandler(sessionID string, body any, user *types.User) *httptest.ResponseRecorder {
	buf, err := json.Marshal(body)
	s.Require().NoError(err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/messages", bytes.NewReader(buf))
	req = mux.SetURLVars(req, map[string]string{"id": sessionID})
	if user != nil {
		req = req.WithContext(setRequestUser(req.Context(), *user))
	}
	rr := httptest.NewRecorder()
	system.Wrapper(s.server.sendSessionMessage)(rr, req)
	return rr
}

// TestQueuesWhenNoWS verifies that with no agent WebSocket connected,
// the handler still creates a Waiting interaction and returns 200 — the
// caller's contract is "queued, will deliver on reconnect" via
// pickupWaitingInteraction.
func (s *SessionMessagesHandlerSuite) TestQueuesWhenNoWS() {
	user := &types.User{ID: "user-1"}
	sessionID := "ses_noWs"

	session := &types.Session{ID: sessionID, Owner: user.ID, GenerationID: 0}
	// Two GetSession calls: handler authz lookup + sendChatMessageToExternalAgent.
	// AnyTimes because sendCommandToExternalAgent fires autoStartDevContainerForSession
	// in a goroutine when there's no WS connection — it does another GetSession lookup.
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).AnyTimes()

	createdInteraction := &types.Interaction{ID: "int-1", SessionID: sessionID}
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *types.Interaction) (*types.Interaction, error) {
			s.Equal(types.InteractionStateWaiting, in.State)
			s.Equal("hello", in.PromptMessage)
			return createdInteraction, nil
		},
	)

	rr := s.callHandler(sessionID, SessionMessageRequest{Content: "hello"}, user)
	s.Require().Equal(http.StatusOK, rr.Code, "body=%s", rr.Body.String())

	var resp SessionMessageResponse
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &resp))
	s.Equal("int-1", resp.InteractionID)
	s.NotEmpty(resp.RequestID)

	// request_id → interaction_id mapping is populated so callers can correlate
	// streamed responses on /api/v1/ws/user.
	s.server.contextMappingsMutex.Lock()
	s.Equal("int-1", s.server.requestToInteractionMapping[resp.RequestID])
	s.server.contextMappingsMutex.Unlock()
}

// TestRejectsCrossUser verifies authorizeUserToSession blocks a different
// owner — the session has no OrganizationID, so only the session owner passes.
func (s *SessionMessagesHandlerSuite) TestRejectsCrossUser() {
	owner := &types.User{ID: "owner-1"}
	other := &types.User{ID: "other-2"}
	sessionID := "ses_cross"

	session := &types.Session{ID: sessionID, Owner: owner.ID}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil)

	rr := s.callHandler(sessionID, SessionMessageRequest{Content: "hello"}, other)
	s.Equal(http.StatusForbidden, rr.Code)
}

// TestRejectsEmptyContent verifies input validation runs before any store calls.
func (s *SessionMessagesHandlerSuite) TestRejectsEmptyContent() {
	user := &types.User{ID: "user-1"}
	rr := s.callHandler("ses_x", SessionMessageRequest{Content: "   "}, user)
	s.Equal(http.StatusBadRequest, rr.Code)
}

// TestNotifyUserIDPropagatesAndClearsOnFailure verifies that when the underlying
// send fails for a non-WS reason (e.g. CreateInteraction error → no interactionID,
// then the WS dispatch returns ErrNoExternalAgentWS — handled as success). To
// exercise the error-cleanup branch we'd need a non-WS failure; instead this
// test asserts the success path: notify mapping is registered for the session.
func (s *SessionMessagesHandlerSuite) TestNotifyUserIDRegisteredOnSuccess() {
	user := &types.User{ID: "user-1"}
	sessionID := "ses_notify"

	session := &types.Session{ID: sessionID, Owner: user.ID}
	// AnyTimes because sendCommandToExternalAgent fires autoStartDevContainerForSession
	// in a goroutine when there's no WS connection — it does another GetSession lookup.
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).AnyTimes()
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).Return(&types.Interaction{ID: "int-2"}, nil)

	rr := s.callHandler(sessionID, SessionMessageRequest{Content: "x", NotifyUserID: "commenter-9"}, user)
	s.Require().Equal(http.StatusOK, rr.Code, "body=%s", rr.Body.String())

	var resp SessionMessageResponse
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &resp))

	s.server.contextMappingsMutex.Lock()
	s.Equal("commenter-9", s.server.requestToCommenterMapping[resp.RequestID])
	s.Equal("commenter-9", s.server.sessionToCommenterMapping[sessionID])
	s.server.contextMappingsMutex.Unlock()
}

// TestErrNoExternalAgentWSIsSurfacedAsSuccess is a direct unit test on the
// session-scoped helper. The interaction is persisted, so callers should see
// a successful return value with both IDs populated even though the WS send
// returned ErrNoExternalAgentWS.
func (s *SessionMessagesHandlerSuite) TestErrNoExternalAgentWSIsSurfacedAsSuccess() {
	sessionID := "ses_helper"
	session := &types.Session{ID: sessionID, Owner: "user-1"}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).AnyTimes()
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).Return(&types.Interaction{ID: "int-3"}, nil)

	requestID, interactionID, err := s.server.sendMessageToSession(context.Background(), sessionID, "hi", "", false)
	s.NoError(err)
	s.NotEmpty(requestID)
	s.Equal("int-3", interactionID)
}

// Sanity: the sentinel error wrapping is what the helper depends on.
func TestErrNoExternalAgentWSIsRecognisable(t *testing.T) {
	wrapped := errors.Join(ErrNoExternalAgentWS, errors.New("more context"))
	if !errors.Is(wrapped, ErrNoExternalAgentWS) {
		t.Fatalf("expected wrapped error to satisfy errors.Is")
	}
}
