package server

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/org/application/instances"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type BotInstancesDeleteSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	store     *store.MockStore
	executor  *external_agent.MockExecutor
	instances botInstances
}

func TestBotInstancesDeleteSuite(t *testing.T) { suite.Run(t, new(BotInstancesDeleteSuite)) }

func (s *BotInstancesDeleteSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)
	s.instances = botInstances{server: &HelixAPIServer{
		Store:                 s.store,
		externalAgentExecutor: s.executor,
		Cfg:                   &config.ServerConfig{},
	}}
}

func (s *BotInstancesDeleteSuite) TearDownTest() { s.ctrl.Finish() }

func instanceSession() *types.Session {
	return &types.Session{
		ID:             "ses_instance",
		Owner:          "usr_owner",
		OrganizationID: "org_one",
		SandboxID:      "sbx_host1",
		Metadata: types.SessionMetadata{
			OrgWorkerID: "b-broker",
			SessionRole: types.SessionRoleOrgBotInstance,
		},
	}
}

func callerCtx(userID string, role types.OrganizationRole) context.Context {
	ctx := runtimehelix.WithUserID(context.Background(), userID)
	return helixorgserver.WithOrgAuthorization(ctx, role, false)
}

// The owner's delete tears down in order: sandbox, workspace, then session.
func (s *BotInstancesDeleteSuite) TestOwnerDeletesSandboxWorkspaceAndSession() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_instance").Return(instanceSession(), nil)
	gomock.InOrder(
		s.executor.EXPECT().StopDesktop(gomock.Any(), "ses_instance").Return(nil),
		s.executor.EXPECT().DeleteWorkspace(gomock.Any(), "ses_instance", "sbx_host1").Return(nil),
		s.store.EXPECT().DeleteSession(gomock.Any(), "ses_instance").Return(instanceSession(), nil),
	)

	s.Require().NoError(s.instances.Delete(callerCtx("usr_owner", types.OrganizationRoleMember), "org_one", "b-broker", "ses_instance"))
}

func (s *BotInstancesDeleteSuite) TestOrgOwnerMayDeleteAnotherUsersInstance() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_instance").Return(instanceSession(), nil)
	s.executor.EXPECT().StopDesktop(gomock.Any(), "ses_instance").Return(nil)
	s.executor.EXPECT().DeleteWorkspace(gomock.Any(), "ses_instance", "sbx_host1").Return(nil)
	s.store.EXPECT().DeleteSession(gomock.Any(), "ses_instance").Return(instanceSession(), nil)

	s.Require().NoError(s.instances.Delete(callerCtx("usr_admin", types.OrganizationRoleOwner), "org_one", "b-broker", "ses_instance"))
}

func (s *BotInstancesDeleteSuite) TestMemberCannotDeleteAnotherUsersInstance() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_instance").Return(instanceSession(), nil)

	err := s.instances.Delete(callerCtx("usr_member", types.OrganizationRoleMember), "org_one", "b-broker", "ses_instance")
	s.Require().ErrorIs(err, instances.ErrForbidden)
}

// A session that isn't an instance of this bot (the bot's main session,
// another bot's instance, another org) must never be deleted through here.
func (s *BotInstancesDeleteSuite) TestOnlyThisBotsInstancesAreDeletable() {
	mainSession := instanceSession()
	mainSession.Metadata.SessionRole = "exploratory"
	otherBot := instanceSession()
	otherBot.Metadata.OrgWorkerID = "b-other"
	otherOrg := instanceSession()
	otherOrg.OrganizationID = "org_two"
	for _, session := range []*types.Session{mainSession, otherBot, otherOrg} {
		s.store.EXPECT().GetSession(gomock.Any(), "ses_instance").Return(session, nil)
		err := s.instances.Delete(callerCtx("usr_owner", types.OrganizationRoleOwner), "org_one", "b-broker", "ses_instance")
		s.Require().ErrorIs(err, helixorgstore.ErrNotFound)
	}
}

func (s *BotInstancesDeleteSuite) TestMissingSessionIsNotFound() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_gone").Return(nil, store.ErrNotFound)

	err := s.instances.Delete(callerCtx("usr_owner", types.OrganizationRoleOwner), "org_one", "b-broker", "ses_gone")
	s.Require().ErrorIs(err, helixorgstore.ErrNotFound)
}

// A workspace that can't be deleted keeps the session, so the delete can be
// retried rather than leaving an orphaned workspace behind.
func (s *BotInstancesDeleteSuite) TestWorkspaceFailureKeepsSession() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_instance").Return(instanceSession(), nil)
	s.executor.EXPECT().StopDesktop(gomock.Any(), "ses_instance").Return(nil)
	s.executor.EXPECT().DeleteWorkspace(gomock.Any(), "ses_instance", "sbx_host1").Return(errors.New("sandbox offline"))

	err := s.instances.Delete(callerCtx("usr_owner", types.OrganizationRoleMember), "org_one", "b-broker", "ses_instance")
	s.Require().ErrorContains(err, "sandbox offline")
}
