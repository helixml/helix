package external_agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"
)

// Resume, fork and design-review rebuild the agent from session state. The task
// remains authoritative for both its resource preset and immutable runtime.
func TestResolveSpecTaskLaunchConfigAppliesTaskPreset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	executor := newTestExecutor(mockStore)

	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_1").Return(&types.SpecTask{
		ID:                       "spt_1",
		SandboxResourceOverrides: &types.SandboxResourceOverrides{VCPUs: 8, MemoryMB: 16384},
	}, nil)

	agent := &types.DesktopAgent{SessionID: "ses_1", SpecTaskID: "spt_1"}
	require.NoError(t, executor.resolveSpecTaskLaunchConfig(context.Background(), agent))
	require.Equal(t, 8, agent.VCPUs)
	require.Equal(t, 16384, agent.MemoryMB)
}

// A legacy task with no explicit override resolves to the same default the
// task UI displays, so the billed size matches what the user is shown.
func TestResolveSpecTaskLaunchConfigFallsBackToTaskDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	executor := newTestExecutor(mockStore)

	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_legacy").Return(&types.SpecTask{ID: "spt_legacy"}, nil)

	agent := &types.DesktopAgent{SessionID: "ses_1", SpecTaskID: "spt_legacy"}
	require.NoError(t, executor.resolveSpecTaskLaunchConfig(context.Background(), agent))

	expected := types.EffectiveSpecTaskSandboxResources(nil)
	require.Equal(t, expected.VCPUs, agent.VCPUs)
	require.Equal(t, expected.MemoryMB, agent.MemoryMB)
}

// An explicit caller-supplied size wins. The task is still read because its
// runtime remains authoritative, but an in-flight resize is not undone.
func TestResolveSpecTaskLaunchConfigLeavesExplicitSizeAlone(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	executor := newTestExecutor(mockStore)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_1").Return(&types.SpecTask{ID: "spt_1"}, nil)

	agent := &types.DesktopAgent{SessionID: "ses_1", SpecTaskID: "spt_1", VCPUs: 1, MemoryMB: 2048}
	require.NoError(t, executor.resolveSpecTaskLaunchConfig(context.Background(), agent))
	require.Equal(t, 1, agent.VCPUs)
	require.Equal(t, 2048, agent.MemoryMB)
}

func TestResolveSpecTaskLaunchConfigIgnoresNonTaskDesktops(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	executor := newTestExecutor(mockStore)

	agent := &types.DesktopAgent{SessionID: "ses_1"}
	require.NoError(t, executor.resolveSpecTaskLaunchConfig(context.Background(), agent))
	require.Zero(t, agent.VCPUs)
	require.Zero(t, agent.MemoryMB)
}

func TestResolveSpecTaskLaunchConfigForcesHeadlessRuntime(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	executor := newTestExecutor(mockStore)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_headless").Return(&types.SpecTask{
		ID:             "spt_headless",
		SandboxRuntime: types.SandboxRuntimeHeadlessUbuntu,
	}, nil)

	agent := &types.DesktopAgent{SessionID: "ses_1", SpecTaskID: "spt_headless", DesktopType: "ubuntu"}
	require.NoError(t, executor.resolveSpecTaskLaunchConfig(context.Background(), agent))
	require.Equal(t, "headless", agent.DesktopType)
}

func TestGetContainerImageUsesUbuntuToolchainForHeadlessTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	executor := newTestExecutor(mockStore)
	mockStore.EXPECT().GetSandboxInstance(gomock.Any(), "runner-1").Return(&types.SandboxInstance{
		ID:              "runner-1",
		DesktopVersions: datatypes.JSON([]byte(`{"ubuntu":"image-123"}`)),
	}, nil)

	image, err := executor.getContainerImage(context.Background(), "headless", "runner-1", &types.DesktopAgent{})
	require.NoError(t, err)
	require.Equal(t, "helix-ubuntu:image-123", image)
}

func TestBuildEnvVarsForcesHeadlessStartup(t *testing.T) {
	executor := newTestExecutor(nil)
	executor.gpuVendor = "nvidia"
	env := executor.buildEnvVars(&types.DesktopAgent{
		SessionID: "ses_1",
		Env:       []string{"HELIX_HEADLESS=0"},
	}, "headless", "/workspace")

	require.Equal(t, 1, countEnvKey(env, "HELIX_HEADLESS"))
	require.Contains(t, env, "HELIX_HEADLESS=1")
	for _, entry := range env {
		require.False(t, strings.HasPrefix(entry, "GAMESCOPE_"), entry)
		require.False(t, strings.HasPrefix(entry, "GOW_REQUIRED_DEVICES="), entry)
		require.False(t, strings.HasPrefix(entry, "GST_DEBUG="), entry)
		require.False(t, strings.HasPrefix(entry, "NVIDIA_"), entry)
		require.False(t, strings.HasPrefix(entry, "ZED_ALLOW_EMULATED_GPU="), entry)
	}
}

