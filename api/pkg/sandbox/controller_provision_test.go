package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// fakeHydra implements hydraProvisionClient by recording every call. Tests
// inspect the captured requests after Create()/Delete() to assert the wire
// payload sent to hydra is correct (image, env, mounts, vcpus, etc.).
type fakeHydra struct {
	mu sync.Mutex

	hostID string

	createCalls []*hydra.CreateDevContainerRequest
	deleteCalls []string
	forgetCalls []string

	createResp *hydra.DevContainerResponse
	createErr  error
	deleteErr  error
	forgetErr  error
}

func (f *fakeHydra) CreateDevContainer(_ context.Context, req *hydra.CreateDevContainerRequest) (*hydra.DevContainerResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls = append(f.createCalls, req)
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.createResp != nil {
		return f.createResp, nil
	}
	return &hydra.DevContainerResponse{
		SessionID:   req.SessionID,
		ContainerID: "ctr_" + req.SessionID,
		Status:      hydra.DevContainerStatusRunning,
	}, nil
}

func (f *fakeHydra) DeleteDevContainer(_ context.Context, sessionID string) (*hydra.DevContainerResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls = append(f.deleteCalls, sessionID)
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &hydra.DevContainerResponse{SessionID: sessionID, Status: hydra.DevContainerStatusStopped}, nil
}

func (f *fakeHydra) ForgetSandboxOps(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forgetCalls = append(f.forgetCalls, sessionID)
	return f.forgetErr
}

func (f *fakeHydra) lastCreate() *hydra.CreateDevContainerRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.createCalls) == 0 {
		return nil
	}
	return f.createCalls[len(f.createCalls)-1]
}

func (f *fakeHydra) deletes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleteCalls...)
}

// envMapFromSlice flips KEY=value entries into a map for stable assertions
// (envSlice is built from a map, so order is undefined).
func envMapFromSlice(env []string) map[string]string {
	out := map[string]string{}
	for _, kv := range env {
		eq := strings.Index(kv, "=")
		if eq <= 0 {
			continue
		}
		out[kv[:eq]] = kv[eq+1:]
	}
	return out
}

// ProvisionSuite drives Controller.Create end-to-end with a fake hydra
// client and asserts the full CreateDevContainer payload — vcpus, memory,
// image, cmd, env, mounts. The goroutine launched by Create() is awaited
// via waitProvisions() so the assertions are deterministic.
type ProvisionSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	store      *store.MockStore
	controller *Controller
	hydra      *fakeHydra
	ctx        context.Context
}

func TestProvisionSuite(t *testing.T) {
	suite.Run(t, new(ProvisionSuite))
}

func (s *ProvisionSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.ctx = context.Background()

	runtimes, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:         "headless-ubuntu=ubuntu:22.04|sleep infinity,node22=node:22-bookworm-slim|tail -f /dev/null",
		DefaultRuntime:   "headless-ubuntu",
		AllowCustomImage: true,
	})
	s.Require().NoError(err)

	s.controller = New(s.store, nil, runtimes, "https://api.example.com", "/sandbox-host")

	s.hydra = &fakeHydra{}
	s.controller.newHydraClient = func(hostID string) hydraProvisionClient {
		s.hydra.hostID = hostID
		return s.hydra
	}
}

func (s *ProvisionSuite) TearDownTest() {
	s.ctrl.Finish()
}

// expectCreateOK wires up the store call sequence for a successful Create:
// limits + (no billing) + CreateSandbox row insert returning sb.
func (s *ProvisionSuite) expectCreateOK(orgID string, sb *types.Sandbox) {
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil)
	s.store.EXPECT().ListSandboxes(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, q *store.ListSandboxesQuery) ([]*types.Sandbox, error) {
			s.Require().Equal(orgID, q.OrganizationID)
			return nil, nil
		},
	)
	s.store.EXPECT().CreateSandbox(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *types.Sandbox) (*types.Sandbox, error) {
			// Simulate DB stamping ID + timestamps.
			in.ID = sb.ID
			in.CreatedAt = time.Now()
			return in, nil
		},
	)
}

// expectProvisionStoreCalls wires up the store sequence inside provision()
// for a successful run targeting host-a.
func (s *ProvisionSuite) expectProvisionStoreCalls(sb *types.Sandbox, host *types.SandboxInstance, hostListed bool) {
	s.store.EXPECT().GetSandbox(gomock.Any(), sb.ID).Return(sb, nil)
	if hostListed {
		s.store.EXPECT().ListSandboxInstances(gomock.Any()).Return([]*types.SandboxInstance{host}, nil)
	} else {
		// Heartbeat-versioned (desktop) path.
		s.store.EXPECT().FindAvailableSandboxInstance(gomock.Any(), gomock.Any()).Return(host, nil)
	}
	s.store.EXPECT().SetSandboxContainer(gomock.Any(), sb.ID, host.ID, gomock.Any()).Return(nil)
	s.store.EXPECT().SetSandboxStatus(gomock.Any(), sb.ID, types.SandboxStatusRunning, "").Return(nil)
}

