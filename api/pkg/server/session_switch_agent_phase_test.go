package server

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureConfigChanges records every config_changed field published for a
// session. The returned func is safe to call after the publisher has stopped.
func captureConfigChanges(t *testing.T, srv *HelixAPIServer, session *types.Session) func() []string {
	t.Helper()
	var mu sync.Mutex
	var fields []string
	sub, err := srv.pubsub.Subscribe(context.Background(),
		pubsub.GetSessionQueue(session.Owner, session.ID),
		func(payload []byte) error {
			var event struct {
				Type  string `json:"type"`
				Field string `json:"field"`
			}
			if err := json.Unmarshal(payload, &event); err != nil || event.Type != "config_changed" {
				return nil
			}
			mu.Lock()
			fields = append(fields, event.Field)
			mu.Unlock()
			return nil
		})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), fields...)
	}
}

func claudeConfig() *types.CodeAgentExecutionConfig {
	return &types.CodeAgentExecutionConfig{
		Runtime: types.CodeAgentRuntimeClaudeCode,
		Model:   "claude-opus-4-7",
	}
}

// newPhaseTransitionTask wires a spec task onto an already-planning session,
// the way ApproveSpecs finds it.
func newPhaseTransitionTask(t *testing.T, mem *memorystore.MemoryStore, session *types.Session) *types.SpecTask {
	t.Helper()
	return &types.SpecTask{
		ID:                "spt_phase_" + randSuffix(),
		PlanningSessionID: session.ID,
		CreatedBy:         session.Owner,
		Status:            types.TaskStatusImplementationQueued,
		CodeAgentConfig:   claudeConfig(),
	}
}

func interactionsFor(t *testing.T, mem *memorystore.MemoryStore, session *types.Session) []*types.Interaction {
	t.Helper()
	interactions, _, err := mem.ListInteractions(context.Background(), &types.ListInteractionsQuery{
		SessionID: session.ID, GenerationID: session.GenerationID, PerPage: 1000,
	})
	require.NoError(t, err)
	return interactions
}

func TestSpecTaskPhaseConfigEquivalent(t *testing.T) {
	shared := claudeConfig()

	t.Run("nil planning config reuses the implementation pointer", func(t *testing.T) {
		task := &types.SpecTask{CodeAgentConfig: shared}
		assert.True(t, specTaskPhaseConfigEquivalent(task))
	})

	t.Run("identical but distinct configs", func(t *testing.T) {
		task := &types.SpecTask{CodeAgentConfig: claudeConfig(), PlanningCodeAgentConfig: claudeConfig()}
		assert.True(t, specTaskPhaseConfigEquivalent(task))
	})

	t.Run("same runtime, different model", func(t *testing.T) {
		planning := claudeConfig()
		planning.Model = "claude-sonnet-5"
		task := &types.SpecTask{CodeAgentConfig: claudeConfig(), PlanningCodeAgentConfig: planning}
		assert.False(t, specTaskPhaseConfigEquivalent(task),
			"a different model changes the rendered settings.json and only takes effect on a new thread")
	})

	t.Run("same runtime, different reasoning effort", func(t *testing.T) {
		planning := claudeConfig()
		planning.ReasoningEffort = "low"
		task := &types.SpecTask{CodeAgentConfig: claudeConfig(), PlanningCodeAgentConfig: planning}
		assert.False(t, specTaskPhaseConfigEquivalent(task))
	})

	t.Run("different runtime", func(t *testing.T) {
		planning := claudeConfig()
		planning.Runtime = types.CodeAgentRuntimeCodexCLI
		task := &types.SpecTask{CodeAgentConfig: claudeConfig(), PlanningCodeAgentConfig: planning}
		assert.False(t, specTaskPhaseConfigEquivalent(task))
	})

	t.Run("different goose recipe", func(t *testing.T) {
		task := &types.SpecTask{
			CodeAgentConfig:         claudeConfig(),
			PlanningCodeAgentConfig: claudeConfig(),
			GooseRecipeName:         "implement",
			PlanningGooseRecipeName: "plan",
		}
		assert.False(t, specTaskPhaseConfigEquivalent(task))
	})

	t.Run("nil task", func(t *testing.T) {
		assert.False(t, specTaskPhaseConfigEquivalent(nil))
	})
}

