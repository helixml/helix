package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// fakeTransport records calls made by zedACPBackend so tests can assert on them
// without a live WebSocket connection.
type fakeTransport struct {
	cancelled    []string
	commandsSent []types.ExternalAgentCommand
	sendErr      error
}

func (f *fakeTransport) cancelCurrentTurnIfActive(_ context.Context, sessionID string) {
	f.cancelled = append(f.cancelled, sessionID)
}

func (f *fakeTransport) sendCommandToExternalAgent(_ string, command types.ExternalAgentCommand) error {
	f.commandsSent = append(f.commandsSent, command)
	return f.sendErr
}

type SessionClearSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	ctx       context.Context
	store     *store.MockStore
	apiServer *HelixAPIServer
}

func TestSessionClearSuite(t *testing.T) {
	suite.Run(t, new(SessionClearSuite))
}

func (suite *SessionClearSuite) SetupTest() {
	suite.ctrl = gomock.NewController(suite.T())
	suite.ctx = context.Background()
	suite.store = store.NewMockStore(suite.ctrl)
	suite.apiServer = &HelixAPIServer{
		Store: suite.store,
	}
}

func (suite *SessionClearSuite) TearDownTest() {
	suite.ctrl.Finish()
}

// --- internal agent backend ---

func (suite *SessionClearSuite) TestInternalBackend_NilLookup_NoOp() {
	b := &internalAgentBackend{}
	suite.NoError(b.Clear(suite.ctx, "ses_1"))
}

func (suite *SessionClearSuite) TestInternalBackend_NoLiveSession_NoOp() {
	called := false
	b := &internalAgentBackend{
		liveSession: func(_ string) *agent.Session {
			called = true
			return nil // no live in-memory session for this id
		},
	}
	suite.NoError(b.Clear(suite.ctx, "ses_1"))
	suite.True(called)
}

// The internal backend relies on MessageList.Clear() to empty a live history;
// document that mechanism here (constructing a full agent.Session is overkill).
func (suite *SessionClearSuite) TestMessageListClearMechanism() {
	history := agent.NewMessageList()
	history.Add(&openai.ChatCompletionMessage{Role: "user", Content: "hi"})
	suite.NotEmpty(history.All())
	history.Clear()
	suite.Empty(history.All())
}

// --- zed/ACP backend ---

func (suite *SessionClearSuite) TestZedBackend_ResetsThreadAndCancels() {
	transport := &fakeTransport{}
	b := &zedACPBackend{store: suite.store, transport: transport}

	session := &types.Session{
		ID: "ses_zed",
		Metadata: types.SessionMetadata{
			AgentType:   "zed_external",
			ZedThreadID: "thread-abc",
		},
	}
	suite.store.EXPECT().GetSession(gomock.Any(), "ses_zed").Return(session, nil)
	suite.store.EXPECT().
		UpdateSessionMetadata(gomock.Any(), "ses_zed", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, m types.SessionMetadata) error {
			suite.Equal("", m.ZedThreadID, "ZedThreadID should be reset to empty")
			return nil
		})

	suite.NoError(b.Clear(suite.ctx, "ses_zed"))
	suite.Equal([]string{"ses_zed"}, transport.cancelled, "in-flight turn should be cancelled")
}

func (suite *SessionClearSuite) TestZedBackend_AlreadyFreshThread_NoPersist() {
	transport := &fakeTransport{}
	b := &zedACPBackend{store: suite.store, transport: transport}

	session := &types.Session{
		ID:       "ses_zed",
		Metadata: types.SessionMetadata{AgentType: "zed_external"}, // no thread id
	}
	suite.store.EXPECT().GetSession(gomock.Any(), "ses_zed").Return(session, nil)
	// No UpdateSessionMetadata expected.

	suite.NoError(b.Clear(suite.ctx, "ses_zed"))
	suite.Equal([]string{"ses_zed"}, transport.cancelled)
}

// --- backendFor dispatch ---

func (suite *SessionClearSuite) TestBackendFor_Dispatch() {
	zed := &types.Session{Metadata: types.SessionMetadata{AgentType: "zed_external"}}
	suite.IsType(&zedACPBackend{}, suite.apiServer.backendFor(zed))

	external := &types.Session{Metadata: types.SessionMetadata{ExternalAgentID: "ext_1"}}
	suite.IsType(&zedACPBackend{}, suite.apiServer.backendFor(external))

	internal := &types.Session{Metadata: types.SessionMetadata{}}
	suite.IsType(&internalAgentBackend{}, suite.apiServer.backendFor(internal))
}