// TestProvisionSendsExpectedHeadlessRequest is the headline test: it
// asserts every field on the CreateDevContainerRequest that the controller
// builds for a headless sandbox.
func (s *ProvisionSuite) TestProvisionSendsExpectedHeadlessRequest() {
	sbID := "sbx_provision_basic"
	sb := &types.Sandbox{
		ID:             sbID,
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
		Image:          "ubuntu:22.04",
		Status:         types.SandboxStatusPending,
		VCPUs:          4,
		MemoryMB:       8192,
		DisplayWidth:   DefaultDisplayWidth,
		DisplayHeight:  DefaultDisplayHeight,
		DisplayFPS:     DefaultDisplayFPS,
		Env:            mustMarshal(map[string]string{"FOO": "bar"}),
	}
	host := &types.SandboxInstance{ID: "host-a", Status: "online"}

	s.expectCreateOK("org_1", sb)
	s.expectProvisionStoreCalls(sb, host, true)

	_, err := s.controller.Create(s.ctx, "org_1", "user_1", &types.CreateSandboxRequest{
		Runtime:  types.SandboxRuntimeHeadlessUbuntu,
		VCPUs:    4,
		MemoryMB: 8192,
		Env:      map[string]string{"FOO": "bar"},
	})
	s.Require().NoError(err)
	s.controller.waitProvisions()

	req := s.hydra.lastCreate()
	s.Require().NotNil(req, "hydra CreateDevContainer should have been called")

	// Identity & routing.
	s.Require().Equal(sbID, req.SessionID)
	s.Require().Equal("host-a", s.hydra.hostID, "factory should be invoked with the chosen host id (the factory itself adds the hydra- prefix when dialling)")
	s.Require().Equal("sbx-provision_basic", req.ContainerName, "container name strips the sbx_ prefix and re-prefixes with sbx-")
	s.Require().Equal(req.ContainerName, req.Hostname, "hostname mirrors the container name")

	// Resources.
	s.Require().Equal(4, req.VCPUs)
	s.Require().Equal(8192, req.MemoryMB)

	// Image + entry.
	s.Require().Equal("ubuntu:22.04", req.Image)
	s.Require().Equal([]string{"/bin/sh", "-c"}, req.Entrypoint)
	s.Require().Equal([]string{"sleep infinity"}, req.Cmd)
	s.Require().True(req.SkipImageValidation, "headless runtimes use plain images so validation must be skipped")
	s.Require().Equal(hydra.DevContainerTypeHeadless, req.ContainerType)
	s.Require().False(req.Privileged, "headless containers run unprivileged")

	// Env: user vars are preserved AND helix injects identity vars.
	env := envMapFromSlice(req.Env)
	s.Require().Equal("bar", env["FOO"], "user-supplied env var must reach hydra")
	s.Require().Equal("1", env["HELIX_DISABLE_AGENT"], "sandboxes must boot without the Zed/Qwen agent")
	s.Require().Equal(sbID, env["HELIX_SANDBOX_ID"])
	s.Require().Equal(sbID, env["HELIX_SESSION_ID"])
	s.Require().Equal("user_1", env["HELIX_USER_ID"])
	s.Require().Equal("org_1", env["HELIX_ORGANIZATION_ID"])
	// Headless containers must NOT receive desktop-specific vars.
	s.Require().NotContains(env, "GAMESCOPE_WIDTH")
	s.Require().NotContains(env, "USER_API_TOKEN")
	s.Require().NotContains(env, "NVIDIA_VISIBLE_DEVICES")

	// Mounts: a single ephemeral workspace bind-mount at /home/retro/work.
	s.Require().Len(req.Mounts, 1)
	s.Require().Equal("/home/retro/work", req.Mounts[0].Destination)
	s.Require().Equal(filepath.Join("/sandbox-host", "ephem", sbID), req.Mounts[0].Source)
	s.Require().False(req.Mounts[0].ReadOnly)
	s.Require().Empty(req.Mounts[0].Type, "workspace mount is a bind, not a named volume")
}

