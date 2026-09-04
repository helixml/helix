package external_agent

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDesktopTypeRequiresDisplay(t *testing.T) {
	assert.False(t, DesktopTypeRequiresDisplay("headless"))
	assert.False(t, DesktopTypeRequiresDisplay("Headless"))
	assert.True(t, DesktopTypeRequiresDisplay("ubuntu"))
	assert.True(t, DesktopTypeRequiresDisplay("sway"))
	assert.True(t, DesktopTypeRequiresDisplay("")) // default desktop
}

func TestValidateSandboxHostPinEmptyIsNoop(t *testing.T) {
	mockStore := store.NewMockStore(gomock.NewController(t))
	require.NoError(t, ValidateSandboxHostPin(context.Background(), mockStore, "", true))
}

func TestValidateSandboxHostPinUnknownHost(t *testing.T) {
	mockStore := store.NewMockStore(gomock.NewController(t))
	mockStore.EXPECT().GetSandboxInstance(gomock.Any(), "ghost").Return(nil, errors.New("record not found"))

	err := ValidateSandboxHostPin(context.Background(), mockStore, "ghost", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestValidateSandboxHostPinOfflineHost(t *testing.T) {
	mockStore := store.NewMockStore(gomock.NewController(t))
	mockStore.EXPECT().GetSandboxInstance(gomock.Any(), "nas").
		Return(&types.SandboxInstance{ID: "nas", Status: "offline"}, nil)

	err := ValidateSandboxHostPin(context.Background(), mockStore, "nas", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not online")
}

func TestValidateSandboxHostPinDesktopOnCPUOnlyHost(t *testing.T) {
	mockStore := store.NewMockStore(gomock.NewController(t))
	mockStore.EXPECT().GetSandboxInstance(gomock.Any(), "nas").
		Return(&types.SandboxInstance{ID: "nas", Status: "online", GPUVendor: "none"}, nil)

	err := ValidateSandboxHostPin(context.Background(), mockStore, "nas", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot run a streamed desktop")
}

func TestValidateSandboxHostPinHeadlessOnCPUOnlyHost(t *testing.T) {
	mockStore := store.NewMockStore(gomock.NewController(t))
	mockStore.EXPECT().GetSandboxInstance(gomock.Any(), "nas").
		Return(&types.SandboxInstance{ID: "nas", Status: "online", GPUVendor: "none"}, nil)

	require.NoError(t, ValidateSandboxHostPin(context.Background(), mockStore, "nas", false))
}

func TestValidateSandboxHostPinDesktopOnGPUHost(t *testing.T) {
	mockStore := store.NewMockStore(gomock.NewController(t))
	mockStore.EXPECT().GetSandboxInstance(gomock.Any(), "omen").
		Return(&types.SandboxInstance{ID: "omen", Status: "online", GPUVendor: "nvidia"}, nil)

	require.NoError(t, ValidateSandboxHostPin(context.Background(), mockStore, "omen", true))
}
