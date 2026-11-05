package webservice

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/sandbox"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"
)

// RecoveryGateSuite covers the guard that stopped the 2026-09-27 we-find.ai
// outage repeating: recovery must not DELETE a web-service sandbox when its
// pinned runner could not accept a replacement.
//
// The assertions lean on gomock's strictness — a Delete or a Redeploy would
// reach the store (DeleteSandbox / GetGitRepository / CreateWebServiceDeploy),
// and with no EXPECT for those the test fails. So "no unexpected store calls"
// IS the assertion that nothing destructive happened.
type RecoveryGateSuite struct {
	suite.Suite
	ctrl  *gomock.Controller
	store *store.MockStore
	ctlr  *Controller
	ctx   context.Context
}

func TestRecoveryGateSuite(t *testing.T) { suite.Run(t, new(RecoveryGateSuite)) }

func (s *RecoveryGateSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.ctx = context.Background()
	runtimes, err := sandbox.NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "headless-ubuntu=ubuntu:22.04|sleep infinity",
		DefaultRuntime: "headless-ubuntu",
	})
	s.Require().NoError(err)
	// The webservice controller and the sandbox controller share the mock
	// store, which is how the real wiring works too.
	s.ctlr = New(s.store, sandbox.New(s.store, nil, runtimes, "", ""))
}

func (s *RecoveryGateSuite) TearDownTest() { s.ctrl.Finish() }

// expectRecoveryUpTo sets up the lookups every recovery does before deciding,
// with a sandbox that is NOT running — which reaches the destructive branch
// without going near the RevDial probe or `docker info`.
func (s *RecoveryGateSuite) expectRecoveryUpTo(host *types.SandboxInstance) {
	s.store.EXPECT().GetProjectWebServiceState(gomock.Any(), "prj_findai").
		Return(&types.ProjectWebServiceState{
			ProjectID:       "prj_findai",
			Enabled:         true,
			ActiveSandboxID: "sbx_web",
			ContainerPort:   8080,
			HostDeviceID:    "code-for-app",
		}, nil)
	s.store.EXPECT().GetProject(gomock.Any(), "prj_findai").
		Return(&types.Project{ID: "prj_findai", UserID: "usr_1"}, nil)
	s.store.EXPECT().GetSandbox(gomock.Any(), "sbx_web").
		Return(&types.Sandbox{
			ID:           "sbx_web",
			Runtime:      types.SandboxRuntimeUbuntuDesktop,
			Persistent:   true,
			HostDeviceID: "code-for-app",
			Status:       types.SandboxStatusStopped,
		}, nil)
	s.store.EXPECT().GetSandboxInstance(gomock.Any(), "code-for-app").Return(host, nil)
}

func advertisingHost(status string, lastSeen time.Time) *types.SandboxInstance {
	versions, _ := json.Marshal(map[string]string{"ubuntu": "2.12.23-linux-amd64"})
	return &types.SandboxInstance{
		ID:              "code-for-app",
		Status:          status,
		LastSeen:        lastSeen,
		GPUVendor:       "nvidia",
		DesktopVersions: datatypes.JSON(versions),
	}
}

// The outage shape: runner offline. Recovery must refuse, destroy nothing, and
// report a FAILED recovery so backoff grows and the looping alert can fire.
func (s *RecoveryGateSuite) TestRefusesToDeleteWhenRunnerOffline() {
	s.expectRecoveryUpTo(advertisingHost("offline", time.Now()))

	deploy, err := s.ctlr.RecoverWebService(s.ctx, "prj_findai")

	s.Nil(deploy, "no deploy should be started when the runner cannot take one")
	s.Require().Error(err, "must report a failed recovery, not a silent no-op")
	s.True(IsRecoveryDeferred(err))
	s.Contains(err.Error(), "offline")
}