func (s *ProvisionSuite) TestProvisionDeletesContainerWhenRowWasDeletedBeforeContainerPersist() {
	sbID := "sbx_deleted_before_container_persist"
	sb := &types.Sandbox{
		ID:             sbID,
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
		Image:          "ubuntu:22.04",
		Status:         types.SandboxStatusPending,
		VCPUs:          1,
		MemoryMB:       2048,
		DisplayWidth:   DefaultDisplayWidth,
		DisplayHeight:  DefaultDisplayHeight,
		DisplayFPS:     DefaultDisplayFPS,
	}
	host := &types.SandboxInstance{ID: "host-a", Status: "online"}

	s.store.EXPECT().GetSandbox(gomock.Any(), sbID).Return(sb, nil)
	s.store.EXPECT().ListSandboxInstances(gomock.Any()).Return([]*types.SandboxInstance{host}, nil)
	s.store.EXPECT().SetSandboxContainer(gomock.Any(), sbID, host.ID, gomock.Any()).Return(store.ErrNotFound)

	s.controller.provision(s.ctx, sbID)

	s.Require().NotNil(s.hydra.lastCreate())
	s.Require().Equal([]string{sbID}, s.hydra.deletes(), "a container created after a concurrent delete must be torn down")
}

func (s *ProvisionSuite) TestProvisionDeletesContainerWhenRowWasDeletedBeforeStatusPersist() {
	sbID := "sbx_deleted_before_status_persist"
	sb := &types.Sandbox{
		ID:             sbID,
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
		Image:          "ubuntu:22.04",
		Status:         types.SandboxStatusPending,
		VCPUs:          1,
		MemoryMB:       2048,
		DisplayWidth:   DefaultDisplayWidth,
		DisplayHeight:  DefaultDisplayHeight,
		DisplayFPS:     DefaultDisplayFPS,
	}
	host := &types.SandboxInstance{ID: "host-a", Status: "online"}

	s.store.EXPECT().GetSandbox(gomock.Any(), sbID).Return(sb, nil)
	s.store.EXPECT().ListSandboxInstances(gomock.Any()).Return([]*types.SandboxInstance{host}, nil)
	s.store.EXPECT().SetSandboxContainer(gomock.Any(), sbID, host.ID, gomock.Any()).Return(nil)
	s.store.EXPECT().SetSandboxStatus(gomock.Any(), sbID, types.SandboxStatusRunning, "").Return(store.ErrNotFound)

	s.controller.provision(s.ctx, sbID)

	s.Require().NotNil(s.hydra.lastCreate())
	s.Require().Equal([]string{sbID}, s.hydra.deletes(), "a container whose status cannot be recorded must be torn down")
}

// TestProvisionPersistentUsesPersistSubdir checks that persistent sandboxes
// are bind-mounted under /sandbox-host/persist/{id} so reaping the container
// does not delete user data.
func (s *ProvisionSuite) TestProvisionPersistentUsesPersistSubdir() {
	sbID := "sbx_persist_x"
	sb := &types.Sandbox{
		ID:             sbID,
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
		Image:          "ubuntu:22.04",
		VCPUs:          1,
		MemoryMB:       2048,
		Persistent:     true,
		Env:            mustMarshal(map[string]string{}),
	}
	host := &types.SandboxInstance{ID: "host-a", Status: "online"}

	s.expectCreateOK("org_1", sb)
	s.expectProvisionStoreCalls(sb, host, true)

	_, err := s.controller.Create(s.ctx, "org_1", "user_1", &types.CreateSandboxRequest{
		Runtime:    types.SandboxRuntimeHeadlessUbuntu,
		Persistent: true,
	})
	s.Require().NoError(err)
	s.controller.waitProvisions()

	req := s.hydra.lastCreate()
	s.Require().NotNil(req)
	s.Require().Len(req.Mounts, 1)
	s.Require().Equal(
		filepath.Join("/sandbox-host", "persist", sbID),
		req.Mounts[0].Source,
		"persistent sandboxes must be mounted under persist/, not ephem/",
	)
}

