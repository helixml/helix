package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// OrgDeleteDesktopTeardownSuite pins the contract that deleting an org tears
// down its running spec-task desktops (stop container + soft-delete session)
// instead of leaving them for the idle reaper. Only sessions with a live
// dev container are torn down.
type OrgDeleteDesktopTeardownSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	server   *HelixAPIServer
}

func TestOrgDeleteDesktopTeardownSuite(t *testing.T) {
	suite.Run(t, new(OrgDeleteDesktopTeardownSuite))
}

func (s *OrgDeleteDesktopTeardownSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)
	s.server = &HelixAPIServer{
		Store:                 s.store,
		externalAgentExecutor: s.executor,
	}
}

func (s *OrgDeleteDesktopTeardownSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *OrgDeleteDesktopTeardownSuite) TestStopsOnlyDesktopSessions() {
	ctx := context.Background()
	orgID := "org_teardown"

	withDesktop := &types.Session{ID: "ses_desktop"}
	withDesktop.Metadata.DevContainerID = "ubuntu-external-abc"
	noDesktop := &types.Session{ID: "ses_chat"} // no container → skipped

	s.store.EXPECT().
		ListSessions(ctx, store.ListSessionsQuery{OrganizationID: orgID, IncludeExternalAgents: true}).
		Return([]*types.Session{withDesktop, noDesktop}, int64(2), nil)

	// Only the desktop session is stopped and soft-deleted.
	s.executor.EXPECT().StopDesktop(ctx, "ses_desktop").Return(nil)
	s.store.EXPECT().DeleteSession(ctx, "ses_desktop").Return(withDesktop, nil)

	s.server.stopOrgDesktops(ctx, orgID)
}

// A StopDesktop failure must not abort the sweep: the session is still
// soft-deleted and remaining sessions are still processed.
func (s *OrgDeleteDesktopTeardownSuite) TestStopFailureDoesNotAbortSweep() {
	ctx := context.Background()
	orgID := "org_teardown"

	first := &types.Session{ID: "ses_1"}
	first.Metadata.DevContainerID = "ubuntu-external-1"
	second := &types.Session{ID: "ses_2"}
	second.Metadata.DevContainerID = "ubuntu-external-2"

	s.store.EXPECT().
		ListSessions(ctx, store.ListSessionsQuery{OrganizationID: orgID, IncludeExternalAgents: true}).
		Return([]*types.Session{first, second}, int64(2), nil)

	s.executor.EXPECT().StopDesktop(ctx, "ses_1").Return(context.DeadlineExceeded)
	s.store.EXPECT().DeleteSession(ctx, "ses_1").Return(first, nil)
	s.executor.EXPECT().StopDesktop(ctx, "ses_2").Return(nil)
	s.store.EXPECT().DeleteSession(ctx, "ses_2").Return(second, nil)

	s.server.stopOrgDesktops(ctx, orgID)
}