// Path 1: an identical harness keeps the existing ACP thread. Nothing is
// published, nothing is seeded, nothing is restarted.
func TestTransitionSpecTaskToImplementation_SameHarnessKeepsThread(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	seedCodingAgent(mem, "app_parent", "anthropic", "claude-opus-4-7")
	session := newTestParentSession("user_a")
	session.ParentApp = ""
	session.Metadata.ZedThreadID = "ctx_planning_thread"
	seedParentWithInteractions(t, mem, session, 2)

	task := newPhaseTransitionTask(t, mem, session)
	const prompt = "## CURRENT PHASE: IMPLEMENTATION\n\nImplement the approved plan."

	require.NoError(t, srv.transitionSpecTaskToImplementation(ctx, task, prompt))

	updated, err := mem.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, "ctx_planning_thread", updated.Metadata.ZedThreadID,
		"the planning thread carries the context; it must not be cleared")
	assert.True(t, updated.Metadata.AgentSwitchedAt.IsZero(), "no switch happened")
	assert.Equal(t, string(types.SpecTaskPhaseImplementation), updated.Metadata.Phase)

	for _, interaction := range interactionsFor(t, mem, session) {
		assert.NotEqual(t, types.InteractionTriggerForkSeed, interaction.Trigger, "Path 1 must not seed a fork_seed")
		assert.NotEqual(t, types.InteractionTriggerForkHandoff, interaction.Trigger, "Path 1 must not create a fork_handoff")
	}

	entries, err := mem.ListPromptHistoryBySession(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, entries, 1, "exactly one implementation instruction must be queued")
	assert.Equal(t, prompt, entries[0].Content)
	assert.False(t, entries[0].Interrupt,
		"interrupt=false defers behind an in-flight planning turn instead of interleaving two prompts on one thread")
}

func TestTransitionSpecTaskToImplementation_SameHarnessIsIdempotent(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	seedCodingAgent(mem, "app_parent", "anthropic", "claude-opus-4-7")
	session := newTestParentSession("user_a")
	session.ParentApp = ""
	session.Metadata.ZedThreadID = "ctx_planning_thread"
	seedParentWithInteractions(t, mem, session, 1)

	task := newPhaseTransitionTask(t, mem, session)
	const prompt = "Implement the approved plan."

	require.NoError(t, srv.transitionSpecTaskToImplementation(ctx, task, prompt))
	require.NoError(t, srv.transitionSpecTaskToImplementation(ctx, task, prompt))

	entries, err := mem.ListPromptHistoryBySession(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "a retried transition must not deliver the instruction twice")
}

// Path 2: a genuine switch still gets a fresh thread, a fork_seed and a
// fork_handoff — and now a non-empty transcript on the seed.
func TestTransitionSpecTaskToImplementation_DifferentHarnessSwitchesAndSeeds(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	seedCodingAgent(mem, "app_parent", "anthropic", "claude-opus-4-7")
	session := newTestParentSession("user_a")
	session.ParentApp = ""
	session.Metadata.ZedThreadID = "ctx_planning_thread"
	seedParentWithInteractions(t, mem, session, 2)

	task := newPhaseTransitionTask(t, mem, session)
	task.CodeAgentConfig = &types.CodeAgentExecutionConfig{
		Runtime: types.CodeAgentRuntimeCodexCLI,
		Model:   "gpt-5.6-sol",
	}
	const prompt = "Implement the approved plan."

	require.NoError(t, srv.transitionSpecTaskToImplementation(ctx, task, prompt))

	updated, err := mem.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Empty(t, updated.Metadata.ZedThreadID, "a genuine switch needs a fresh thread")
	assert.False(t, updated.Metadata.AgentSwitchedAt.IsZero())
	assert.Equal(t, types.CodeAgentRuntimeCodexCLI, updated.Metadata.CodeAgentRuntime)

	var seed, handoff *types.Interaction
	for _, interaction := range interactionsFor(t, mem, session) {
		switch interaction.Trigger {
		case types.InteractionTriggerForkSeed:
			seed = interaction
		case types.InteractionTriggerForkHandoff:
			handoff = interaction
		}
	}
	require.NotNil(t, seed)
	require.NotEmpty(t, seed.ResponseMessage, "the new thread must not start blind")
	assert.Contains(t, seed.ResponseMessage, "user turn 0")
	require.NotNil(t, handoff)
	assert.Contains(t, handoff.PromptMessage, prompt)
	assert.Contains(t, handoff.PromptMessage, "Do not summarise or restate it",
		"the model must be told not to summarise the prepended transcript")

	// The seeded transcript is actually delivered, not just stored.
	seeded := srv.maybePrependTranscript(ctx, updated, handoff.PromptMessage)
	assert.Contains(t, seeded, "user turn 0")
	assert.Contains(t, seeded, prompt)
}