// TestProvisionDesktopBuildsFullEnvAndMounts: desktop runtime requires a
// suite of GNOME/NVIDIA env vars and a docker-data volume + pipewire +
// crash-dump bind mounts. Without these the GNOME shell exits during init.
func (s *ProvisionSuite) TestProvisionDesktopBuildsFullEnvAndMounts() {
	sbID := "sbx_desktop_x"
	sb := &types.Sandbox{
		ID:             sbID,
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		Status:         types.SandboxStatusPending,
		VCPUs:          4,
		MemoryMB:       8192,
		DisplayWidth:   2560,
		DisplayHeight:  1440,
		DisplayFPS:     60,
		Env:            mustMarshal(map[string]string{}),
	}
	versions := mustMarshal(map[string]string{"ubuntu": "abc123"})
	host := &types.SandboxInstance{ID: "host-a", Status: "online", DesktopVersions: versions}

	s.expectCreateOK("org_1", sb)
	s.expectProvisionStoreCalls(sb, host, false) // versioned -> FindAvailable path
	// Desktop runtime mints an API token.
	s.store.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
	s.store.EXPECT().CreateAPIKey(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, k *types.ApiKey) (*types.ApiKey, error) {
			k.Key = "minted-token"
			return k, nil
		},
	)

	_, err := s.controller.Create(s.ctx, "org_1", "user_1", &types.CreateSandboxRequest{
		Runtime:       types.SandboxRuntimeUbuntuDesktop,
		VCPUs:         4,
		MemoryMB:      8192,
		DisplayWidth:  2560,
		DisplayHeight: 1440,
		DisplayFPS:    60,
	})
	s.Require().NoError(err)
	s.controller.waitProvisions()

	req := s.hydra.lastCreate()
	s.Require().NotNil(req)

	// Image is heartbeat-resolved against the host's desktop_versions blob.
	s.Require().Equal("helix-ubuntu:abc123", req.Image)
	s.Require().False(req.SkipImageValidation, "versioned helix-ubuntu image must go through validation")
	s.Require().Equal(hydra.DevContainerTypeUbuntu, req.ContainerType)
	s.Require().True(req.Privileged, "desktop runtime requires /dev access -> privileged")

	// Display passes through verbatim.
	s.Require().Equal(2560, req.DisplayWidth)
	s.Require().Equal(1440, req.DisplayHeight)
	s.Require().Equal(60, req.DisplayFPS)

	env := envMapFromSlice(req.Env)
	s.Require().Equal("ubuntu", env["HELIX_DESKTOP_TYPE"])
	s.Require().Equal("2560", env["GAMESCOPE_WIDTH"])
	s.Require().Equal("1440", env["GAMESCOPE_HEIGHT"])
	s.Require().Equal("60", env["GAMESCOPE_REFRESH"])
	s.Require().Equal("/run/user/1000", env["XDG_RUNTIME_DIR"])
	s.Require().Equal("/home/retro/work", env["WORKSPACE_DIR"])
	s.Require().Equal("https://api.example.com", env["HELIX_API_URL"])
	s.Require().Equal("https://api.example.com", env["HELIX_API_BASE_URL"])
	s.Require().Equal("all", env["NVIDIA_VISIBLE_DEVICES"])
	s.Require().Equal("compute,utility,video,graphics,display", env["NVIDIA_DRIVER_CAPABILITIES"])
	// Token suite minted by ensureSandboxAPIToken.
	s.Require().Equal("minted-token", env["USER_API_TOKEN"])
	s.Require().Equal("minted-token", env["ANTHROPIC_API_KEY"])
	s.Require().Equal("minted-token", env["OPENAI_API_KEY"])
	s.Require().Equal("minted-token", env["ZED_HELIX_TOKEN"])

	// Mounts: workspace + docker-data volume + pipewire + crash-dumps.
	mountByDest := map[string]hydra.MountConfig{}
	for _, m := range req.Mounts {
		mountByDest[m.Destination] = m
	}
	s.Require().Contains(mountByDest, "/home/retro/work")
	dockerData, ok := mountByDest["/var/lib/docker"]
	s.Require().True(ok, "desktop runtime must mount /var/lib/docker as a named volume")
	s.Require().Equal("volume", dockerData.Type, "docker-data must be a named volume, not a bind")
	s.Require().Equal(fmt.Sprintf("docker-data-%s", sbID), dockerData.Source)

	pipewire, ok := mountByDest["/run/user/1000"]
	s.Require().True(ok, "PipeWire socket dir must be bind-mounted")
	s.Require().Equal(filepath.Join("/sandbox-host", "runtime", sbID, "pipewire"), pipewire.Source)

	cores, ok := mountByDest["/tmp/cores"]
	s.Require().True(ok, "crash-dumps dir must be bind-mounted")
	s.Require().Equal(filepath.Join("/sandbox-host", "runtime", sbID, "crash-dumps"), cores.Source)
}

