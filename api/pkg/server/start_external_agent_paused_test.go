package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// StartExternalAgentPausedSuite is a regression gate for the bug where the
// desktop viewer rendered "paused" while the container was actually running.
//
// StartExternalAgentSession used to re-save its stale in-memory `session`
// struct via UpdateSession after StartDesktop had already persisted the
// container metadata (container_name, external_agent_status="running", …)
// onto the same row. That trailing write blanked container_name and
// external_agent_status, and the frontend useSandboxState hook maps an empty
// container_name to "absent" → "paused".
//
// The fix re-fetches the fresh row after StartDesktop instead of re-saving the
// stale copy. This test mirrors HydraExecutor.StartDesktop's persistence and
// asserts the metadata survives both on the returned struct and on the row.
//
// See helix-specs:002113_desktop-shows-paused/design.md.
type StartExternalAgentPausedSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	server   *HelixAPIServer
}

func TestStartExternalAgentPausedSuite(t *testing.T) {
	suite.Run(t, new(StartExternalAgentPausedSuite))
}

func (s *StartExternalAgentPausedSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)

	s.server = &HelixAPIServer{
		Store:                 s.store,
		externalAgentExecutor: s.executor,
		Cfg:                   &config.ServerConfig{},
		Controller: &controller.Controller{
			Options: controller.Options{Store: s.store, PubSub: pubsub.NewNoop()},
		},
	}
}

func (s *StartExternalAgentPausedSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *StartExternalAgentPausedSuite) TestContainerMetadataSurvivesStartDesktop() {
	ctx := context.Background()
	const (
		userID            = "user_op"
		wantContainerName = "ubuntu-external-cold123"
	)

	// Stateful session row, exactly like the DB. GetSession reads it,
	// UpdateSession writes it. This lets the StartDesktop mock persist
	// container metadata the way the real HydraExecutor does, so we can
	// observe whether StartExternalAgentSession clobbers it afterwards.
	sessions := make(map[string]*types.Session)
	s.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id string) (*types.Session, error) {
			if sess, ok := sessions[id]; ok {
				cp := *sess
				return &cp, nil
			}
			return nil, store.ErrNotFound
		},
	).AnyTimes()
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, sess types.Session) (*types.Session, error) {
			cp := sess
			sessions[sess.ID] = &cp
			return &cp, nil
		},
	).AnyTimes()
	s.store.EXPECT().CreateInteractions(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	s.store.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{
		ID:   userID,
		Type: types.OwnerTypeUser,
	}, nil)

	// Mirror HydraExecutor.StartDesktop: re-fetch the row and persist the
	// container metadata onto it before returning.
	s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error) {
			dbSession, ok := sessions[agent.SessionID]
			s.Require().True(ok, "session row must exist before StartDesktop persists container metadata")
			dbSession.Metadata.ContainerName = wantContainerName
			dbSession.Metadata.ContainerID = "ctr_123"
			dbSession.Metadata.DevContainerID = "ctr_123"
			dbSession.Metadata.ExternalAgentStatus = "running"
			return &types.DesktopAgentResponse{
				DevContainerID: "ctr_123",
				ContainerName:  wantContainerName,
				SandboxID:      "sbx_1",
			}, nil
		},
	).Times(1)

	req := &types.SessionChatRequest{
		AgentType: "zed_external",
		Messages: []*types.Message{{
			Role:    "user",
			Content: types.MessageContent{Parts: []any{"do the thing"}},
		}},
	}

	got, err := s.server.StartExternalAgentSession(ctx, req, userID)
	s.Require().NoError(err)
	s.Require().NotNil(got)

	// Load-bearing assertions. On main (before the fix) both fail because the
	// trailing UpdateSession(*session) re-saved the stale in-memory struct,
	// wiping container_name/external_agent_status and returning the stale copy.
	s.Equal(wantContainerName, got.Metadata.ContainerName,
		"returned session must carry the container_name StartDesktop persisted, not an empty value")
	s.Equal("running", got.Metadata.ExternalAgentStatus,
		"returned session must report external_agent_status=running, not blanked")

	// And the persisted row must not have been clobbered.
	persisted := sessions[got.ID]
	s.Require().NotNil(persisted)
	s.Equal(wantContainerName, persisted.Metadata.ContainerName,
		"persisted row must keep container_name; clobbering it makes the desktop viewer show paused")
	s.Equal("running", persisted.Metadata.ExternalAgentStatus,
		"persisted row must keep external_agent_status=running")
}
