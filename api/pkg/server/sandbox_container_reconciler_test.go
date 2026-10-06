package server

import (
	"context"
	"errors"
	"testing"
	"time"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// Only online sandboxes are swept. An offline sandbox has no RevDial connection,
// so discovery would fail for every session on it and teach us nothing; its
// sessions are reconciled by the existing on-reconnect discovery instead.
func TestReconcileSandboxContainers_OnlySweepsOnlineSandboxes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockExecutor := external_agent.NewMockExecutor(ctrl)
	server := &HelixAPIServer{Store: mockStore, externalAgentExecutor: mockExecutor}

	mockStore.EXPECT().
		ListSandboxInstances(gomock.Any()).
		Return([]*types.SandboxInstance{
			{ID: "sbox_online", Status: "online"},
			{ID: "sbox_offline", Status: "offline"},
			{ID: "local", Status: "online"},
		}, nil)

	swept := map[string]bool{}
	mockExecutor.EXPECT().
		DiscoverContainersFromSandbox(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, sandboxID string) error {
			swept[sandboxID] = true
			return nil
		}).
		Times(2)

	server.reconcileSandboxContainers(context.Background())

	assert.True(t, swept["sbox_online"], "online sandbox should be swept")
	assert.True(t, swept["local"], "the single-node local sandbox should be swept too")
	assert.False(t, swept["sbox_offline"], "offline sandbox should be skipped")
}

// One sandbox failing must not abort the sweep — a wedged RevDial connection on
// sandbox A cannot be allowed to leave sandbox B's dead sessions unreconciled.
func TestReconcileSandboxContainers_ContinuesAfterDiscoveryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockExecutor := external_agent.NewMockExecutor(ctrl)
	server := &HelixAPIServer{Store: mockStore, externalAgentExecutor: mockExecutor}

	mockStore.EXPECT().
		ListSandboxInstances(gomock.Any()).
		Return([]*types.SandboxInstance{
			{ID: "sbox_broken", Status: "online"},
			{ID: "sbox_ok", Status: "online"},
		}, nil)

	mockExecutor.EXPECT().
		DiscoverContainersFromSandbox(gomock.Any(), "sbox_broken").
		Return(errors.New("revdial: connection closed"))
	reachedSecond := false
	mockExecutor.EXPECT().
		DiscoverContainersFromSandbox(gomock.Any(), "sbox_ok").
		DoAndReturn(func(_ context.Context, _ string) error {
			reachedSecond = true
			return nil
		})

	server.reconcileSandboxContainers(context.Background())

	assert.True(t, reachedSecond, "sweep must continue past a failing sandbox")
}

// Each sandbox gets a bounded context so a hung RevDial call cannot stall the
// whole sweep indefinitely.
func TestReconcileSandboxContainers_BoundsEachSandbox(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockExecutor := external_agent.NewMockExecutor(ctrl)
	server := &HelixAPIServer{Store: mockStore, externalAgentExecutor: mockExecutor}

	mockStore.EXPECT().
		ListSandboxInstances(gomock.Any()).
		Return([]*types.SandboxInstance{{ID: "sbox_1", Status: "online"}}, nil)

	mockExecutor.EXPECT().
		DiscoverContainersFromSandbox(gomock.Any(), "sbox_1").
		DoAndReturn(func(ctx context.Context, _ string) error {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok, "per-sandbox context must carry a deadline")
			assert.LessOrEqual(t, time.Until(deadline), perSandboxReconcileTimeout)
			return nil
		})

	server.reconcileSandboxContainers(context.Background())
}

// A store failure must be swallowed so the ticker survives a transient DB hiccup.
func TestReconcileSandboxContainers_StoreErrorDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockExecutor := external_agent.NewMockExecutor(ctrl)
	server := &HelixAPIServer{Store: mockStore, externalAgentExecutor: mockExecutor}

	mockStore.EXPECT().
		ListSandboxInstances(gomock.Any()).
		Return(nil, errors.New("connection refused"))

	server.reconcileSandboxContainers(context.Background())
}

// A cancelled context stops the sweep rather than working through the remaining
// instances during shutdown.
func TestReconcileSandboxContainers_StopsOnCancelledContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockExecutor := external_agent.NewMockExecutor(ctrl)
	server := &HelixAPIServer{Store: mockStore, externalAgentExecutor: mockExecutor}

	ctx, cancel := context.WithCancel(context.Background())

	mockStore.EXPECT().
		ListSandboxInstances(gomock.Any()).
		DoAndReturn(func(_ context.Context) ([]*types.SandboxInstance, error) {
			cancel()
			return []*types.SandboxInstance{
				{ID: "sbox_1", Status: "online"},
				{ID: "sbox_2", Status: "online"},
			}, nil
		})

	// No DiscoverContainersFromSandbox expectations: the cancelled context must
	// short-circuit before the first sandbox.
	server.reconcileSandboxContainers(ctx)
}
