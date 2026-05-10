package server

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// AutoStartDevContainerSuite covers autoStartDevContainerForSession and
// startDevContainerForSession — specifically the cold-start case for fresh
// zed_external sessions that have no spec task. See helixml/helix#2397.
type AutoStartDevContainerSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	server   *HelixAPIServer
}

func TestAutoStartDevContainerSuite(t *testing.T) {
	suite.Run(t, new(AutoStartDevContainerSuite))
}

func (s *AutoStartDevContainerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)
	s.server = &HelixAPIServer{
		Cfg: &config.ServerConfig{
			WebServer: config.WebServer{URL: "http://localhost:0"},
		},
		Store:                 s.store,
		pubsub:                pubsub.NewNoop(),
		externalAgentExecutor: s.executor,
		Controller: &controller.Controller{
			Options: controller.Options{Store: s.store, PubSub: pubsub.NewNoop()},
		},
	}
}

func (s *AutoStartDevContainerSuite) TearDownTest() {
	s.ctrl.Finish()
}

// Fresh zed_external session with no spec task — used to early-return without
// calling the executor, leaving queued messages stuck. The fix: build a
// DesktopAgent from the session and invoke StartDesktop. See
// helixml/helix#2397.
func (s *AutoStartDevContainerSuite) TestAutoStart_ExploratoryZedExternal_CallsStartDesktop() {
	sessionID := "ses_exploratory"
	projectID := "prj_exploratory"
	orgID := "org_exploratory"

	session := &types.Session{
		ID:             sessionID,
		Owner:          "user-1",
		ProjectID:      projectID,
		OrganizationID: orgID,
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
			// Deliberately no SpecTaskID — exploratory session.
		},
	}

	// autoStartDevContainerForSession does its own GetSession; startDevContainer-
	// ForSession then re-fetches after StartDesktop returns. AnyTimes covers both.
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).AnyTimes()
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return([]*types.GitRepository{}, nil)
	// UpdateSession runs only when the post-StartDesktop refetch returns non-nil.
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(&types.Session{ID: sessionID}, nil).AnyTimes()

	called := false
	s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error) {
			called = true
			s.Equal(sessionID, agent.SessionID)
			s.Equal(projectID, agent.ProjectID)
			s.Equal(orgID, agent.OrganizationID)
			s.Empty(agent.SpecTaskID, "exploratory session must not pin a spec task on the agent")
			return &types.DesktopAgentResponse{SessionID: sessionID, Status: "running"}, nil
		},
	)

	s.server.autoStartDevContainerForSession(sessionID)
	s.True(called, "StartDesktop must be invoked for exploratory zed_external sessions")
}

// Non-zed_external session (e.g. a helix-agent text session) has no desktop
// container to wake. The auto-start should be a no-op.
func (s *AutoStartDevContainerSuite) TestAutoStart_NonZedExternal_IsNoop() {
	sessionID := "ses_text"
	session := &types.Session{
		ID:    sessionID,
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			AgentType: "helix",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil)
	// No StartDesktop expectation — gomock will fail if it's called.

	s.server.autoStartDevContainerForSession(sessionID)
}

// startDevContainerForSession surfaces StartDesktop errors so callers can log
// or retry. autoStartDevContainerForSession swallows the error (fire-and-forget),
// but the helper itself should bubble it up.
func (s *AutoStartDevContainerSuite) TestStartDevContainerForSession_PropagatesExecutorError() {
	session := &types.Session{
		ID:    "ses_err",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
		},
	}
	s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	err := s.server.startDevContainerForSession(context.Background(), session)
	s.Require().Error(err)
	s.Contains(err.Error(), "failed to start dev container")
}

// Defence-in-depth: if the executor isn't wired up, the helper must surface a
// clear error rather than panicking on a nil deref.
func (s *AutoStartDevContainerSuite) TestStartDevContainerForSession_NoExecutor() {
	noExec := &HelixAPIServer{
		Cfg:    &config.ServerConfig{},
		Store:  s.store,
		pubsub: pubsub.NewNoop(),
		Controller: &controller.Controller{
			Options: controller.Options{Store: s.store, PubSub: pubsub.NewNoop()},
		},
	}
	err := noExec.startDevContainerForSession(context.Background(), &types.Session{ID: "ses_x"})
	s.Require().Error(err)
	s.Contains(err.Error(), "external agent executor not available")
}