// The control plane is the single author of the sandbox-facing Helix API URL:
// buildEnvVars must emit the canonical hydra.SandboxAPIProxyURL for every API/LLM
// env var, so hydra and the settings daemon never rewrite addresses.
func TestBuildEnvVarsEmitsCanonicalSandboxAPIURL(t *testing.T) {
	executor := newTestExecutor(nil)
	env := executor.buildEnvVars(&types.DesktopAgent{SessionID: "ses_1"}, "ubuntu", "/workspace")

	proxy := hydra.SandboxAPIProxyURL
	require.Contains(t, env, "HELIX_API_URL="+proxy)
	require.Contains(t, env, "HELIX_API_BASE_URL="+proxy)
	require.Contains(t, env, "ANTHROPIC_BASE_URL="+proxy)
	require.Contains(t, env, "OPENAI_BASE_URL="+proxy+"/v1")
	require.Contains(t, env, "ZED_HELIX_URL="+hydra.SandboxAPIProxyHostname+":18080")
	require.Contains(t, env, "ZED_HELIX_TLS=false")
}

func TestExternalAgentIsolation(t *testing.T) {
	require.Equal(t, containerIsolation{rootlessContainerEngine: true}, externalAgentIsolation("headless", false, false))
	for _, containerType := range []string{"ubuntu", "sway", "zorin", "xfce", "kde"} {
		require.Equal(t, containerIsolation{privileged: true}, externalAgentIsolation(containerType, false, false), containerType)
	}
	// Bot instances: unprivileged and engine-free, desktop or not. A
	// no-container-engine (browser) instance wins over desktopRootless.
	for _, containerType := range []string{"headless", "ubuntu"} {
		require.Equal(t, containerIsolation{browserSandbox: true}, externalAgentIsolation(containerType, true, false), containerType)
		require.Equal(t, containerIsolation{browserSandbox: true}, externalAgentIsolation(containerType, true, true), containerType)
	}
	// desktopRootless on: desktops go unprivileged via rootless Podman; headless
	// is unaffected (it is already rootless).
	for _, containerType := range []string{"ubuntu", "sway", "zorin", "xfce", "kde"} {
		require.Equal(t, containerIsolation{desktopRootless: true}, externalAgentIsolation(containerType, false, true), containerType)
	}
	require.Equal(t, containerIsolation{rootlessContainerEngine: true}, externalAgentIsolation("headless", false, true))
}

func TestValidateDesktopRootlessLaunch(t *testing.T) {
	require.NoError(t, validateDesktopRootlessLaunch("sway", "custom/image", true, false, false))
	require.NoError(t, validateDesktopRootlessLaunch("ubuntu", "", false, false, true))
	require.NoError(t, validateDesktopRootlessLaunch("headless", "custom/image", false, false, true))
	require.NoError(t, validateDesktopRootlessLaunch("sway", "custom/image", false, true, true))
	require.EqualError(t, validateDesktopRootlessLaunch("ubuntu", "", true, false, true), "desktop rootless mode does not support golden builds")
	require.EqualError(t, validateDesktopRootlessLaunch("ubuntu", "custom/image", false, false, true), "desktop rootless mode does not support custom images")
	for _, desktopType := range []string{"sway", "zorin", "xfce", "kde"} {
		require.EqualError(t, validateDesktopRootlessLaunch(desktopType, "", false, false, true),
			fmt.Sprintf("desktop rootless mode does not support desktop type %q", desktopType))
	}
}