// The middle window that made this so hard to see: the row still reads
// "online" because the reaper only flips at 5m, but the host has already
// stopped being a valid placement target at 90s.
func (s *RecoveryGateSuite) TestRefusesToDeleteWhenRunnerHeartbeatStale() {
	s.expectRecoveryUpTo(advertisingHost("online", time.Now().Add(-4*time.Minute)))

	deploy, err := s.ctlr.RecoverWebService(s.ctx, "prj_findai")

	s.Nil(deploy)
	s.Require().Error(err)
	s.Contains(err.Error(), "heartbeat")
}

// A runner that stopped advertising the desktop image (mid-upgrade, failed
// pull) also cannot take a replacement.
func (s *RecoveryGateSuite) TestRefusesToDeleteWhenRunnerStoppedAdvertisingImage() {
	host := advertisingHost("online", time.Now())
	host.DesktopVersions = nil
	s.expectRecoveryUpTo(host)

	deploy, err := s.ctlr.RecoverWebService(s.ctx, "prj_findai")

	s.Nil(deploy)
	s.Require().Error(err)
	s.Contains(err.Error(), "advertises")
}

func (s *RecoveryGateSuite) TestLowDiskReadinessDoesNotDeleteOrRedeploy() {
	s.store.EXPECT().GetProjectWebServiceState(gomock.Any(), "prj_findai").Return(&types.ProjectWebServiceState{
		ProjectID: "prj_findai", Enabled: true, ActiveSandboxID: "sbx_web", HostDeviceID: "code-for-app",
	}, nil)
	s.store.EXPECT().GetProject(gomock.Any(), "prj_findai").Return(&types.Project{ID: "prj_findai", UserID: "usr_1"}, nil)
	s.store.EXPECT().GetSandbox(gomock.Any(), "sbx_web").Return(&types.Sandbox{
		ID: "sbx_web", Runtime: types.SandboxRuntimeUbuntuDesktop, Persistent: true,
		HostDeviceID: "code-for-app", Status: types.SandboxStatusStopped,
	}, nil)
	s.ctlr.hostReadyForReplacement = func(context.Context, *types.Sandbox) (bool, string) {
		return false, "Hydra is not ready: disk space critically low"
	}
	s.ctlr.deleteSandboxForRecovery = func(context.Context, string) error {
		s.FailNow("low-disk recovery deleted the existing sandbox")
		return nil
	}

	deploy, err := s.ctlr.RecoverWebService(s.ctx, "prj_findai")

	s.Nil(deploy)
	s.Require().Error(err)
	s.True(IsRecoveryDeferred(err))
	s.Contains(err.Error(), "disk space critically low")
}

func (s *RecoveryGateSuite) TestMissingSandboxUsesPinnedHostBeforeRedeploy() {
	s.store.EXPECT().GetProjectWebServiceState(gomock.Any(), "prj_findai").Return(&types.ProjectWebServiceState{
		ProjectID: "prj_findai", Enabled: true, ActiveSandboxID: "sbx_missing", HostDeviceID: "host-pinned",
	}, nil)
	s.store.EXPECT().GetProject(gomock.Any(), "prj_findai").Return(&types.Project{ID: "prj_findai", UserID: "usr_1"}, nil)
	s.store.EXPECT().GetSandbox(gomock.Any(), "sbx_missing").Return(nil, store.ErrNotFound)
	s.ctlr.hostReadyForReplacement = func(_ context.Context, sb *types.Sandbox) (bool, string) {
		s.Equal("host-pinned", sb.HostDeviceID)
		return false, "revdial disconnected"
	}

	deploy, err := s.ctlr.RecoverWebService(s.ctx, "prj_findai")
	s.Nil(deploy)
	s.True(IsRecoveryDeferred(err))
}

