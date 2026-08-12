package services

import (
	"context"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

// recordingSink captures the events handed to the AttentionService sink.
type recordingSink struct {
	mu     sync.Mutex
	events []*types.AttentionEvent
}

func (r *recordingSink) PublishAttentionEvent(_ context.Context, ev *types.AttentionEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return nil
}

func (r *recordingSink) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

// expectSlackDrain stubs the async notifySlack path (ListApps) so the
// fire-and-forget goroutine can't make an unexpected mock call, and
// returns a channel closed once it has run — letting the test wait for
// the goroutine before the controller tears down.
func expectSlackDrain(mockStore *store.MockStore) <-chan struct{} {
	done := make(chan struct{})
	mockStore.EXPECT().ListApps(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *store.ListAppsQuery) ([]*types.App, error) {
			close(done)
			return []*types.App{}, nil
		}).AnyTimes()
	return done
}

// TestAttentionService_PublishesToSink pins that a freshly-created
// attention event is forwarded to the sink (the org topic publisher).
func TestAttentionService_PublishesToSink(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)

	task := &types.SpecTask{ID: "task_1", ProjectID: "prj_1", Name: "x"}
	ctx := context.Background()
	mockStore.EXPECT().GetProject(ctx, "prj_1").Return(&types.Project{ID: "prj_1", UserID: "user_1"}, nil)
	mockStore.EXPECT().CreateAttentionEvent(ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, ev *types.AttentionEvent) (*types.AttentionEvent, error) {
			return ev, nil // not deduped: returns the same event
		})
	slackDone := expectSlackDrain(mockStore)

	sink := &recordingSink{}
	svc := NewAttentionService(mockStore, nil)
	svc.SetEventSink(sink)

	if _, err := svc.EmitEvent(ctx, types.AttentionEventPRReady, task, "", nil); err != nil {
		t.Fatalf("EmitEvent: %v", err)
	}
	<-slackDone
	if sink.count() != 1 {
		t.Errorf("sink received %d events, want 1", sink.count())
	}
}

// TestAttentionService_SkipsSinkOnDedup pins that the deduplicated path
// (CreateAttentionEvent returns a different/existing row) does NOT publish
// — mirroring how the existing Slack notify is skipped on dedup.
func TestAttentionService_SkipsSinkOnDedup(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)

	task := &types.SpecTask{ID: "task_1", ProjectID: "prj_1", Name: "x"}
	ctx := context.Background()
	mockStore.EXPECT().GetProject(ctx, "prj_1").Return(&types.Project{ID: "prj_1", UserID: "user_1"}, nil)
	mockStore.EXPECT().CreateAttentionEvent(ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *types.AttentionEvent) (*types.AttentionEvent, error) {
			// Simulate dedup: return a pre-existing row with a different ID.
			return &types.AttentionEvent{ID: "existing_row"}, nil
		})

	sink := &recordingSink{}
	svc := NewAttentionService(mockStore, nil)
	svc.SetEventSink(sink)

	if _, err := svc.EmitEvent(ctx, types.AttentionEventPRReady, task, "", nil); err != nil {
		t.Fatalf("EmitEvent: %v", err)
	}
	if sink.count() != 0 {
		t.Errorf("sink received %d events on dedup, want 0", sink.count())
	}
}

// TestAttentionService_NilSinkSafe pins that EmitEvent works with no sink
// wired (the default for non-org deployments).
func TestAttentionService_NilSinkSafe(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)

	task := &types.SpecTask{ID: "task_1", ProjectID: "prj_1", Name: "x"}
	ctx := context.Background()
	mockStore.EXPECT().GetProject(ctx, "prj_1").Return(&types.Project{ID: "prj_1", UserID: "user_1"}, nil)
	mockStore.EXPECT().CreateAttentionEvent(ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, ev *types.AttentionEvent) (*types.AttentionEvent, error) {
			return ev, nil
		})
	slackDone := expectSlackDrain(mockStore)

	svc := NewAttentionService(mockStore, nil) // no sink
	if _, err := svc.EmitEvent(ctx, types.AttentionEventPRReady, task, "", nil); err != nil {
		t.Fatalf("EmitEvent with nil sink: %v", err)
	}
	<-slackDone
}