func TestHasImplementationInstruction_SeesPendingQueueRow(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	session := newTestParentSession("user_a")
	seedParentWithInteractions(t, mem, session, 1)

	assert.False(t, srv.hasImplementationInstruction(ctx, session, "implement it"))

	require.NoError(t, mem.CreatePromptHistoryEntry(ctx, &types.PromptHistoryEntry{
		ID: "ph_1", UserID: session.Owner, SessionID: session.ID, Content: "implement it", Status: "pending",
	}))
	assert.True(t, srv.hasImplementationInstruction(ctx, session, "implement it"))
}

func TestSerializeTranscript_MiddleOutKeepsBothEnds(t *testing.T) {
	head := "**User:** FRAMING-HEAD " + strings.Repeat("a", 4_000)
	middle := "**User:** MIDDLE " + strings.Repeat("b", 40_000)
	tail := "**User:** LATEST-TAIL " + strings.Repeat("c", 4_000)

	interactions := []*types.Interaction{
		{State: types.InteractionStateComplete, PromptMessage: head},
		{State: types.InteractionStateComplete, PromptMessage: middle},
		{State: types.InteractionStateComplete, PromptMessage: tail},
	}

	const budget = 20_000
	out, stats := serializeTranscriptWithMode(interactions, budget, truncateMiddleOut)
	assert.LessOrEqual(t, len(out), budget)
	assert.True(t, stats.truncated)
	assert.Contains(t, out, "FRAMING-HEAD", "the framing must survive a phase handoff")
	assert.Contains(t, out, "LATEST-TAIL")
	assert.NotContains(t, out, "MIDDLE")
	assert.Contains(t, out, "the middle of this transcript was elided")

	// Generic switches keep today's oldest-first behaviour.
	oldest, _ := serializeTranscriptWithMode(interactions, budget, truncateOldestFirst)
	assert.LessOrEqual(t, len(oldest), budget)
	assert.NotContains(t, oldest, "FRAMING-HEAD")
	assert.Contains(t, oldest, "LATEST-TAIL")
}

func TestSerializeTranscript_MiddleOutCutsInsideASingleBlock(t *testing.T) {
	// A planning session is typically one or two very large blocks, so block
	// selection alone can never fit — the cut has to work at the byte level.
	only := "**User:** FRAMING-HEAD " + strings.Repeat("a", 60_000) + " LATEST-TAIL"
	interactions := []*types.Interaction{{State: types.InteractionStateComplete, PromptMessage: only}}

	const budget = 10_000
	out, stats := serializeTranscriptWithMode(interactions, budget, truncateMiddleOut)
	assert.LessOrEqual(t, len(out), budget)
	assert.True(t, stats.truncated)
	assert.Contains(t, out, "FRAMING-HEAD")
	assert.Contains(t, out, "LATEST-TAIL")
}

// Planning transcripts are full of em-dashes and emoji. A naive byte slice puts
// a replacement character right where the model starts and stops reading.
func TestSerializeTranscript_MiddleOutDoesNotSplitRunes(t *testing.T) {
	only := "**User:** " + strings.Repeat("—", 20_000)
	interactions := []*types.Interaction{{State: types.InteractionStateComplete, PromptMessage: only}}

	const budget = 9_999 // deliberately not a multiple of 3
	out, _ := serializeTranscriptWithMode(interactions, budget, truncateMiddleOut)
	assert.LessOrEqual(t, len(out), budget)
	assert.True(t, utf8.ValidString(out), "the elided transcript must stay valid UTF-8")
	assert.NotContains(t, out, "�")
}

func TestSerializeTranscript_EmptyWhenNothingComplete(t *testing.T) {
	interactions := []*types.Interaction{
		{State: types.InteractionStateWaiting, PromptMessage: "cancelled planning turn"},
		{Trigger: types.InteractionTriggerForkSeed, State: types.InteractionStateComplete, PromptMessage: "marker"},
	}
	out, stats := serializeTranscriptWithMode(interactions, maxTranscriptBytes, truncateMiddleOut)
	assert.Empty(t, out)
	assert.Equal(t, 1, stats.skippedNotComplete)
	assert.Equal(t, 1, stats.skippedForkMarkers)
}