// TestProvisionMarksSandboxFailedOnHydraError: when hydra rejects
// CreateDevContainer the controller must persist status=failed with the
// error reason instead of leaving the row stuck in pending.
func (s *ProvisionSuite) TestProvisionMarksSandboxFailedOnHydraError() {
	sbID := "sbx_fail_provision"
	sb := &types.Sandbox{
		ID:             sbID,
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
		Image:          "ubuntu:22.04",
		VCPUs:          1,
		MemoryMB:       2048,
		Env:            mustMarshal(map[string]string{}),
	}
	host := &types.SandboxInstance{ID: "host-a", Status: "online"}

	s.expectCreateOK("org_1", sb)
	s.store.EXPECT().GetSandbox(gomock.Any(), sbID).Return(sb, nil)
	s.store.EXPECT().ListSandboxInstances(gomock.Any()).Return([]*types.SandboxInstance{host}, nil)

	failedSet := make(chan struct{})
	s.store.EXPECT().SetSandboxStatus(gomock.Any(), sbID, types.SandboxStatusFailed, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, status types.SandboxStatus, msg string) error {
			s.Require().Equal(types.SandboxStatusFailed, status)
			s.Require().Contains(msg, "hydra create")
			s.Require().Contains(msg, "boom")
			close(failedSet)
			return nil
		},
	)

	s.hydra.createErr = errors.New("boom")
	_, err := s.controller.Create(s.ctx, "org_1", "user_1", &types.CreateSandboxRequest{
		Runtime: types.SandboxRuntimeHeadlessUbuntu,
	})
	s.Require().NoError(err)
	s.controller.waitProvisions()

	select {
	case <-failedSet:
	default:
		s.Fail("expected SetSandboxStatus(failed) to be called")
	}
}

// TestProvisionMarksFailedWhenNoHostAvailable: placement failure should
// surface as status=failed with a clear reason and must NOT call hydra.
func (s *ProvisionSuite) TestProvisionMarksFailedWhenNoHostAvailable() {
	sbID := "sbx_no_host"
	sb := &types.Sandbox{
		ID:             sbID,
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
		Image:          "ubuntu:22.04",
		Env:            mustMarshal(map[string]string{}),
	}

	s.expectCreateOK("org_1", sb)
	s.store.EXPECT().GetSandbox(gomock.Any(), sbID).Return(sb, nil)
	s.store.EXPECT().ListSandboxInstances(gomock.Any()).Return(
		[]*types.SandboxInstance{{ID: "h", Status: "offline"}}, nil)

	s.store.EXPECT().SetSandboxStatus(gomock.Any(), sbID, types.SandboxStatusFailed, gomock.Any()).Return(nil)

	_, err := s.controller.Create(s.ctx, "org_1", "user_1", &types.CreateSandboxRequest{
		Runtime: types.SandboxRuntimeHeadlessUbuntu,
	})
	s.Require().NoError(err)
	s.controller.waitProvisions()

	s.Require().Empty(s.hydra.createCalls, "hydra CreateDevContainer must not be called when no host is available")
}

// TestDeleteCallsHydraDeleteAndForgetOps: Delete must tear down the hydra
// container AND forget the cached command ops, in that order, before
// soft-deleting the row.
func (s *ProvisionSuite) TestDeleteCallsHydraDeleteAndForgetOps() {
	sb := &types.Sandbox{
		ID:             "sbx_delete_path",
		OrganizationID: "org_1",
		Owner:          "user_1",
		HostDeviceID:   "host-z",
		Status:         types.SandboxStatusPending, // not running -> billing skipped
	}
	s.store.EXPECT().GetSandbox(s.ctx, sb.ID).Return(sb, nil)
	s.store.EXPECT().SetSandboxStatus(s.ctx, sb.ID, types.SandboxStatusStopping, "").Return(nil)
	s.store.EXPECT().GetAPIKey(s.ctx, gomock.Any()).Return(nil, store.ErrNotFound)
	s.store.EXPECT().DeleteSandbox(s.ctx, sb.ID).Return(nil)

	s.Require().NoError(s.controller.Delete(s.ctx, sb.ID))
	s.Require().Equal([]string{sb.ID}, s.hydra.deleteCalls)
	s.Require().Equal([]string{sb.ID}, s.hydra.forgetCalls)
	s.Require().Equal("host-z", s.hydra.hostID, "Delete should route to the host that owns the container")
}

// TestDeleteContinuesWhenHydraDeleteFails: hydra DeleteDevContainer is
// best-effort. A failure there must NOT prevent the row from being deleted —
// otherwise an orphaned container on a dead host would block forever.
func (s *ProvisionSuite) TestDeleteContinuesWhenHydraDeleteFails() {
	sb := &types.Sandbox{
		ID:             "sbx_hydra_dead",
		OrganizationID: "org_1",
		Owner:          "user_1",
		HostDeviceID:   "host-z",
		Status:         types.SandboxStatusPending,
	}
	s.store.EXPECT().GetSandbox(s.ctx, sb.ID).Return(sb, nil)
	s.store.EXPECT().SetSandboxStatus(s.ctx, sb.ID, types.SandboxStatusStopping, "").Return(nil)
	s.store.EXPECT().GetAPIKey(s.ctx, gomock.Any()).Return(nil, store.ErrNotFound)
	s.store.EXPECT().DeleteSandbox(s.ctx, sb.ID).Return(nil)

	s.hydra.deleteErr = errors.New("hydra unreachable")
	s.Require().NoError(s.controller.Delete(s.ctx, sb.ID))
}