// --- coordinator ---

func (suite *SessionClearSuite) TestClearSession_InternalAlwaysClearsDB() {
	session := &types.Session{ID: "ses_int", Owner: "user123"}

	gomock.InOrder(
		suite.store.EXPECT().GetSession(gomock.Any(), "ses_int").Return(session, nil),
		suite.store.EXPECT().ClearSessionInteractions(gomock.Any(), "ses_int").Return(nil),
		suite.store.EXPECT().TouchSession(gomock.Any(), "ses_int").Return(nil),
		suite.store.EXPECT().GetSession(gomock.Any(), "ses_int").Return(session, nil),
	)

	out, err := suite.apiServer.ClearSession(suite.ctx, "ses_int")
	suite.NoError(err)
	suite.Equal("ses_int", out.ID)
}

func (suite *SessionClearSuite) TestClearSession_ZedDispatch() {
	session := &types.Session{
		ID:    "ses_zed",
		Owner: "user123",
		Metadata: types.SessionMetadata{
			AgentType:   "zed_external",
			ZedThreadID: "thread-abc",
		},
	}

	// GetSession is called by ClearSession, by cancelCurrentTurnIfActive, by the
	// zed backend, and for the final return — allow it any number of times.
	suite.store.EXPECT().GetSession(gomock.Any(), "ses_zed").Return(session, nil).AnyTimes()
	suite.store.EXPECT().ClearSessionInteractions(gomock.Any(), "ses_zed").Return(nil)
	suite.store.EXPECT().
		UpdateSessionMetadata(gomock.Any(), "ses_zed", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, m types.SessionMetadata) error {
			suite.Equal("", m.ZedThreadID)
			return nil
		})
	suite.store.EXPECT().TouchSession(gomock.Any(), "ses_zed").Return(nil)

	out, err := suite.apiServer.ClearSession(suite.ctx, "ses_zed")
	suite.NoError(err)
	suite.Equal("ses_zed", out.ID)
}

// --- handler ---

func (suite *SessionClearSuite) clearRequest(id string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+id+"/clear", nil)
	ctx := setRequestUser(context.Background(), types.User{ID: "user123", Type: types.OwnerTypeUser})
	// SetURLVars stores vars in the request context, so apply it after WithContext
	// (which would otherwise replace the context and drop the vars).
	return mux.SetURLVars(req.WithContext(ctx), map[string]string{"id": id})
}

func (suite *SessionClearSuite) TestHandler_Success() {
	session := &types.Session{ID: "ses_int", Owner: "user123"}
	suite.store.EXPECT().GetSession(gomock.Any(), "ses_int").Return(session, nil).AnyTimes()
	suite.store.EXPECT().ClearSessionInteractions(gomock.Any(), "ses_int").Return(nil)
	suite.store.EXPECT().TouchSession(gomock.Any(), "ses_int").Return(nil)

	out, herr := suite.apiServer.clearSessionHandler(httptest.NewRecorder(), suite.clearRequest("ses_int"))
	suite.Nil(herr)
	suite.NotNil(out)
	suite.Equal("ses_int", out.ID)
}

func (suite *SessionClearSuite) TestHandler_NotFound() {
	suite.store.EXPECT().GetSession(gomock.Any(), "ses_missing").Return(nil, store.ErrNotFound)

	out, herr := suite.apiServer.clearSessionHandler(httptest.NewRecorder(), suite.clearRequest("ses_missing"))
	suite.Nil(out)
	suite.Require().NotNil(herr)
	suite.Equal(http.StatusNotFound, herr.StatusCode)
}

func (suite *SessionClearSuite) TestHandler_Unauthorized() {
	// Session owned by a different user, no organization → not authorized.
	session := &types.Session{ID: "ses_other", Owner: "someone-else"}
	suite.store.EXPECT().GetSession(gomock.Any(), "ses_other").Return(session, nil)

	out, herr := suite.apiServer.clearSessionHandler(httptest.NewRecorder(), suite.clearRequest("ses_other"))
	suite.Nil(out)
	suite.Require().NotNil(herr)
	suite.Equal(http.StatusForbidden, herr.StatusCode)
}