// The fallback must not kill a Zed that the daemon confirmed is working.
func TestAgentSwitchRestartFallback_CreditsTheAppliedCallback(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	session := newTestParentSession("user_a")
	switchedAt := time.Now()
	session.Metadata.AgentSwitchedAt = switchedAt
	session.Metadata.AgentConfigAppliedAt = switchedAt.Add(2 * time.Second)
	session.Metadata.AgentHandoffDeliveredAt = switchedAt.Add(2 * time.Second)
	seedParentWithInteractions(t, mem, session, 1)

	restarts := captureConfigChanges(t, srv, session)

	fallbackCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		srv.agentSwitchRestartFallback(fallbackCtx, session.ID, switchedAt)
		close(done)
	}()

	// Well past the 9s live-delivery budget, inside the 60s applied budget.
	time.Sleep(11 * time.Second)
	cancel()
	<-done

	assert.Empty(t, restarts(), "a confirmed-applied config must not be restarted at the 9s mark")
}

func TestAgentSwitchRestartFallback_RestartsWithoutTheCallback(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	session := newTestParentSession("user_a")
	switchedAt := time.Now()
	session.Metadata.AgentSwitchedAt = switchedAt
	seedParentWithInteractions(t, mem, session, 1)

	restarts := captureConfigChanges(t, srv, session)

	fallbackCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	srv.agentSwitchRestartFallback(fallbackCtx, session.ID, switchedAt)

	assert.Eventually(t, func() bool {
		for _, field := range restarts() {
			if field == "agent_restart" {
				return true
			}
		}
		return false
	}, 5*time.Second, 50*time.Millisecond, "with no callback the 9s budget is unchanged")
}

func TestAgentSwitchRestartFallback_ExitsOnNewThread(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	session := newTestParentSession("user_a")
	switchedAt := time.Now()
	session.Metadata.AgentSwitchedAt = switchedAt
	session.Metadata.ZedThreadID = "ctx_new_thread"
	seedParentWithInteractions(t, mem, session, 1)

	restarts := captureConfigChanges(t, srv, session)

	fallbackCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	start := time.Now()
	srv.agentSwitchRestartFallback(fallbackCtx, session.ID, switchedAt)

	assert.Empty(t, restarts())
	assert.Less(t, time.Since(start), 5*time.Second,
		"polling must exit as soon as thread_created lands, not sleep out the whole budget")
}

// Helix knows the dispatch it is about to kill is dead, so the reconnect must
// choose deliver, not attach_and_verify.
func TestInvalidateDispatchForRequestedRestart_MakesReconnectDeliver(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	session := newTestParentSession("user_a")
	seedParentWithInteractions(t, mem, session, 1)

	dispatched := time.Now()
	waiting, err := mem.CreateInteraction(ctx, &types.Interaction{
		SessionID:                 session.ID,
		UserID:                    session.Owner,
		GenerationID:              session.GenerationID,
		State:                     types.InteractionStateWaiting,
		PromptMessage:             "Implement the approved plan.",
		ExternalAgentRequestID:    "req_killed",
		ExternalAgentDispatchedAt: &dispatched,
	})
	require.NoError(t, err)

	action, _ := decideResume(agentTurnReport{}, waiting)
	require.Equal(t, resumeAttachAndVerify, action, "precondition: a dispatched turn normally attaches")

	srv.invalidateDispatchForRequestedRestart(ctx, session)

	reloaded, err := mem.GetInteraction(ctx, waiting.ID)
	require.NoError(t, err)
	assert.Nil(t, reloaded.ExternalAgentDispatchedAt)
	action, _ = decideResume(agentTurnReport{}, reloaded)
	assert.Equal(t, resumeDeliver, action,
		"after a Helix-requested Zed restart the reconnect must re-deliver, not wait out the 180s silence budget")
}

func TestInvalidateDispatchForRequestedRestart_SkipsWhenThreadStillBound(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	session := newTestParentSession("user_a")
	session.Metadata.ZedThreadID = "ctx_live_thread"
	seedParentWithInteractions(t, mem, session, 1)

	dispatched := time.Now()
	waiting, err := mem.CreateInteraction(ctx, &types.Interaction{
		SessionID:                 session.ID,
		UserID:                    session.Owner,
		GenerationID:              session.GenerationID,
		State:                     types.InteractionStateWaiting,
		ExternalAgentRequestID:    "req_live",
		ExternalAgentDispatchedAt: &dispatched,
	})
	require.NoError(t, err)

	srv.invalidateDispatchForRequestedRestart(ctx, session)

	reloaded, err := mem.GetInteraction(ctx, waiting.ID)
	require.NoError(t, err)
	assert.NotNil(t, reloaded.ExternalAgentDispatchedAt,
		"an ordinary reconnect must keep attach_and_verify — Zed may really be running the turn")
}
