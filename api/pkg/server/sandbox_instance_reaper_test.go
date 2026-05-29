package server

import (
	"context"
	"errors"
	"sync/atomic"
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

	// Two online rows expected to be flipped, in either order — gomock's
	// default declaration-order matching is brittle if the reaper's
	// iteration ever changes. AnyOrder makes the assertion robust to that.
	gomock.InOrder() // no ordering constraint
	updates := []*gomock.Call{
		mockStore.EXPECT().
			MarkSandboxInstanceOfflineIfStale(gomock.Any(), "online-stale", gomock.Any()).
			Return(int64(1), nil),
		mockStore.EXPECT().
			MarkSandboxInstanceOfflineIfStale(gomock.Any(), "online-fresh-mislabeled", gomock.Any()).
			Return(int64(1), nil),
	}
	// Suppress unused if gomock changes default behaviour; InAnyOrder is
	// the idiomatic gomock construct for "expected, order not asserted".
	_ = updates

	server.reapStaleSandboxInstances(context.Background(), 5*time.Minute)
}

func TestReapStaleSandboxInstances_HeartbeatRaceLost(t *testing.T) {
	// Simulates: reaper SELECTs a stale row, but between SELECT and the
	// conditional UPDATE a heartbeat lands, refreshing last_seen. The CAS
	// UPDATE matches no rows and returns 0. Reaper must treat that as a
	// non-transition, not an error.
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	stale := []*types.SandboxInstance{
		{ID: "raced", Status: "online", LastSeen: time.Now().Add(-10 * time.Minute)},
	}
	mockStore.EXPECT().
		GetSandboxInstancesOlderThanHeartbeat(gomock.Any(), gomock.Any()).
		Return(stale, nil)
	mockStore.EXPECT().
		MarkSandboxInstanceOfflineIfStale(gomock.Any(), "raced", gomock.Any()).
		Return(int64(0), nil) // CAS lost to a concurrent heartbeat.

	server.reapStaleSandboxInstances(context.Background(), 5*time.Minute)
}

func TestReapStaleSandboxInstances_QueryErrorDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	mockStore.EXPECT().
		GetSandboxInstancesOlderThanHeartbeat(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("transient db error"))

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
		MarkSandboxInstanceOfflineIfStale(gomock.Any(), "fails", gomock.Any()).
		Return(int64(0), errors.New("db lock"))
	mockStore.EXPECT().
		MarkSandboxInstanceOfflineIfStale(gomock.Any(), "succeeds", gomock.Any()).
		Return(int64(1), nil)

	server.reapStaleSandboxInstances(context.Background(), 5*time.Minute)
}

func TestReapStaleSandboxInstances_StopsOnContextCancelMidSweep(t *testing.T) {
	// 100 stale rows; we cancel the context during the sweep. The reaper
	// must bail out via the ctx.Err() guard at the top of the loop instead
	// of issuing all 100 UPDATEs.
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	stale := make([]*types.SandboxInstance, 100)
	for i := range stale {
		stale[i] = &types.SandboxInstance{
			ID:       "row-" + string(rune('A'+i)),
			Status:   "online",
			LastSeen: time.Now().Add(-10 * time.Minute),
		}
	}

	mockStore.EXPECT().
		GetSandboxInstancesOlderThanHeartbeat(gomock.Any(), gomock.Any()).
		Return(stale, nil)

	ctx, cancel := context.WithCancel(context.Background())

	var calls atomic.Int32
	// Mark up to 5 succeed, then cancel mid-sweep. AnyTimes() because we
	// don't care exactly how many fire before cancellation lands — only
	// that fewer than `len(stale)` do.
	mockStore.EXPECT().
		MarkSandboxInstanceOfflineIfStale(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, _ string, _ time.Time) (int64, error) {
			n := calls.Add(1)
			if n == 5 {
				cancel()
			}
			return 1, nil
		})

	server.reapStaleSandboxInstances(ctx, 5*time.Minute)

	got := int(calls.Load())
	if got >= len(stale) {
		t.Fatalf("expected sweep to bail early; saw %d/%d calls", got, len(stale))
	}
}

func TestStartSandboxInstanceReaper_TickerFiresAndStopsOnCancel(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	ctx, cancel := context.WithCancel(context.Background())

	queriedCh := make(chan struct{}, 4)
	mockStore.EXPECT().
		GetSandboxInstancesOlderThanHeartbeat(gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, _ time.Time) ([]*types.SandboxInstance, error) {
			select {
			case queriedCh <- struct{}{}:
			default:
			}
			return nil, nil
		})

	done := make(chan struct{})
	go func() {
		server.startSandboxInstanceReaper(ctx, 50*time.Millisecond, 5*time.Minute)
		close(done)
	}()

	// Assert the ticker actually fired at least once before we cancel —
	// the previous test passed against an empty loop body because no such
	// assertion existed.
	select {
	case <-queriedCh:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("reaper ticker never fired in 2s")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reaper did not exit within 1s of context cancellation")
	}
}