func TestStartDesktopRejectsUnsupportedRootlessLaunchBeforeProvisioning(t *testing.T) {
	for _, test := range []struct {
		name  string
		agent *types.DesktopAgent
		err   string
	}{
		{"golden build", &types.DesktopAgent{SessionID: "ses_golden", GoldenBuild: true}, "desktop rootless mode does not support golden builds"},
		{"custom image", &types.DesktopAgent{SessionID: "ses_custom", DesktopType: "ubuntu", CustomImage: "custom/image"}, "desktop rootless mode does not support custom images"},
		{"sway", &types.DesktopAgent{SessionID: "ses_sway", DesktopType: "sway"}, `desktop rootless mode does not support desktop type "sway"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			mockStore.EXPECT().GetSession(gomock.Any(), test.agent.SessionID).
				Return(&types.Session{ID: test.agent.SessionID}, nil).Times(2)
			executor := newTestExecutor(mockStore)
			executor.desktopRootless = true

			_, err := executor.StartDesktop(context.Background(), test.agent)
			require.EqualError(t, err, test.err)
		})
	}
}

func TestStartDesktopReusesUnsupportedRunningSessionWithRootlessFlag(t *testing.T) {
	for _, agent := range []*types.DesktopAgent{
		{SessionID: "ses_sway", DesktopType: "sway"},
		{SessionID: "ses_custom", DesktopType: "ubuntu", CustomImage: "custom/image"},
		{SessionID: "ses_golden", DesktopType: "ubuntu", GoldenBuild: true},
	} {
		executor := newTestExecutor(nil)
		executor.desktopRootless = true
		executor.sessions[agent.SessionID] = &ZedSession{
			SessionID:   agent.SessionID,
			Status:      "running",
			ContainerID: "existing-container",
		}

		response, err := executor.StartDesktop(context.Background(), agent)
		require.NoError(t, err)
		require.Equal(t, "running", response.Status)
		require.Equal(t, "existing-container", response.DevContainerID)
	}
}

func TestBuildMountsUsesContainerEngineStorageForRuntime(t *testing.T) {
	executor := newTestExecutor(nil)
	agent := &types.DesktopAgent{SessionID: "ses_1"}

	headlessMounts := executor.buildMounts(agent, "/workspace/ses_1", "headless")
	require.Equal(t, "docker-data-ses_1", mountSourceForDestination(headlessMounts, "/home/retro/.local/share/containers"))
	require.Empty(t, mountSourceForDestination(headlessMounts, "/var/lib/docker"))

	desktopMounts := executor.buildMounts(agent, "/workspace/ses_1", "ubuntu")
	require.Equal(t, "docker-data-ses_1", mountSourceForDestination(desktopMounts, "/var/lib/docker"))
	require.Empty(t, mountSourceForDestination(desktopMounts, "/home/retro/.local/share/containers"))
	require.Equal(t, agentBinaryCacheDir, mountSourceForDestination(desktopMounts, "/opt/helix/agent-cache"))

	instance := &types.DesktopAgent{SessionID: "ses_1", OrgWorkerID: "b-broker", NoContainerEngine: true}
	for _, containerType := range []string{"headless", "ubuntu"} {
		mounts := executor.buildMounts(instance, "/workspace/ses_1", containerType)
		require.Empty(t, mountSourceForDestination(mounts, "/var/lib/docker"), containerType)
		require.Empty(t, mountSourceForDestination(mounts, "/home/retro/.local/share/containers"), containerType)
		require.Empty(t, mountSourceForDestination(mounts, "/opt/helix/agent-cache"), containerType)
	}

	mainBot := &types.DesktopAgent{SessionID: "ses_main", OrgWorkerID: "b-broker"}
	require.Empty(t, mountSourceForDestination(executor.buildMounts(mainBot, "/workspace/ses_main", "ubuntu"), "/opt/helix/agent-cache"))
}

func mountSourceForDestination(mounts []hydra.MountConfig, destination string) string {
	for _, item := range mounts {
		if item.Destination == destination {
			return item.Source
		}
	}
	return ""
}

func countEnvKey(env []string, key string) int {
	count := 0
	for _, entry := range env {
		if strings.HasPrefix(entry, key+"=") {
			count++
		}
	}
	return count
}

// Desktops with no cap (exploratory sessions, subscription logins) can use the
// whole host, so charging them a single core would undercharge the largest
// consumers. They bill at the standard desktop preset instead.
func TestDesktopBillingResourcesUsesStandardPresetWhenUncapped(t *testing.T) {
	standard := types.EffectiveSpecTaskSandboxResources(nil)

	vcpus, memoryMB := desktopBillingResources(&types.DesktopAgent{})
	require.Equal(t, standard.VCPUs, vcpus)
	require.Equal(t, standard.MemoryMB, memoryMB)

	vcpus, memoryMB = desktopBillingResources(&types.DesktopAgent{VCPUs: 8, MemoryMB: 16384})
	require.Equal(t, 8, vcpus)
	require.Equal(t, 16384, memoryMB)
}
