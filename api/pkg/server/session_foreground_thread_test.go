package server

import (
	"encoding/json"
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

// ForegroundThreadHandlerSuite tests POST /api/v1/sessions/{id}/foreground-thread,
// which tells the per-spec-task Zed desktop to foreground the thread belonging to
// the session the user is viewing — so the streamed desktop tracks the chat panel
// instead of drifting to a different thread ("opened != sent-to").
type ForegroundThreadHandlerSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	server   *HelixAPIServer
}

func TestForegroundThreadHandlerSuite(t *testing.T) {
	suite.Run(t, new(ForegroundThreadHandlerSuite))
}

func (s *ForegroundThreadHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)

	s.server = &HelixAPIServer{
		Cfg: &config.ServerConfig{
			WebServer: config.WebServer{URL: "http://localhost:0", Host: "localhost", Port: 0, RunnerToken: "test"},
		},
		Store:  s.store,
		pubsub: pubsub.NewNoop(),
		Controller: &controller.Controller{
			Options: controller.Options{Store: s.store, PubSub: pubsub.NewNoop()},
		},
		externalAgentExecutor:  s.executor,
		externalAgentWSManager: NewExternalAgentWSManager(),
	}
}

func (s *ForegroundThreadHandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ForegroundThreadHandlerSuite) callHandler(sessionID string, user *types.User) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/foreground-thread", nil)
	req = mux.SetURLVars(req, map[string]string{"id": sessionID})
	if user != nil {
		req = req.WithContext(setRequestUser(req.Context(), *user))
	}
	rr := httptest.NewRecorder()
	system.Wrapper(s.server.foregroundSessionThread)(rr, req)
	return rr
}

func (s *ForegroundThreadHandlerSuite) decode(rr *httptest.ResponseRecorder) map[string]string {
	var out map[string]string
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &out), "body=%s", rr.Body.String())
	return out
}

// TestRejectsCrossUser: only the session owner (no org) may foreground its thread.
func (s *ForegroundThreadHandlerSuite) TestRejectsCrossUser() {
	owner := &types.User{ID: "owner-1"}
	other := &types.User{ID: "other-2"}
	sessionID := "ses_cross"

	session := &types.Session{ID: sessionID, Owner: owner.ID, Metadata: types.SessionMetadata{AgentType: "zed_external", ZedThreadID: "thr-1"}}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil)

	rr := s.callHandler(sessionID, other)
	s.Equal(http.StatusForbidden, rr.Code)
}

// TestRejectsNonZedExternal: foregrounding only applies to external Zed sessions.
func (s *ForegroundThreadHandlerSuite) TestRejectsNonZedExternal() {
	user := &types.User{ID: "user-1"}
	sessionID := "ses_plain"

	session := &types.Session{ID: sessionID, Owner: user.ID, Metadata: types.SessionMetadata{AgentType: "text"}}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil)

	rr := s.callHandler(sessionID, user)
	s.Equal(http.StatusBadRequest, rr.Code)
}

// TestNoopWhenNoThread: a session whose first message hasn't created a thread yet
// is a no-op, not an error.
func (s *ForegroundThreadHandlerSuite) TestNoopWhenNoThread() {
	user := &types.User{ID: "user-1"}
	sessionID := "ses_nothread"

	session := &types.Session{ID: sessionID, Owner: user.ID, Metadata: types.SessionMetadata{AgentType: "zed_external"}}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil)

	rr := s.callHandler(sessionID, user)
	s.Require().Equal(http.StatusOK, rr.Code)
	s.Equal("noop", s.decode(rr)["status"])
	s.Equal("no_thread", s.decode(rr)["reason"])
}

// TestNoopWhenNotConnected_DoesNotAutoStart is the critical safety property:
// foregrounding must NEVER boot a desktop. With no live WS connection the handler
// returns a no-op and must not invoke StartDesktop (unlike the message-send path,
// which deliberately auto-starts). StartDesktop has no mock expectation, so any
// call fails the test.
func (s *ForegroundThreadHandlerSuite) TestNoopWhenNotConnected_DoesNotAutoStart() {
	user := &types.User{ID: "user-1"}
	sessionID := "ses_disconnected"

	session := &types.Session{ID: sessionID, Owner: user.ID, Metadata: types.SessionMetadata{AgentType: "zed_external", ZedThreadID: "thr-9", SpecTaskID: "spt_x"}}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).AnyTimes()

	rr := s.callHandler(sessionID, user)
	s.Require().Equal(http.StatusOK, rr.Code)
	s.Equal("noop", s.decode(rr)["status"])
	s.Equal("desktop_not_connected", s.decode(rr)["reason"])

	// Give any (incorrectly) spawned auto-start goroutine a chance to fire so the
	// mock would catch an unexpected StartDesktop call.
	time.Sleep(100 * time.Millisecond)
}

// TestSendsOpenThreadWhenConnected: with a live connection, the handler enqueues
// an open_thread command addressing THIS session's own ZedThreadID — the same
// thread the chat/message path sends to, so opened == sent-to.
func (s *ForegroundThreadHandlerSuite) TestSendsOpenThreadWhenConnected() {
	user := &types.User{ID: "user-1"}
	sessionID := "ses_live"

	session := &types.Session{
		ID:    sessionID,
		Owner: user.ID,
		// ZedAgentName set so getAgentNameForSession short-circuits without store lookups.
		Metadata: types.SessionMetadata{AgentType: "zed_external", ZedThreadID: "thr-live", ZedAgentName: "claude"},
	}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil)

	conn := &ExternalAgentWSConnection{SessionID: sessionID, SendChan: make(chan types.ExternalAgentCommand, 4)}
	s.server.externalAgentWSManager.registerConnection(sessionID, conn)

	rr := s.callHandler(sessionID, user)
	s.Require().Equal(http.StatusOK, rr.Code, "body=%s", rr.Body.String())
	s.Equal("ok", s.decode(rr)["status"])

	select {
	case cmd := <-conn.SendChan:
		s.Equal("open_thread", cmd.Type)
		s.Equal("thr-live", cmd.Data["acp_thread_id"])
		s.Equal("claude", cmd.Data["agent_name"])
	case <-time.After(time.Second):
		s.FailNow("no open_thread command was enqueued")
	}
}