// TestDeleteSkipsHydraWhenSandboxHasNoHost: a sandbox whose provision step
// failed before host binding has no container to tear down. Delete must not
// call hydra in that case.
func (s *ProvisionSuite) TestDeleteSkipsHydraWhenSandboxHasNoHost() {
	sb := &types.Sandbox{
		ID:             "sbx_orphan",
		OrganizationID: "org_1",
		Owner:          "user_1",
		Status:         types.SandboxStatusFailed,
	}
	s.store.EXPECT().GetSandbox(s.ctx, sb.ID).Return(sb, nil)
	s.store.EXPECT().SetSandboxStatus(s.ctx, sb.ID, types.SandboxStatusStopping, "").Return(nil)
	s.store.EXPECT().GetAPIKey(s.ctx, gomock.Any()).Return(nil, store.ErrNotFound)
	s.store.EXPECT().DeleteSandbox(s.ctx, sb.ID).Return(nil)

	s.Require().NoError(s.controller.Delete(s.ctx, sb.ID))
	s.Require().Empty(s.hydra.deleteCalls, "Delete must not contact hydra for sandboxes without a host")
}

// TestStartReaperRunsThenStopsOnContextCancel: the reaper goroutine must
// shut down cleanly when its context is cancelled. We validate by giving
// it a tight tick interval so at least one iteration runs, then cancel.
func (s *ProvisionSuite) TestStartReaperRunsThenStopsOnContextCancel() {
	tickerInterval := 10 * time.Millisecond

	// Each iteration calls all three reapers. We want at least one full
	// tick to complete before the context is cancelled — match each call
	// AnyTimes to tolerate the inherent race with the cancel.
	s.store.EXPECT().ListExpiredSandboxes(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	s.store.EXPECT().ListStoppedNonPersistentSandboxes(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	ctx, cancel := context.WithCancel(s.ctx)
	done := make(chan struct{})
	go func() {
		s.controller.StartReaper(ctx, tickerInterval)
		close(done)
	}()

	// Let the ticker fire a couple of times.
	time.Sleep(40 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		s.Fail("StartReaper did not return within 1s after context cancellation")
	}
}

// TestStartReaperDefaultsToOneMinuteWhenZero: passing 0 must not panic with
// a zero-duration ticker. We can't easily observe the default value, so we
// just assert the goroutine survives a short window without crashing and
// shuts down on cancel.
func (s *ProvisionSuite) TestStartReaperDefaultsToOneMinuteWhenZero() {
	ctx, cancel := context.WithCancel(s.ctx)
	done := make(chan struct{})
	go func() {
		s.controller.StartReaper(ctx, 0)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		s.Fail("StartReaper(0) must still respect ctx cancellation")
	}
}

// TestProvisionRecordsContainerIDFromHydraResponse: the host/container ids
// returned by hydra must be persisted via SetSandboxContainer so subsequent
// API calls can route through the correct host.
func (s *ProvisionSuite) TestProvisionRecordsContainerIDFromHydraResponse() {
	sbID := "sbx_record_ids"
	sb := &types.Sandbox{
		ID:             sbID,
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
		Image:          "ubuntu:22.04",
		Env:            mustMarshal(map[string]string{}),
	}
	host := &types.SandboxInstance{ID: "host-recorded", Status: "online"}

	s.expectCreateOK("org_1", sb)
	s.store.EXPECT().GetSandbox(gomock.Any(), sbID).Return(sb, nil)
	s.store.EXPECT().ListSandboxInstances(gomock.Any()).Return([]*types.SandboxInstance{host}, nil)
	gotContainer := make(chan string, 1)
	s.store.EXPECT().SetSandboxContainer(gomock.Any(), sbID, "host-recorded", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ string, containerID string) error {
			gotContainer <- containerID
			return nil
		},
	)
	s.store.EXPECT().SetSandboxStatus(gomock.Any(), sbID, types.SandboxStatusRunning, "").Return(nil)

	s.hydra.createResp = &hydra.DevContainerResponse{
		SessionID:   sbID,
		ContainerID: "ctr_explicit",
		Status:      hydra.DevContainerStatusRunning,
	}

	_, err := s.controller.Create(s.ctx, "org_1", "user_1", &types.CreateSandboxRequest{
		Runtime: types.SandboxRuntimeHeadlessUbuntu,
	})
	s.Require().NoError(err)
	s.controller.waitProvisions()

	select {
	case got := <-gotContainer:
		s.Require().Equal("ctr_explicit", got)
	default:
		s.Fail("SetSandboxContainer should have received the container id from the hydra response")
	}
}

// TestProvisionUsesPendingStatusWhenHydraNotYetRunning: if hydra reports
// the container is still starting, the sandbox row stays pending instead
// of jumping straight to running.
func (s *ProvisionSuite) TestProvisionUsesPendingStatusWhenHydraNotYetRunning() {
	sbID := "sbx_pending_status"
	sb := &types.Sandbox{
		ID:             sbID,
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
		Image:          "ubuntu:22.04",
		Env:            mustMarshal(map[string]string{}),
	}
	host := &types.SandboxInstance{ID: "host-a", Status: "online"}

	s.expectCreateOK("org_1", sb)
	s.store.EXPECT().GetSandbox(gomock.Any(), sbID).Return(sb, nil)
	s.store.EXPECT().ListSandboxInstances(gomock.Any()).Return([]*types.SandboxInstance{host}, nil)
	s.store.EXPECT().SetSandboxContainer(gomock.Any(), sbID, "host-a", gomock.Any()).Return(nil)
	s.store.EXPECT().SetSandboxStatus(gomock.Any(), sbID, types.SandboxStatusPending, "").Return(nil)

	s.hydra.createResp = &hydra.DevContainerResponse{
		SessionID:   sbID,
		ContainerID: "ctr_x",
		Status:      hydra.DevContainerStatusStarting,
	}

	_, err := s.controller.Create(s.ctx, "org_1", "user_1", &types.CreateSandboxRequest{
		Runtime: types.SandboxRuntimeHeadlessUbuntu,
	})
	s.Require().NoError(err)
	s.controller.waitProvisions()
}

// TestProvisionCustomImageRoutesThroughCustomSpec: when AllowCustomImage is
// on and the request supplies an explicit image, the resulting hydra
// request must carry that image with skip_image_validation=true.
func (s *ProvisionSuite) TestProvisionCustomImageRoutesThroughCustomSpec() {
	sbID := "sbx_custom_image"
	sb := &types.Sandbox{
		ID:             sbID,
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        "custom",
		Image:          "alpine:3.19",
		Env:            mustMarshal(map[string]string{}),
	}
	host := &types.SandboxInstance{ID: "host-a", Status: "online"}

	s.expectCreateOK("org_1", sb)
	s.expectProvisionStoreCalls(sb, host, true)

	_, err := s.controller.Create(s.ctx, "org_1", "user_1", &types.CreateSandboxRequest{
		Image: "alpine:3.19",
	})
	s.Require().NoError(err)
	s.controller.waitProvisions()

	req := s.hydra.lastCreate()
	s.Require().NotNil(req)
	s.Require().Equal("alpine:3.19", req.Image)
	s.Require().True(req.SkipImageValidation)
	s.Require().Equal([]string{"tail -f /dev/null"}, req.Cmd, "custom-image specs use the generic keep-alive cmd")
}

// TestSpecForSandboxRecoversCustomFromImage: provision() reconstructs an
// ad-hoc spec for stored custom-image rows so re-provisioning still works
// after a server restart (the original request is gone).
func TestSpecForSandboxRecoversCustomFromImage(t *testing.T) {
	r, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "headless-ubuntu=ubuntu:22.04",
		DefaultRuntime: "headless-ubuntu",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	c := &Controller{runtimes: r}

	spec, err := c.specForSandbox(&types.Sandbox{Runtime: "custom", Image: "alpine:3.19"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Name != "custom" || spec.Image != "alpine:3.19" {
		t.Fatalf("got %+v", spec)
	}
	if spec.ContainerType != hydra.DevContainerTypeHeadless {
		t.Fatalf("custom spec must default to headless container type, got %q", spec.ContainerType)
	}
}

// TestSpecForSandboxRejectsCustomWithEmptyImage: the recovery path needs
// an image to know what to launch — a row with Runtime=custom but no Image
// is corrupt and must error.
func TestSpecForSandboxRejectsCustomWithEmptyImage(t *testing.T) {
	c := &Controller{}
	_, err := c.specForSandbox(&types.Sandbox{Runtime: "custom", Image: ""})
	if err == nil {
		t.Fatal("expected error when custom row has no image")
	}
}

// TestSpecForSandboxResolvesRegisteredRuntime: the common case — a row
// whose Runtime is in the registry returns the registered spec verbatim.
func TestSpecForSandboxResolvesRegisteredRuntime(t *testing.T) {
	r, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "headless-ubuntu=ubuntu:22.04|sleep infinity",
		DefaultRuntime: "headless-ubuntu",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	c := &Controller{runtimes: r}
	spec, err := c.specForSandbox(&types.Sandbox{Runtime: types.SandboxRuntimeHeadlessUbuntu})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Image != "ubuntu:22.04" {
		t.Fatalf("got Image=%q, want ubuntu:22.04", spec.Image)
	}
}

// TestBuildMountsHeadlessOmitsDesktopMounts is a direct unit test of
// buildMounts to lock in the headless = workspace-only invariant.
func (s *ProvisionSuite) TestBuildMountsHeadlessOmitsDesktopMounts() {
	mounts := s.controller.buildMounts(
		&types.Sandbox{ID: "sbx_only_workspace", Persistent: false},
		&RuntimeSpec{Name: "headless-ubuntu", ContainerType: hydra.DevContainerTypeHeadless},
	)
	s.Require().Len(mounts, 1)
	s.Require().Equal("/home/retro/work", mounts[0].Destination)
}

// TestBuildMountsDesktopIncludesPipewireDockerAndCrashDumps mirrors the
// desktop-runtime expectations in TestProvisionDesktopBuildsFullEnvAndMounts
// but tests buildMounts() in isolation.
func (s *ProvisionSuite) TestBuildMountsDesktopIncludesPipewireDockerAndCrashDumps() {
	mounts := s.controller.buildMounts(
		&types.Sandbox{ID: "sbx_full_desktop", Persistent: false},
		&RuntimeSpec{Name: "ubuntu-desktop", ContainerType: hydra.DevContainerTypeUbuntu},
	)
	dest := map[string]hydra.MountConfig{}
	for _, m := range mounts {
		dest[m.Destination] = m
	}
	s.Require().Contains(dest, "/home/retro/work")
	s.Require().Contains(dest, "/var/lib/docker")
	s.Require().Equal("volume", dest["/var/lib/docker"].Type)
	s.Require().Contains(dest, "/run/user/1000")
	s.Require().Contains(dest, "/tmp/cores")
}

// TestBuildMountsWebServiceAddsDataDir locks in the durable web-service data
// mount: a Purpose=web-service sandbox gets /data bind-mounted from a
// PROJECT-keyed host path (so it survives across deploys/sandboxes), and
// nothing else does.
func (s *ProvisionSuite) TestBuildMountsWebServiceAddsDataDir() {
	mounts := s.controller.buildMounts(
		&types.Sandbox{ID: "sbx_ws", ProjectID: "prj_123", Persistent: true, Purpose: types.SandboxPurposeWebService},
		&RuntimeSpec{Name: "headless-ubuntu", ContainerType: hydra.DevContainerTypeHeadless},
	)
	dest := map[string]hydra.MountConfig{}
	for _, m := range mounts {
		dest[m.Destination] = m
	}
	s.Require().Contains(dest, "/data")
	// Keyed by PROJECT, not sandbox ID — that is what makes it survive redeploys.
	s.Require().Equal("/sandbox-host/webservice/prj_123/data", dest["/data"].Source)
	s.Require().False(dest["/data"].ReadOnly)
	s.Require().Contains(dest, "/home/retro/work")
}

// TestBuildMountsNonWebServiceOmitsDataDir — ordinary sandboxes never get the
// /data mount, even when persistent.
func (s *ProvisionSuite) TestBuildMountsNonWebServiceOmitsDataDir() {
	mounts := s.controller.buildMounts(
		&types.Sandbox{ID: "sbx_plain", ProjectID: "prj_123", Persistent: true},
		&RuntimeSpec{Name: "headless-ubuntu", ContainerType: hydra.DevContainerTypeHeadless},
	)
	for _, m := range mounts {
		s.Require().NotEqual("/data", m.Destination, "non-web-service sandbox must not get /data")
	}
}

// Sanity: round-trip an envMap through json so the test helpers used above
// don't drift silently from the JSON representation the controller persists.
func (s *ProvisionSuite) TestEnvMapJSONRoundTrip() {
	in := map[string]string{"FOO": "bar", "BAZ": "qux"}
	b, err := json.Marshal(in)
	s.Require().NoError(err)
	var out map[string]string
	s.Require().NoError(json.Unmarshal(b, &out))
	s.Require().Equal(in, out)
}
