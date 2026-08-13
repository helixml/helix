package memorystore

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestSpecTaskThreadTracking(t *testing.T) {
	ctx := context.Background()
	store := New()
	workSession := &types.SpecTaskWorkSession{
		SpecTaskID:     "task-1",
		HelixSessionID: "session-1",
		Phase:          types.SpecTaskPhaseImplementation,
	}
	if err := store.CreateSpecTaskWorkSession(ctx, workSession); err != nil {
		t.Fatal(err)
	}
	if workSession.ID == "" || workSession.Status != types.SpecTaskWorkSessionStatusPending {
		t.Fatalf("work session defaults not applied: %#v", workSession)
	}
	zedThread := &types.SpecTaskZedThread{
		WorkSessionID: workSession.ID,
		SpecTaskID:    "task-1",
		ZedThreadID:   "thread-1",
	}
	if err := store.CreateSpecTaskZedThread(ctx, zedThread); err != nil {
		t.Fatal(err)
	}
	if zedThread.ID == "" || zedThread.Status != types.SpecTaskZedStatusPending {
		t.Fatalf("zed thread defaults not applied: %#v", zedThread)
	}

	phase := types.SpecTaskPhaseImplementation
	workSessions, err := store.ListWorkSessionsBySpecTask(ctx, "task-1", &phase)
	if err != nil || len(workSessions) != 1 || workSessions[0].ZedThread == nil {
		t.Fatalf("listed work sessions = %#v, err = %v", workSessions, err)
	}
	workSessions[0].ZedThread.ZedThreadID = "changed-copy"
	found, err := store.GetSpecTaskZedThreadByZedThreadID(ctx, "thread-1")
	if err != nil || found.WorkSession == nil || found.WorkSession.ID != workSession.ID {
		t.Fatalf("found thread = %#v, err = %v", found, err)
	}
	found.ZedThreadID = "thread-2"
	if err := store.UpdateSpecTaskZedThread(ctx, found); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSpecTaskZedThreadByZedThreadID(ctx, "thread-1"); err == nil {
		t.Fatal("old thread ID still resolves after update")
	}
	if updated, err := store.GetSpecTaskZedThreadByZedThreadID(ctx, "thread-2"); err != nil || updated.ID != zedThread.ID {
		t.Fatalf("updated thread = %#v, err = %v", updated, err)
	}
}
