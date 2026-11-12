package memorystore

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestRecoverProviderPromptClaimsSentPromptOnce(t *testing.T) {
	ctx := context.Background()
	memory := New()
	now := time.Now()
	_, err := memory.CreateSession(ctx, types.Session{ID: "session-1", Metadata: types.SessionMetadata{
		Recovery: &types.ProviderRecovery{PromptID: "prompt-1", InteractionID: "interaction-1", NextCheckAt: now.Add(-time.Second)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := memory.CreatePromptHistoryEntry(ctx, &types.PromptHistoryEntry{ID: "prompt-1", SessionID: "session-1", Status: "sent"}); err != nil {
		t.Fatal(err)
	}
	interaction, err := memory.CreateInteraction(ctx, &types.Interaction{ID: "interaction-1", SessionID: "session-1", State: types.InteractionStateError})
	if err != nil {
		t.Fatal(err)
	}

	claimed, err := memory.RecoverProviderPrompt(ctx, "session-1", "prompt-1", now, now.Add(time.Minute), 3)
	if err != nil || !claimed {
		t.Fatalf("first recovery: claimed=%v err=%v", claimed, err)
	}
	claimed, err = memory.RecoverProviderPrompt(ctx, "session-1", "prompt-1", now, now.Add(time.Minute), 3)
	if err != nil || claimed {
		t.Fatalf("duplicate recovery: claimed=%v err=%v", claimed, err)
	}

	interaction.State = types.InteractionStateComplete
	if _, err := memory.UpdateInteraction(ctx, interaction); err != nil {
		t.Fatal(err)
	}
	memory.setPromptStatus("prompt-1", "sent")
	claimed, err = memory.RecoverProviderPrompt(ctx, "session-1", "prompt-1", now.Add(2*time.Minute), now.Add(3*time.Minute), 3)
	if err != nil || claimed {
		t.Fatalf("completed interaction recovery: claimed=%v err=%v", claimed, err)
	}
}

func TestRecordSessionAgentConfigAppliedPreservesRecovery(t *testing.T) {
	ctx := context.Background()
	memory := New()
	newRecovery := &types.ProviderRecovery{PromptID: "prompt-new", InteractionID: "interaction-new"}
	_, err := memory.CreateSession(ctx, types.Session{ID: "session-1", Metadata: types.SessionMetadata{
		Recovery: newRecovery, ExternalAgentStatus: "running",
	}})
	if err != nil {
		t.Fatal(err)
	}
	appliedAt := time.Now()
	if err := memory.RecordSessionAgentConfigApplied(ctx, "session-1", appliedAt, true); err != nil {
		t.Fatal(err)
	}
	stored, err := memory.GetSession(ctx, "session-1")
	if err != nil || stored.Metadata.Recovery != newRecovery || stored.Metadata.ExternalAgentStatus != "running" {
		t.Fatalf("targeted update changed unrelated metadata: session=%#v err=%v", stored, err)
	}
	if !stored.Metadata.AgentConfigAppliedAt.Equal(appliedAt) || !stored.Metadata.AgentHandoffDeliveredAt.Equal(appliedAt) {
		t.Fatalf("timestamps not recorded: %#v", stored.Metadata)
	}

	secondAppliedAt := appliedAt.Add(time.Second)
	if err := memory.RecordSessionAgentConfigApplied(ctx, "session-1", secondAppliedAt, false); err != nil {
		t.Fatal(err)
	}
	stored, err = memory.GetSession(ctx, "session-1")
	if err != nil || !stored.Metadata.AgentConfigAppliedAt.Equal(secondAppliedAt) || !stored.Metadata.AgentHandoffDeliveredAt.Equal(appliedAt) {
		t.Fatalf("undelivered callback changed delivered timestamp: session=%#v err=%v", stored, err)
	}
}

func TestGetLatestActivationPromptHistoryEntryIncludesCancelled(t *testing.T) {
	ctx := context.Background()
	memory := New()
	now := time.Now()
	deletedAt := now
	entries := []*types.PromptHistoryEntry{
		{ID: "prompt_activation_old", SessionID: "session-1", Status: "sent", CreatedAt: now.Add(-time.Minute)},
		{ID: "prompt_activation_cancelled", SessionID: "session-1", Status: "sent", CreatedAt: now, DeletedAt: &deletedAt},
		{ID: "promptXactivation_lookalike", SessionID: "session-1", Status: "sent", CreatedAt: now.Add(time.Minute)},
	}
	for _, entry := range entries {
		if err := memory.CreatePromptHistoryEntry(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := memory.GetLatestActivationPromptHistoryEntry(ctx, "session-1")
	if err != nil || latest == nil || latest.ID != "prompt_activation_cancelled" || latest.DeletedAt == nil {
		t.Fatalf("latest activation = %#v, err = %v", latest, err)
	}
}

func TestRecoverProviderPromptForConfigChangeOnlyClaimsLatestUncancelledFailure(t *testing.T) {
	ctx := context.Background()
	memory := New()
	now := time.Now()
	_, err := memory.CreateSession(ctx, types.Session{ID: "session-1", Metadata: types.SessionMetadata{
		Recovery: &types.ProviderRecovery{PromptID: "old-prompt"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := memory.ClearSessionProviderRecovery(ctx, "session-1", "newer-prompt", "")
	if err != nil || cleared {
		t.Fatalf("mismatched recovery clear: cleared=%v err=%v", cleared, err)
	}
	unchangedSession, err := memory.GetSession(ctx, "session-1")
	if err != nil || unchangedSession.Metadata.Recovery == nil || unchangedSession.Metadata.Recovery.PromptID != "old-prompt" {
		t.Fatalf("mismatched clear changed session: session=%#v err=%v", unchangedSession, err)
	}
	if err := memory.CreatePromptHistoryEntry(ctx, &types.PromptHistoryEntry{ID: "old-prompt", SessionID: "session-1", Status: "sent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.CreateInteraction(ctx, &types.Interaction{
		ID: "old-error", SessionID: "session-1", PromptID: "old-prompt", Created: now, State: types.InteractionStateError,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.CreateInteraction(ctx, &types.Interaction{
		ID: "new-success", SessionID: "session-1", PromptID: "old-prompt", Created: now.Add(time.Second), State: types.InteractionStateComplete,
	}); err != nil {
		t.Fatal(err)
	}
	claimed, err := memory.RecoverProviderPromptForConfigChange(ctx, "session-1", "old-prompt", "old-error")
	if err != nil || claimed {
		t.Fatalf("old failure with newer success: claimed=%v err=%v", claimed, err)
	}

	deletedAt := now
	if err := memory.CreatePromptHistoryEntry(ctx, &types.PromptHistoryEntry{
		ID: "cancelled-prompt", SessionID: "session-1", Status: "sent", DeletedAt: &deletedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.CreateInteraction(ctx, &types.Interaction{
		ID: "cancelled-error", SessionID: "session-1", PromptID: "cancelled-prompt", State: types.InteractionStateError,
	}); err != nil {
		t.Fatal(err)
	}
	claimed, err = memory.RecoverProviderPromptForConfigChange(ctx, "session-1", "cancelled-prompt", "cancelled-error")
	if err != nil || claimed {
		t.Fatalf("cancelled failure: claimed=%v err=%v", claimed, err)
	}

	if err := memory.CreatePromptHistoryEntry(ctx, &types.PromptHistoryEntry{ID: "eligible-prompt", SessionID: "session-1", Status: "sent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.CreateInteraction(ctx, &types.Interaction{
		ID: "eligible-error", SessionID: "session-1", PromptID: "eligible-prompt", State: types.InteractionStateError,
	}); err != nil {
		t.Fatal(err)
	}
	claimed, err = memory.RecoverProviderPromptForConfigChange(ctx, "session-1", "eligible-prompt", "eligible-error")
	if err != nil || !claimed {
		t.Fatalf("eligible failure: claimed=%v err=%v", claimed, err)
	}
	session, err := memory.GetSession(ctx, "session-1")
	if err != nil || session.Metadata.Recovery == nil || session.Metadata.Recovery.PromptID != "old-prompt" {
		t.Fatalf("unrelated provider recovery changed: session=%#v err=%v", session, err)
	}
	prompt, err := memory.GetPromptHistoryEntry(ctx, "eligible-prompt")
	if err != nil || prompt.Status != "pending" {
		t.Fatalf("eligible prompt status=%q err=%v", prompt.Status, err)
	}
}

func TestListSessionsAnyOrganizationIsExplicit(t *testing.T) {
	ctx := context.Background()
	memory := New()
	provider := "pe-1"
	for _, session := range []types.Session{
		{ID: "personal", Metadata: types.SessionMetadata{Recovery: &types.ProviderRecovery{Provider: provider}}},
		{ID: "organization", OrganizationID: "org-1", Metadata: types.SessionMetadata{Recovery: &types.ProviderRecovery{Provider: provider}}},
	} {
		if _, err := memory.CreateSession(ctx, session); err != nil {
			t.Fatal(err)
		}
	}

	personal, _, err := memory.ListSessions(ctx, store.ListSessionsQuery{ProviderRecoveryProviders: []string{provider}})
	if err != nil || len(personal) != 1 || personal[0].ID != "personal" {
		t.Fatalf("personal sessions = %#v, err = %v", personal, err)
	}
	all, _, err := memory.ListSessions(ctx, store.ListSessionsQuery{AnyOrganization: true, ProviderRecoveryProviders: []string{provider}})
	if err != nil || len(all) != 2 {
		t.Fatalf("all sessions = %#v, err = %v", all, err)
	}
}

func TestListInteractionsFiltersPendingQuestions(t *testing.T) {
	ctx := context.Background()
	memory := New()
	for _, interaction := range []*types.Interaction{
		{ID: "interaction-complete", SessionID: "session-1", UserID: "user-1", GenerationID: 1, State: types.InteractionStateComplete},
		{ID: "interaction-resolved", SessionID: "session-1", UserID: "user-1", GenerationID: 1, State: types.InteractionStateWaiting},
		{ID: "interaction-question", SessionID: "session-1", UserID: "user-1", GenerationID: 1, State: types.InteractionStateWaiting},
	} {
		if _, err := memory.CreateInteraction(ctx, interaction); err != nil {
			t.Fatal(err)
		}
	}
	if _, changed, err := memory.SetInteractionPendingQuestion(ctx, "interaction-question", 1, &types.PendingQuestion{
		RequestID: "question-1", Questions: []types.UserQuestion{{ID: "choice", Question: "Choose one"}},
	}); err != nil || !changed {
		t.Fatalf("set pending question: changed=%v err=%v", changed, err)
	}

	interactions, _, err := memory.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID: "session-1", GenerationID: 1, PerPage: 1, HasPendingQuestion: true,
	})
	if err != nil || len(interactions) != 1 || interactions[0].ID != "interaction-question" {
		t.Fatalf("pending interactions = %#v, err = %v", interactions, err)
	}
}

func TestListInteractionsOrdersByCreationTime(t *testing.T) {
	ctx := context.Background()
	memory := New()
	old := &types.Interaction{
		ID: "int_activation_ffff", SessionID: "session-1", UserID: "user-1",
		Created: time.Now().Add(-time.Hour), State: types.InteractionStateError,
	}
	newer := &types.Interaction{
		ID: "int_01newer", SessionID: "session-1", UserID: "user-1",
		Created: time.Now(), State: types.InteractionStateComplete,
	}
	for _, interaction := range []*types.Interaction{old, newer} {
		if _, err := memory.CreateInteraction(ctx, interaction); err != nil {
			t.Fatal(err)
		}
	}

	ascending, _, err := memory.ListInteractions(ctx, &types.ListInteractionsQuery{SessionID: "session-1", GenerationID: -1})
	if err != nil || len(ascending) != 2 || ascending[0].ID != old.ID || ascending[1].ID != newer.ID {
		t.Fatalf("ascending interactions = %#v, err = %v", ascending, err)
	}
	descending, _, err := memory.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID: "session-1", GenerationID: -1, PerPage: 1, Order: "created DESC, id DESC",
	})
	if err != nil || len(descending) != 1 || descending[0].ID != newer.ID {
		t.Fatalf("descending interactions = %#v, err = %v", descending, err)
	}
}

func TestListInteractionsFiltersCompletedSuccessfully(t *testing.T) {
	ctx := context.Background()
	memory := New()
	now := time.Now()
	for _, interaction := range []*types.Interaction{
		{ID: "successful", SessionID: "session-1", GenerationID: 1, State: types.InteractionStateComplete, Created: now, Completed: now.Add(time.Second)},
		{ID: "synthetic", SessionID: "session-1", State: types.InteractionStateComplete, Created: now.Add(time.Minute)},
		{ID: "errored", SessionID: "session-1", State: types.InteractionStateError, Created: now.Add(2 * time.Minute), Completed: now.Add(3 * time.Minute), Error: "failed"},
		{ID: "failed-complete", SessionID: "session-1", State: types.InteractionStateComplete, Created: now.Add(4 * time.Minute), Completed: now.Add(5 * time.Minute), Error: "failed"},
	} {
		if _, err := memory.CreateInteraction(ctx, interaction); err != nil {
			t.Fatal(err)
		}
	}

	interactions, _, err := memory.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID: "session-1", GenerationID: -1, PerPage: 1,
		CompletedSuccessfully: true, Order: "created DESC, id DESC",
	})
	if err != nil || len(interactions) != 1 || interactions[0].ID != "successful" {
		t.Fatalf("successful interactions = %#v, err = %v", interactions, err)
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
