package sandbox

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// ControllerPlacementSuite locks down the host scheduler. The most important
// behaviour to guarantee is that a sandbox already bound to a host stays on
// that host across stop/start, so the persistent volume on local disk is
// reattached. Moving a persistent sandbox to a different host is silent data
// loss and must never happen.
type ControllerPlacementSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	store      *store.MockStore
	controller *Controller
	ctx        context.Context
}

func TestControllerPlacementSuite(t *testing.T) {
	suite.Run(t, new(ControllerPlacementSuite))
}

func (s *ControllerPlacementSuite) SetupTest() {
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

func (s *ControllerPlacementSuite) TearDownTest() {
	s.ctrl.Finish()
}

// headlessSpec is a small helper so each test reads cleanly.
func (s *ControllerPlacementSuite) headlessSpec() *RuntimeSpec {
	return &RuntimeSpec{
		Name:          "headless-ubuntu",
		Image:         "ubuntu:22.04",
		ContainerType: hydra.DevContainerTypeHeadless,
	}
}

// desktopSpec returns the heartbeat-versioned ubuntu-desktop spec.
func (s *ControllerPlacementSuite) desktopSpec() *RuntimeSpec {
	return &RuntimeSpec{
		Name:          string(types.SandboxRuntimeUbuntuDesktop),
		ContainerType: hydra.DevContainerTypeUbuntu,
		Privileged:    true,
		VersionKey:    "ubuntu",
	}
}

// TestStickyHostPersistentReturnsSameHost is the headline guarantee: a
// persistent sandbox already bound to host A must come back on host A even if
// other hosts are online and would normally be picked.
func (s *ControllerPlacementSuite) TestStickyHostPersistentReturnsSameHost() {
	sb := &types.Sandbox{
		ID:           "sbx_persistent_existing",
		Persistent:   true,
		HostDeviceID: "host-a",
	}
	hostA := &types.SandboxInstance{ID: "host-a", Status: "online"}
	s.store.EXPECT().GetSandboxInstance(s.ctx, "host-a").Return(hostA, nil)

	host, err := s.controller.pickHostForSandbox(s.ctx, sb, s.headlessSpec())
	s.Require().NoError(err)
	s.Require().NotNil(host)
	s.Require().Equal("host-a", host.ID, "persistent sandbox must re-bind to its previous host")
}

// TestStickyHostNonPersistentReturnsSameHost — even non-persistent sandboxes
// stick when the host is still around. The desktop runtime stores
// ephemeral docker-data on the host's local disk, so moving would still
// surprise the user (their installed packages would vanish).
func (s *ControllerPlacementSuite) TestStickyHostNonPersistentReturnsSameHost() {
	sb := &types.Sandbox{
		ID:           "sbx_nonpersistent_existing",
		Persistent:   false,
		HostDeviceID: "host-a",
	}
	hostA := &types.SandboxInstance{ID: "host-a", Status: "online"}
	s.store.EXPECT().GetSandboxInstance(s.ctx, "host-a").Return(hostA, nil)

	host, err := s.controller.pickHostForSandbox(s.ctx, sb, s.headlessSpec())
	s.Require().NoError(err)
	s.Require().Equal("host-a", host.ID)
}

// TestPersistentRefusesToMoveWhenHostOffline — the data-loss-prevention
// invariant. If host-a is offline and the sandbox is persistent, we MUST NOT
// silently reschedule onto host-b: the workspace is on host-a's local disk.
func (s *ControllerPlacementSuite) TestPersistentRefusesToMoveWhenHostOffline() {
	sb := &types.Sandbox{
		ID:           "sbx_persistent_orphan",
		Persistent:   true,
		HostDeviceID: "host-a",
	}
	hostA := &types.SandboxInstance{ID: "host-a", Status: "offline"}
	s.store.EXPECT().GetSandboxInstance(s.ctx, "host-a").Return(hostA, nil)
	// We must NOT see a fresh-host scan here — the controller must fail
	// before trying to pick a different host.

	host, err := s.controller.pickHostForSandbox(s.ctx, sb, s.headlessSpec())
	s.Require().Error(err, "persistent sandbox must refuse to move when its host is offline")
	s.Require().Nil(host)
	s.Require().Contains(err.Error(), "host-a", "error must name the offline host so the user knows what to bring back")
	s.Require().Contains(err.Error(), "persistent", "error should mention persistence as the reason for refusing")
}

// TestNonPersistentReschedulesWhenHostOffline — counterpoint to the
// persistent case: non-persistent sandboxes can safely move because there's
// no data on disk to orphan. We must pick a fresh online host.
func (s *ControllerPlacementSuite) TestNonPersistentReschedulesWhenHostOffline() {
	sb := &types.Sandbox{
		ID:           "sbx_ephem_relocate",
		Persistent:   false,
		HostDeviceID: "host-a",
	}
	hostAOffline := &types.SandboxInstance{ID: "host-a", Status: "offline"}
	hostB := &types.SandboxInstance{ID: "host-b", Status: "online"}
	s.store.EXPECT().GetSandboxInstance(s.ctx, "host-a").Return(hostAOffline, nil)
	s.store.EXPECT().ListSandboxInstances(s.ctx).Return([]*types.SandboxInstance{hostAOffline, hostB}, nil)

	host, err := s.controller.pickHostForSandbox(s.ctx, sb, s.headlessSpec())
	s.Require().NoError(err)
	s.Require().Equal("host-b", host.ID, "non-persistent sandbox should reschedule onto a different online host")
}

// TestFirstTimePlacementHeadless — no HostDeviceID yet, picks any online
// host via ListSandboxInstances.
func (s *ControllerPlacementSuite) TestFirstTimePlacementHeadless() {
	sb := &types.Sandbox{ID: "sbx_new", Persistent: true}
	hostB := &types.SandboxInstance{ID: "host-b", Status: "online"}
	s.store.EXPECT().ListSandboxInstances(s.ctx).Return(
		[]*types.SandboxInstance{
			{ID: "host-a", Status: "offline"},
			hostB,
		}, nil)

	host, err := s.controller.pickHostForSandbox(s.ctx, sb, s.headlessSpec())
	s.Require().NoError(err)
	s.Require().Equal("host-b", host.ID)
}

// TestFirstTimePlacementDesktopRequiresVersionMatch — desktop runtimes go
// through FindAvailableSandboxInstance, which only returns hosts whose
// heartbeat advertises the right image version.
func (s *ControllerPlacementSuite) TestFirstTimePlacementDesktopRequiresVersionMatch() {
	sb := &types.Sandbox{ID: "sbx_new_desktop"}
	hostA := &types.SandboxInstance{ID: "host-a", Status: "online"}
	s.store.EXPECT().FindAvailableSandboxInstance(s.ctx, "ubuntu").Return(hostA, nil)

	host, err := s.controller.pickHostForSandbox(s.ctx, sb, s.desktopSpec())
	s.Require().NoError(err)
	s.Require().Equal("host-a", host.ID)
}

// TestStickyDesktopHostDroppedVersion — sticky placement for desktop has an
// extra constraint: the previously-bound host must still advertise the
// required image. If the operator removed it from that host, we must NOT
// re-bind (the container can't actually start), but we must NOT silently
// move either when persistent (data-loss). Surface the error.
func (s *ControllerPlacementSuite) TestStickyDesktopHostDroppedVersion() {
	versions, _ := json.Marshal(map[string]string{"sway": "abc123"}) // missing "ubuntu"
	sb := &types.Sandbox{
		ID:           "sbx_desktop_persistent",
		Persistent:   true,
		HostDeviceID: "host-a",
	}
	hostA := &types.SandboxInstance{
		ID:              "host-a",
		Status:          "online",
		DesktopVersions: versions,
	}
	s.store.EXPECT().GetSandboxInstance(s.ctx, "host-a").Return(hostA, nil)

	host, err := s.controller.pickHostForSandbox(s.ctx, sb, s.desktopSpec())
	s.Require().Error(err)
	s.Require().Nil(host)
	s.Require().Contains(err.Error(), "host-a")
	s.Require().Contains(err.Error(), "ubuntu", "error should name the missing version key")
}

// TestStickyDesktopHostStillAdvertisesVersion — happy path for desktop
// sticky: the host is online and still has the right image. Re-bind.
func (s *ControllerPlacementSuite) TestStickyDesktopHostStillAdvertisesVersion() {
	versions, _ := json.Marshal(map[string]string{"ubuntu": "abc123"})
	sb := &types.Sandbox{
		ID:           "sbx_desktop_sticky",
		Persistent:   true,
		HostDeviceID: "host-a",
	}
	hostA := &types.SandboxInstance{
		ID:              "host-a",
		Status:          "online",
		DesktopVersions: versions,
	}
	s.store.EXPECT().GetSandboxInstance(s.ctx, "host-a").Return(hostA, nil)

	host, err := s.controller.pickHostForSandbox(s.ctx, sb, s.desktopSpec())
	s.Require().NoError(err)
	s.Require().Equal("host-a", host.ID)
}

// TestNoHostsAvailable — first-time placement with no online hosts must
// return a clear error, not panic.
func (s *ControllerPlacementSuite) TestNoHostsAvailable() {
	sb := &types.Sandbox{ID: "sbx_orphan_first_time"}
	s.store.EXPECT().ListSandboxInstances(s.ctx).Return(
		[]*types.SandboxInstance{
			{ID: "host-a", Status: "offline"},
			{ID: "host-b", Status: "offline"},
		}, nil)

	host, err := s.controller.pickHostForSandbox(s.ctx, sb, s.headlessSpec())
	s.Require().Error(err)
	s.Require().Nil(host)
}
