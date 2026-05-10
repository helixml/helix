package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/controller"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// StartDevContainerForSessionSuite covers the project-context resolution
// in startDevContainerForSession. Regression coverage for helixml/helix#2397
// where exploratory zed_external sessions (no spec task) silently no-op'd.
type StartDevContainerForSessionSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	server   *HelixAPIServer
}

func TestStartDevContainerForSessionSuite(t *testing.T) {
	suite.Run(t, new(StartDevContainerForSessionSuite))
}

func (s *StartDevContainerForSessionSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)

	s.server = &HelixAPIServer{
		Store:                 s.store,
		externalAgentExecutor: s.executor,
		Controller: &controller.Controller{
			Options: controller.Options{Store: s.store, PubSub: pubsub.NewNoop()},
		},
	}
}

func (s *StartDevContainerForSessionSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestSpecTaskShape: session has SpecTaskID — load spec task, take project/org from it.
func (s *StartDevContainerForSessionSuite) TestSpecTaskShape() {
	session := &types.Session{
		ID:    "ses_spec",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			AgentType:  "zed_external",
			SpecTaskID: "spt_abc",
		},
	}
	specTask := &types.SpecTask{
		ID:             "spt_abc",
		ProjectID:      "prj_from_spectask",
		OrganizationID: "org_from_spectask",
	}
	s.store.EXPECT().GetSpecTask(gomock.Any(), "spt_abc").Return(specTask, nil)
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.store.EXPECT().GetSession(gomock.Any(), "ses_spec").Return(session, nil)
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(&types.Session{}, nil)

	var captured *types.DesktopAgent
	s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error) {
			captured = agent
			return &types.DesktopAgentResponse{DevContainerID: "dev_test"}, nil
		},
	).Times(1)

	err := s.server.startDevContainerForSession(context.Background(), session)
	s.NoError(err)
	s.Require().NotNil(captured)
	s.Equal("ses_spec", captured.SessionID)
	s.Equal("spt_abc", captured.SpecTaskID)
	s.Equal("prj_from_spectask", captured.ProjectID)
	s.Equal("org_from_spectask", captured.OrganizationID)
}

// TestExploratoryShape: session has Metadata.ProjectID, no spec task — the
// helixml/helix#2397 regression case. Must call StartDesktop with project
// from session metadata.
func (s *StartDevContainerForSessionSuite) TestExploratoryShape() {
	session := &types.Session{
		ID:             "ses_exploratory",
		Owner:          "user-1",
		OrganizationID: "org_from_session",
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
			ProjectID: "prj_from_metadata",
		},
	}
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.store.EXPECT().GetSession(gomock.Any(), "ses_exploratory").Return(session, nil)
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(&types.Session{}, nil)

	var captured *types.DesktopAgent
	s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error) {
			captured = agent
			return &types.DesktopAgentResponse{DevContainerID: "dev_test"}, nil
		},
	).Times(1)

	err := s.server.startDevContainerForSession(context.Background(), session)
	s.NoError(err)
	s.Require().NotNil(captured)
	s.Equal("ses_exploratory", captured.SessionID)
	s.Empty(captured.SpecTaskID)
	s.Equal("prj_from_metadata", captured.ProjectID)
	s.Equal("org_from_session", captured.OrganizationID)
}

// TestLegacyShape: session has session.ProjectID (no metadata project, no spec task).
func (s *StartDevContainerForSessionSuite) TestLegacyShape() {
	session := &types.Session{
		ID:             "ses_legacy",
		Owner:          "user-1",
		ProjectID:      "prj_from_row",
		OrganizationID: "org_from_session",
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
		},
	}
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.store.EXPECT().GetSession(gomock.Any(), "ses_legacy").Return(session, nil)
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(&types.Session{}, nil)

	var captured *types.DesktopAgent
	s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error) {
			captured = agent
			return &types.DesktopAgentResponse{DevContainerID: "dev_test"}, nil
		},
	).Times(1)

	err := s.server.startDevContainerForSession(context.Background(), session)
	s.NoError(err)
	s.Require().NotNil(captured)
	s.Equal("prj_from_row", captured.ProjectID)
	s.Equal("org_from_session", captured.OrganizationID)
}

// TestNoProjectContext: session has no spec task, no metadata project, no
// session.ProjectID. Helper must log + return nil (no error). StartDesktop
// must NOT be called — gomock will fail if it is.
func (s *StartDevContainerForSessionSuite) TestNoProjectContext() {
	session := &types.Session{
		ID:    "ses_empty",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
		},
	}

	err := s.server.startDevContainerForSession(context.Background(), session)
	s.NoError(err)
}

// TestNilSession: defensive — must return error, not panic.
func (s *StartDevContainerForSessionSuite) TestNilSession() {
	err := s.server.startDevContainerForSession(context.Background(), nil)
	s.Error(err)
}

// TestNoExecutor: external agent executor not wired (e.g. on instances that
// don't run desktops). Helper must return error, not panic.
func (s *StartDevContainerForSessionSuite) TestNoExecutor() {
	s.server.externalAgentExecutor = nil
	session := &types.Session{
		ID: "ses_x",
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
			ProjectID: "prj_x",
		},
	}
	err := s.server.startDevContainerForSession(context.Background(), session)
	s.Error(err)
}
