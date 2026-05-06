package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type ControllerCleanupSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	store      *store.MockStore
	controller *Controller
	ctx        context.Context
}

func TestControllerCleanupSuite(t *testing.T) {
	suite.Run(t, new(ControllerCleanupSuite))
}

func (s *ControllerCleanupSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.ctx = context.Background()

	runtimes, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "headless-ubuntu=ubuntu:22.04|sleep infinity",
		DefaultRuntime: "headless-ubuntu",
	})
	s.Require().NoError(err)
	s.controller = New(s.store, nil, runtimes, "", "")
}

func (s *ControllerCleanupSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ControllerCleanupSuite) TestCleanupStoppedNonPersistentDeletesRowsPastGracePeriod() {
	stoppedAt := time.Now().Add(-2 * time.Hour)
	sb := &types.Sandbox{
		ID:        "sbx_stopped",
		Runtime:   types.SandboxRuntimeHeadlessUbuntu,
		Status:    types.SandboxStatusStopped,
		StoppedAt: &stoppedAt,
	}

	s.store.EXPECT().ListStoppedNonPersistentSandboxes(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, before time.Time) ([]*types.Sandbox, error) {
			s.Require().WithinDuration(time.Now().Add(-time.Hour), before, time.Second)
			return []*types.Sandbox{sb}, nil
		},
	)
	s.store.EXPECT().GetSandbox(gomock.Any(), "sbx_stopped").Return(sb, nil)
	s.store.EXPECT().SetSandboxStatus(gomock.Any(), "sbx_stopped", types.SandboxStatusStopping, "").Return(nil)
	s.store.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
	s.store.EXPECT().DeleteSandbox(gomock.Any(), "sbx_stopped").Return(nil)

	s.Require().NoError(s.controller.CleanupStoppedNonPersistent(s.ctx))
}
