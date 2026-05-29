package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

func TestReapStaleSandboxInstances_FlipsOnlineRowsOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	now := time.Now()
	stale := []*types.SandboxInstance{
		{ID: "online-stale", Status: "online", LastSeen: now.Add(-10 * time.Minute)},
		{ID: "already-offline", Status: "offline", LastSeen: now.Add(-1 * time.Hour)},
		{ID: "online-fresh-mislabeled", Status: "online", LastSeen: now.Add(-10 * time.Minute)},
	}

	mockStore.EXPECT().
		GetSandboxInstancesOlderThanHeartbeat(gomock.Any(), gomock.Any()).
		Return(stale, nil)

	// Only the two online rows get updated; the already-offline row is
	// skipped (no UpdateSandboxInstanceStatus call for it).
	mockStore.EXPECT().
		UpdateSandboxInstanceStatus(gomock.Any(), "online-stale", "offline").
		Return(nil)
	mockStore.EXPECT().
		UpdateSandboxInstanceStatus(gomock.Any(), "online-fresh-mislabeled", "offline").
		Return(nil)

	server.reapStaleSandboxInstances(context.Background(), 5*time.Minute)
}

func TestReapStaleSandboxInstances_QueryErrorDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	mockStore.EXPECT().
		GetSandboxInstancesOlderThanHeartbeat(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("transient db error"))

	// No update calls expected; reaper should log and return.
	server.reapStaleSandboxInstances(context.Background(), 5*time.Minute)
}

func TestReapStaleSandboxInstances_UpdateErrorContinuesLoop(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	stale := []*types.SandboxInstance{
		{ID: "fails", Status: "online", LastSeen: time.Now().Add(-10 * time.Minute)},
		{ID: "succeeds", Status: "online", LastSeen: time.Now().Add(-10 * time.Minute)},
	}

	mockStore.EXPECT().
		GetSandboxInstancesOlderThanHeartbeat(gomock.Any(), gomock.Any()).
		Return(stale, nil)
	mockStore.EXPECT().
		UpdateSandboxInstanceStatus(gomock.Any(), "fails", "offline").
		Return(errors.New("db lock"))
	mockStore.EXPECT().
		UpdateSandboxInstanceStatus(gomock.Any(), "succeeds", "offline").
		Return(nil)

	server.reapStaleSandboxInstances(context.Background(), 5*time.Minute)
}

func TestStartSandboxInstanceReaper_StopsOnContextCancel(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	ctx, cancel := context.WithCancel(context.Background())

	// The reaper runs on a ticker; we expect zero queries because we
	// cancel immediately. Use AnyTimes to be lenient against race
	// (cancel happening just after the first tick).
	mockStore.EXPECT().
		GetSandboxInstancesOlderThanHeartbeat(gomock.Any(), gomock.Any()).
		AnyTimes().
		Return(nil, nil)

	done := make(chan struct{})
	go func() {
		server.startSandboxInstanceReaper(ctx, 50*time.Millisecond, 5*time.Minute)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reaper did not exit within 1s of context cancellation")
	}
}
