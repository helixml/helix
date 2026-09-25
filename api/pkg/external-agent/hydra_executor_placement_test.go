package external_agent

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestResolveSandboxIDHonoursExplicitPin(t *testing.T) {
	mockStore := store.NewMockStore(gomock.NewController(t))
	h := &HydraExecutor{store: mockStore}

	id, err := h.resolveSandboxID(context.Background(), &types.DesktopAgent{SandboxID: "omen"}, "ubuntu")
	require.NoError(t, err)
	assert.Equal(t, "omen", id)
}

func TestResolveSandboxIDAutoSelectsAvailableHost(t *testing.T) {
	mockStore := store.NewMockStore(gomock.NewController(t))
	mockStore.EXPECT().
		FindAvailableSandboxInstance(gomock.Any(), "ubuntu", true).
		Return(&types.SandboxInstance{ID: "host-1"}, nil)
	h := &HydraExecutor{store: mockStore}

	id, err := h.resolveSandboxID(context.Background(), &types.DesktopAgent{}, "ubuntu")
	require.NoError(t, err)
	assert.Equal(t, "host-1", id)
}

func TestResolveSandboxIDHeadlessPlacesOnUbuntuImageWithoutDisplay(t *testing.T) {
	mockStore := store.NewMockStore(gomock.NewController(t))
	// Headless containers use the helix-ubuntu toolchain image but need no
	// display-capable host.
	mockStore.EXPECT().
		FindAvailableSandboxInstance(gomock.Any(), "ubuntu", false).
		Return(&types.SandboxInstance{ID: "nas"}, nil)
	h := &HydraExecutor{store: mockStore}

	id, err := h.resolveSandboxID(context.Background(), &types.DesktopAgent{}, "headless")
	require.NoError(t, err)
	assert.Equal(t, "nas", id)
}

func TestResolveSandboxIDErrorsWhenFleetHasNoCapacity(t *testing.T) {
	mockStore := store.NewMockStore(gomock.NewController(t))
	mockStore.EXPECT().
		FindAvailableSandboxInstance(gomock.Any(), "ubuntu", true).
		Return(nil, nil)
	mockStore.EXPECT().
		ListSandboxInstances(gomock.Any()).
		Return([]*types.SandboxInstance{{ID: "full-host"}}, nil)
	h := &HydraExecutor{store: mockStore}

	_, err := h.resolveSandboxID(context.Background(), &types.DesktopAgent{}, "ubuntu")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no sandbox host has capacity")
}

func TestResolveSandboxIDFallsBackToLocalOnEmptyRegistry(t *testing.T) {
	mockStore := store.NewMockStore(gomock.NewController(t))
	mockStore.EXPECT().
		FindAvailableSandboxInstance(gomock.Any(), "ubuntu", true).
		Return(nil, nil)
	mockStore.EXPECT().
		ListSandboxInstances(gomock.Any()).
		Return(nil, nil)
	h := &HydraExecutor{store: mockStore}

	id, err := h.resolveSandboxID(context.Background(), &types.DesktopAgent{}, "ubuntu")
	require.NoError(t, err)
	assert.Equal(t, "local", id)
}
