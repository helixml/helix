package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"
)

// HostReadyForReplacement decides whether it is safe to DESTROY a container in
// order to rebuild it. Getting it wrong in the permissive direction is how a
// host memory hiccup on code.helix.ml turned into a ~28h customer outage: the
// runner was undispatchable, the replacement could not be placed, and a
// container that was actually serving had already been deleted. So these tests
// lean on the refusals.
type HostReadyForReplacementSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	store      *store.MockStore
	controller *Controller
	ctx        context.Context
}

func TestHostReadyForReplacementSuite(t *testing.T) {
	suite.Run(t, new(HostReadyForReplacementSuite))
}

func (s *HostReadyForReplacementSuite) SetupTest() {
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

func (s *HostReadyForReplacementSuite) TearDownTest() { s.ctrl.Finish() }

// desktopSandbox is the shape a web-service sandbox has: desktop runtime
// (Docker-capable image), persistent, pinned to a host.
func desktopSandbox() *types.Sandbox {
	return &types.Sandbox{
		ID:           "sbx_web",
		Runtime:      types.SandboxRuntimeUbuntuDesktop,
		Persistent:   true,
		HostDeviceID: "code-for-app",
	}
}

func onlineDesktopHost() *types.SandboxInstance {
	versions, _ := json.Marshal(map[string]string{"ubuntu": "2.12.23-linux-amd64"})
	return &types.SandboxInstance{
		ID:              "code-for-app",
		Status:          "online",
		LastSeen:        time.Now(),
		GPUVendor:       "nvidia",
		DesktopVersions: datatypes.JSON(versions),
	}
}

func (s *HostReadyForReplacementSuite) TestReadyWhenHostOnlineAndAdvertising() {
	s.store.EXPECT().GetSandboxInstance(s.ctx, "code-for-app").Return(onlineDesktopHost(), nil)
	ready, why := s.controller.HostReadyForReplacement(s.ctx, desktopSandbox())
	s.True(ready)
	s.Empty(why)
}

// The 2026-09-27 outage: the reaper had flipped the runner to offline, so the
// replacement could never be placed. Refusing here keeps the existing
// container (and its restart=unless-stopped app) alive to recover on its own.
func (s *HostReadyForReplacementSuite) TestRefusesWhenHostOffline() {
	host := onlineDesktopHost()
	host.Status = "offline"
	s.store.EXPECT().GetSandboxInstance(s.ctx, "code-for-app").Return(host, nil)
	ready, why := s.controller.HostReadyForReplacement(s.ctx, desktopSandbox())
	s.False(ready)
	s.Contains(why, "offline")
}

// The dangerous middle window: the row still says "online" because the reaper
// only flips at 5m, but the host has already missed enough heartbeats to be
// undispatchable (90s). A recreate decided here would fail at placement.
func (s *HostReadyForReplacementSuite) TestRefusesWhenHeartbeatStaleButStillMarkedOnline() {
	host := onlineDesktopHost()
	host.LastSeen = time.Now().Add(-3 * time.Minute)
	s.store.EXPECT().GetSandboxInstance(s.ctx, "code-for-app").Return(host, nil)
	ready, why := s.controller.HostReadyForReplacement(s.ctx, desktopSandbox())
	s.False(ready)
	s.Contains(why, "heartbeat")
}

// A heartbeat that is merely late (one missed beat) must NOT block a genuine
// recreate — the container really can be rebuilt on this host.
func (s *HostReadyForReplacementSuite) TestReadyWhenHeartbeatOnlySlightlyLate() {
	host := onlineDesktopHost()
	host.LastSeen = time.Now().Add(-config.DefaultSandboxDispatchStaleThreshold / 2)
	s.store.EXPECT().GetSandboxInstance(s.ctx, "code-for-app").Return(host, nil)
	ready, why := s.controller.HostReadyForReplacement(s.ctx, desktopSandbox())
	s.True(ready, why)
}

// Fail CLOSED: "I could not read the host row" must never authorise a delete.
func (s *HostReadyForReplacementSuite) TestRefusesWhenHostLookupErrors() {
	s.store.EXPECT().GetSandboxInstance(s.ctx, "code-for-app").Return(nil, errors.New("connection refused"))
	ready, why := s.controller.HostReadyForReplacement(s.ctx, desktopSandbox())
	s.False(ready)
	s.Contains(why, "could not be looked up")
}

func (s *HostReadyForReplacementSuite) TestRefusesWhenHostStoppedAdvertisingTheImage() {
	host := onlineDesktopHost()
	host.DesktopVersions = nil
	s.store.EXPECT().GetSandboxInstance(s.ctx, "code-for-app").Return(host, nil)
	ready, why := s.controller.HostReadyForReplacement(s.ctx, desktopSandbox())
	s.False(ready)
	s.Contains(why, "advertises")
}

// The desktop runtime requires a render node; a CPU-only host cannot take it.
func (s *HostReadyForReplacementSuite) TestRefusesWhenHostCannotHostDesktop() {
	host := onlineDesktopHost()
	host.GPUVendor = "none"
	s.store.EXPECT().GetSandboxInstance(s.ctx, "code-for-app").Return(host, nil)
	ready, why := s.controller.HostReadyForReplacement(s.ctx, desktopSandbox())
	s.False(ready)
	s.Contains(why, "cannot host runtime")
}

// A headless runtime has no display requirement, so the same CPU-only host is
// a perfectly good home for it — the display gate must not leak across.
func (s *HostReadyForReplacementSuite) TestHeadlessIgnoresDisplayCapability() {
	host := onlineDesktopHost()
	host.GPUVendor = "none"
	host.DesktopVersions = nil // headless spec has no VersionKey either
	s.store.EXPECT().GetSandboxInstance(s.ctx, "code-for-app").Return(host, nil)
	sb := desktopSandbox()
	sb.Runtime = "headless-ubuntu"
	ready, why := s.controller.HostReadyForReplacement(s.ctx, sb)
	s.True(ready, why)
}

// Never-placed sandbox: nothing to strand, so the normal scheduler decides.
func (s *HostReadyForReplacementSuite) TestUnplacedSandboxIsReady() {
	sb := desktopSandbox()
	sb.HostDeviceID = ""
	ready, why := s.controller.HostReadyForReplacement(s.ctx, sb)
	s.True(ready, why)
}

func (s *HostReadyForReplacementSuite) TestNilSandboxRefused() {
	ready, why := s.controller.HostReadyForReplacement(s.ctx, nil)
	s.False(ready)
	s.NotEmpty(why)
}
