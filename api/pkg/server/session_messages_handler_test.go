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
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
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
	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	server   *HelixAPIServer
}

func TestSessionMessagesHandlerSuite(t *testing.T) {
	suite.Run(t, new(SessionMessagesHandlerSuite))
}

func (s *SessionMessagesHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)

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
		externalAgentExecutor:       s.executor,
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

// TestEnqueuesOntoQueue verifies the endpoint now ENQUEUES onto the
// session-scoped prompt queue (the single sender path) rather than
// immediately dispatching: it persists a pending prompt_history row and
// returns its id. Delivery (interaction creation, auto-start, dispatch)
// happens asynchronously in the poller — covered by the prompt-history
// handler tests — so this asserts only the synchronous enqueue contract.
func (s *SessionMessagesHandlerSuite) TestEnqueuesOntoQueue() {
	user := &types.User{ID: "user-1"}
	sessionID := "ses_noWs"

	session := &types.Session{ID: sessionID, Owner: user.ID}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).AnyTimes()

	// The synchronous enqueue: a pending row keyed on the session.
	var captured *types.PromptHistoryEntry
	s.store.EXPECT().CreatePromptHistoryEntry(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, e *types.PromptHistoryEntry) error {
			captured = e
			return nil
		},
	)
	// The background nudge lists the session's prompts; return none so the
	// poller bails immediately and the test stays deterministic.
	s.store.EXPECT().ListPromptHistoryBySession(gomock.Any(), sessionID).Return(nil, nil).AnyTimes()

	rr := s.callHandler(sessionID, SessionMessageRequest{Content: "hello"}, user)
	s.Require().Equal(http.StatusOK, rr.Code, "body=%s", rr.Body.String())

	var resp SessionMessageResponse
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &resp))
	s.NotEmpty(resp.PromptID)

	s.Require().NotNil(captured)
	s.Equal(sessionID, captured.SessionID)
	s.Equal("hello", captured.Content)
	s.Equal("pending", captured.Status)
	s.False(captured.Interrupt)

	// Let the background nudge run its single (AnyTimes) list call and exit
	// before the controller is finished.
	time.Sleep(100 * time.Millisecond)
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

// TestNotifyUserIDCarriedOnRow verifies the commenter notify id is persisted on
// the enqueued row (NotifyUserID). The queue registers the commenter streaming
// mapping from this field at dispatch — see sendQueuedPromptToSession — rather
// than synchronously at the endpoint as the old direct path did.
func (s *SessionMessagesHandlerSuite) TestNotifyUserIDCarriedOnRow() {
	user := &types.User{ID: "user-1"}
	sessionID := "ses_notify"

	session := &types.Session{ID: sessionID, Owner: user.ID}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).AnyTimes()
	var captured *types.PromptHistoryEntry
	s.store.EXPECT().CreatePromptHistoryEntry(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, e *types.PromptHistoryEntry) error {
			captured = e
			return nil
		},
	)
	s.store.EXPECT().ListPromptHistoryBySession(gomock.Any(), sessionID).Return(nil, nil).AnyTimes()

	rr := s.callHandler(sessionID, SessionMessageRequest{Content: "x", NotifyUserID: "commenter-9"}, user)
	s.Require().Equal(http.StatusOK, rr.Code, "body=%s", rr.Body.String())

	s.Require().NotNil(captured)
	s.Equal("commenter-9", captured.NotifyUserID)

	time.Sleep(100 * time.Millisecond)
}

// Sanity: the sentinel error wrapping is what the helper depends on.
func TestErrNoExternalAgentWSIsRecognisable(t *testing.T) {
	wrapped := errors.Join(ErrNoExternalAgentWS, errors.New("more context"))
	if !errors.Is(wrapped, ErrNoExternalAgentWS) {
		t.Fatalf("expected wrapped error to satisfy errors.Is")
	}
}
