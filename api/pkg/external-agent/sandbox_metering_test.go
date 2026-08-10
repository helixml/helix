package external_agent

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// Only the three spec_driven_task_service launch paths put resources on the
// agent. Resume, fork and design-review rebuild it from the session, so without
// this resolution a resumed 8 vCPU task would come back uncapped and be billed
// at the default preset.
func TestResolveSpecTaskResourcesAppliesTaskPreset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	executor := newTestExecutor(mockStore)

	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_1").Return(&types.SpecTask{
		ID:                       "spt_1",
		SandboxResourceOverrides: &types.SandboxResourceOverrides{VCPUs: 8, MemoryMB: 16384},
	}, nil)

	agent := &types.DesktopAgent{SessionID: "ses_1", SpecTaskID: "spt_1"}
	require.NoError(t, executor.resolveSpecTaskResources(context.Background(), agent))
	require.Equal(t, 8, agent.VCPUs)
	require.Equal(t, 16384, agent.MemoryMB)
}

// A legacy task with no explicit override resolves to the same default the
// task UI displays, so the billed size matches what the user is shown.
func TestResolveSpecTaskResourcesFallsBackToTaskDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	executor := newTestExecutor(mockStore)

	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_legacy").Return(&types.SpecTask{ID: "spt_legacy"}, nil)

	agent := &types.DesktopAgent{SessionID: "ses_1", SpecTaskID: "spt_legacy"}
	require.NoError(t, executor.resolveSpecTaskResources(context.Background(), agent))

	expected := types.EffectiveSpecTaskSandboxResources(nil)
	require.Equal(t, expected.VCPUs, agent.VCPUs)
	require.Equal(t, expected.MemoryMB, agent.MemoryMB)
}

// An explicit caller-supplied size wins — it already knows what it wants, and
// re-reading the task would undo an in-flight resize.
func TestResolveSpecTaskResourcesLeavesExplicitSizeAlone(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	executor := newTestExecutor(mockStore)

	agent := &types.DesktopAgent{SessionID: "ses_1", SpecTaskID: "spt_1", VCPUs: 1, MemoryMB: 2048}
	require.NoError(t, executor.resolveSpecTaskResources(context.Background(), agent))
	require.Equal(t, 1, agent.VCPUs)
	require.Equal(t, 2048, agent.MemoryMB)
}

func TestResolveSpecTaskResourcesIgnoresNonTaskDesktops(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	executor := newTestExecutor(mockStore)

	agent := &types.DesktopAgent{SessionID: "ses_1"}
	require.NoError(t, executor.resolveSpecTaskResources(context.Background(), agent))
	require.Zero(t, agent.VCPUs)
	require.Zero(t, agent.MemoryMB)
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
