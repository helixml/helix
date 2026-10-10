package memorystore

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
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

func TestInteractionCancellationLifecycleClearsIntent(t *testing.T) {
	tests := []struct {
		name       string
		state      types.InteractionState
		transition func(*MemoryStore, context.Context, string, int) (bool, error)
	}{
		{
			name:  "interrupted",
			state: types.InteractionStateInterrupted,
			transition: func(memory *MemoryStore, ctx context.Context, id string, generationID int) (bool, error) {
				return memory.MarkInteractionInterruptedIfWaiting(ctx, id, generationID)
			},
		},
		{
			name:  "complete",
			state: types.InteractionStateComplete,
			transition: func(memory *MemoryStore, ctx context.Context, id string, generationID int) (bool, error) {
				return memory.MarkInteractionCompleteIfWaiting(ctx, id, generationID)
			},
		},
		{
			name:  "error",
			state: types.InteractionStateError,
			transition: func(memory *MemoryStore, ctx context.Context, id string, generationID int) (bool, error) {
				return memory.MarkInteractionErrorIfWaiting(ctx, id, generationID, "test error")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			memory := New()
			interaction, err := memory.CreateInteraction(ctx, &types.Interaction{
				ID: "interaction-cancel", SessionID: "session-cancel", UserID: "user-1",
				GenerationID: 1, State: types.InteractionStateWaiting,
			})
			if err != nil {
				t.Fatal(err)
			}
			requested, err := memory.RequestInteractionCancellationIfWaiting(ctx, interaction.ID, interaction.GenerationID)
			if err != nil || !requested {
				t.Fatalf("request cancellation = requested %v, err %v", requested, err)
			}
			waiting, err := memory.GetInteraction(ctx, interaction.ID)
			if err != nil || waiting.ExternalAgentCancelRequestedAt == nil {
				t.Fatalf("cancellation intent was not persisted: %#v, err %v", waiting, err)
			}

			transitioned, err := test.transition(memory, ctx, interaction.ID, interaction.GenerationID)
			if err != nil || !transitioned {
				t.Fatalf("terminal transition = transitioned %v, err %v", transitioned, err)
			}
			final, err := memory.GetInteraction(ctx, interaction.ID)
			if err != nil {
				t.Fatal(err)
			}
			if final.State != test.state || final.ExternalAgentCancelRequestedAt != nil {
				t.Fatalf("terminal cancellation state retained intent: %#v", final)
			}
		})
	}
}

func TestUpdateInteractionTerminalStateClearsCancellationIntent(t *testing.T) {
	ctx := context.Background()
	for _, state := range []types.InteractionState{
		types.InteractionStateComplete,
		types.InteractionStateError,
		types.InteractionStateInterrupted,
	} {
		t.Run(string(state), func(t *testing.T) {
			memory := New()
			interaction, err := memory.CreateInteraction(ctx, &types.Interaction{
				ID: "interaction-generic-terminal", SessionID: "session-generic-terminal", UserID: "user-1",
				GenerationID: 1, State: types.InteractionStateWaiting,
			})
			if err != nil {
				t.Fatal(err)
			}
			requested, err := memory.RequestInteractionCancellationIfWaiting(ctx, interaction.ID, interaction.GenerationID)
			if err != nil || !requested {
				t.Fatalf("request cancellation = requested %v, err %v", requested, err)
			}

			interaction.State = state
			if _, err := memory.UpdateInteraction(ctx, interaction); err != nil {
				t.Fatal(err)
			}
			final, err := memory.GetInteraction(ctx, interaction.ID)
			if err != nil {
				t.Fatal(err)
			}
			if final.State != state || final.ExternalAgentCancelRequestedAt != nil {
				t.Fatalf("terminal update retained cancellation intent: %#v", final)
			}
		})
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

func TestGoldenBuilds(t *testing.T) {
	ctx := context.Background()
	m := New()

	builds, err := m.ListGoldenBuilds(ctx, &store.ListGoldenBuildsQuery{ProjectID: "prj_1"})
	if err != nil || len(builds) != 0 {
		t.Fatalf("empty store: builds=%v err=%v", builds, err)
	}
	if _, err := m.GetGoldenBuild(ctx, "prj_1", "sb_1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetGoldenBuild on missing row: err=%v, want ErrNotFound", err)
	}

	// update=false creates the row but leaves it at its defaults.
	row, err := m.UpdateGoldenBuild(ctx, "prj_1", "sb_1", func(*types.SandboxCacheState) bool { return false })
	if err != nil || row.Status != types.GoldenBuildStatusNone {
		t.Fatalf("seeded row: %+v err=%v", row, err)
	}
	row, err = m.UpdateGoldenBuild(ctx, "prj_1", "sb_1", func(s *types.SandboxCacheState) bool {
		s.Status = types.GoldenBuildStatusBuilding
		s.Attempt = 1
		return true
	})
	if err != nil || row.Status != types.GoldenBuildStatusBuilding || row.Attempt != 1 {
		t.Fatalf("updated row: %+v err=%v", row, err)
	}
	if _, err := m.UpdateGoldenBuild(ctx, "prj_2", "sb_1", func(s *types.SandboxCacheState) bool {
		s.Status = types.GoldenBuildStatusReady
		return true
	}); err != nil {
		t.Fatal(err)
	}

	active, _ := m.ListGoldenBuilds(ctx, &store.ListGoldenBuildsQuery{ActiveOnly: true})
	if len(active) != 1 || active[0].ProjectID != "prj_1" {
		t.Fatalf("active builds = %+v, want only prj_1", active)
	}
	onSandbox, _ := m.ListGoldenBuilds(ctx, &store.ListGoldenBuildsQuery{SandboxID: "sb_1"})
	if len(onSandbox) != 2 {
		t.Fatalf("builds on sb_1 = %d, want 2", len(onSandbox))
	}

	if err := m.DeleteGoldenBuilds(ctx, "prj_1"); err != nil {
		t.Fatal(err)
	}
	left, _ := m.ListGoldenBuilds(ctx, nil)
	if len(left) != 1 || left[0].ProjectID != "prj_2" {
		t.Fatalf("after delete = %+v, want only prj_2", left)
	}
}
