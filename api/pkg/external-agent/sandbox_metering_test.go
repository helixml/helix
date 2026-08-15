package external_agent

import (
	"context"
	"strings"
	"testing"

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
