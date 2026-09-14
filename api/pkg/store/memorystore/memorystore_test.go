package memorystore

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestInteractionQuestionLifecycle(t *testing.T) {
	ctx := context.Background()
	memory := New()
	interaction, err := memory.CreateInteraction(ctx, &types.Interaction{
		ID: "interaction-1", SessionID: "session-1", UserID: "user-1",
		GenerationID: 1, State: types.InteractionStateWaiting,
	})
	if err != nil {
		t.Fatal(err)
	}
	question := &types.PendingQuestion{
		RequestID: "question-1",
		Questions: []types.UserQuestion{{ID: "choice", Question: "Choose one"}},
	}
	updated, changed, err := memory.SetInteractionPendingQuestion(ctx, interaction.ID, 1, question)
	if err != nil || !changed || updated.PendingQuestion == nil {
		t.Fatalf("set pending question = %#v, changed = %v, err = %v", updated, changed, err)
	}

	// A stale full-row update must not erase state owned by the targeted
	// question methods.
	interaction.ResponseMessage = "still working"
	if _, err := memory.UpdateInteraction(ctx, interaction); err != nil {
		t.Fatal(err)
	}
	updated, err = memory.GetInteraction(ctx, interaction.ID)
	if err != nil || updated.PendingQuestion == nil {
		t.Fatalf("pending question lost after stale update: %#v, err = %v", updated, err)
	}

	updated, changed, err = memory.ResolveInteractionPendingQuestion(
		ctx, interaction.ID, 1, question.RequestID, "answered", map[string]string{"choice": "A"},
	)
	if err != nil || !changed || updated.PendingQuestion != nil || len(updated.QuestionHistory) != 1 {
		t.Fatalf("resolve pending question = %#v, changed = %v, err = %v", updated, changed, err)
	}
	updated, changed, err = memory.SetInteractionPendingQuestion(ctx, interaction.ID, 1, question)
	if err != nil || changed || updated.PendingQuestion != nil || len(updated.QuestionHistory) != 1 {
		t.Fatalf("resolved question was resurrected: %#v, changed = %v, err = %v", updated, changed, err)
	}
}

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