func (s *RecoveryGateSuite) TestSandboxLookupFailureIsNotTreatedAsMissing() {
	s.store.EXPECT().GetProjectWebServiceState(gomock.Any(), "prj_findai").Return(&types.ProjectWebServiceState{
		ProjectID: "prj_findai", Enabled: true, ActiveSandboxID: "sbx_web", HostDeviceID: "host-pinned",
	}, nil)
	s.store.EXPECT().GetProject(gomock.Any(), "prj_findai").Return(&types.Project{ID: "prj_findai", UserID: "usr_1"}, nil)
	s.store.EXPECT().GetSandbox(gomock.Any(), "sbx_web").Return(nil, errors.New("database unavailable"))

	deploy, err := s.ctlr.RecoverWebService(s.ctx, "prj_findai")
	s.Nil(deploy)
	s.Require().Error(err)
	s.False(IsRecoveryDeferred(err))
	s.Contains(err.Error(), "database unavailable")
}

func (s *RecoveryGateSuite) TestUnconfirmedStrictTeardownDoesNotRedeploy() {
	s.store.EXPECT().GetProjectWebServiceState(gomock.Any(), "prj_findai").Return(&types.ProjectWebServiceState{
		ProjectID: "prj_findai", Enabled: true, ActiveSandboxID: "sbx_web", HostDeviceID: "host-pinned",
	}, nil)
	s.store.EXPECT().GetProject(gomock.Any(), "prj_findai").Return(&types.Project{ID: "prj_findai", UserID: "usr_1"}, nil)
	s.store.EXPECT().GetSandbox(gomock.Any(), "sbx_web").Return(&types.Sandbox{
		ID: "sbx_web", Runtime: types.SandboxRuntimeUbuntuDesktop, Persistent: true,
		HostDeviceID: "host-pinned", Status: types.SandboxStatusStopped,
	}, nil)
	s.ctlr.hostReadyForReplacement = func(context.Context, *types.Sandbox) (bool, string) { return true, "" }
	s.ctlr.deleteSandboxForRecovery = func(_ context.Context, id string) error {
		return &sandbox.RecoveryTeardownUnconfirmedError{SandboxID: id, Err: errors.New("revdial disconnected")}
	}

	deploy, err := s.ctlr.RecoverWebService(s.ctx, "prj_findai")
	s.Nil(deploy)
	s.True(IsRecoveryDeferred(err))
}

func (s *RecoveryGateSuite) TestPersistsSandboxRunnerBeforeDestructiveRecovery() {
	s.store.EXPECT().GetProjectWebServiceState(gomock.Any(), "prj_findai").Return(&types.ProjectWebServiceState{
		ProjectID: "prj_findai", Enabled: true, ActiveSandboxID: "sbx_web",
	}, nil)
	s.store.EXPECT().GetProject(gomock.Any(), "prj_findai").Return(&types.Project{ID: "prj_findai", UserID: "usr_1"}, nil)
	s.store.EXPECT().GetSandbox(gomock.Any(), "sbx_web").Return(&types.Sandbox{
		ID: "sbx_web", Runtime: types.SandboxRuntimeUbuntuDesktop, Persistent: true,
		HostDeviceID: "host-pinned", Status: types.SandboxStatusStopped,
	}, nil)
	s.ctlr.hostReadyForReplacement = func(context.Context, *types.Sandbox) (bool, string) { return true, "" }
	s.store.EXPECT().SetWebServiceHostDeviceID(gomock.Any(), "prj_findai", "host-pinned").
		Return(errors.New("database unavailable"))

	deploy, err := s.ctlr.RecoverWebService(s.ctx, "prj_findai")
	s.Nil(deploy)
	s.Require().Error(err)
	s.Contains(err.Error(), "record web service runner pin")
}

// Nothing to recover: the guard must not fire, and nothing else is touched.
func (s *RecoveryGateSuite) TestNoActiveSandboxIsNotAnError() {
	s.store.EXPECT().GetProjectWebServiceState(gomock.Any(), "prj_findai").
		Return(&types.ProjectWebServiceState{ProjectID: "prj_findai", Enabled: true}, nil)

	deploy, err := s.ctlr.RecoverWebService(s.ctx, "prj_findai")
	s.Nil(deploy)
	s.NoError(err)
}
