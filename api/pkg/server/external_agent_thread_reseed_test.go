package server

import (
	"context"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestReseedBudgetForContext(t *testing.T) {
	assert.Equal(t, maxTranscriptBytes, reseedBudgetForContext(0), "unknown window keeps the agent-switch budget")
	// qwen3.8-flash-next (the incident model) has a 262,144-token window.
	assert.Equal(t, maxTranscriptBytes, reseedBudgetForContext(262_144))
	assert.Equal(t, 64_000, reseedBudgetForContext(32_000), "a small window caps the seed at about half of it")
}

func completeTurn(id, prompt, response string) *types.Interaction {
	return &types.Interaction{
		ID: id, SessionID: "ses_seed", State: types.InteractionStateComplete,
		PromptMessage: prompt, ResponseMessage: response,
	}
}

// A session with nothing to say — no task, no prior turns — sends the user's
// message alone rather than a preamble explaining an empty history.
func (s *WebSocketSyncSuite) TestThreadReseedPreamble_EmptyWithoutTaskOrHistory() {
	session := &types.Session{ID: "ses_bare"}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{{ID: "int-current", SessionID: "ses_bare", State: types.InteractionStateWaiting, PromptMessage: "hello"}}, int64(1), nil,
	)

	preamble, _ := s.server.buildThreadReseedPreamble(context.Background(), session, "int-current")
	s.Empty(preamble)
}

// The seed is the whole conversation in order, minus the turn about to be sent
// and the synthetic fork markers.
func (s *WebSocketSyncSuite) TestThreadReseedPreamble_FullTranscriptExcludesReplayAndForkMarkers() {
	session := &types.Session{ID: "ses_seed", GenerationID: 3}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, query *types.ListInteractionsQuery) ([]*types.Interaction, int64, error) {
			s.Equal("ses_seed", query.SessionID)
			s.Equal(3, query.GenerationID)
			return []*types.Interaction{
				completeTurn("int-1", "my codeword is PELICAN", "noted"),
				{ID: "int-seed", SessionID: "ses_seed", Trigger: types.InteractionTriggerForkSeed, State: types.InteractionStateComplete, PromptMessage: "Agent switched", ResponseMessage: "OLD SEED BLOB"},
				{ID: "int-handoff", SessionID: "ses_seed", Trigger: types.InteractionTriggerForkHandoff, State: types.InteractionStateComplete, PromptMessage: "HANDOFF META", ResponseMessage: "ack"},
				completeTurn("int-2", "second", "two"),
				// The replayed turn is Complete here on purpose: exclusion is by
				// id, not by relying on the state filter.
				completeTurn("int-current", "what is my codeword?", ""),
			}, int64(5), nil
		},
	)

	preamble, stats := s.server.buildThreadReseedPreamble(context.Background(), session, "int-current")

	s.Contains(preamble, "previous agent session for this conversation was lost")
	s.Contains(preamble, "re-read files")
	s.Contains(preamble, "PELICAN")
	s.Less(strings.Index(preamble, "PELICAN"), strings.Index(preamble, "second"), "oldest first")
	s.NotContains(preamble, "what is my codeword?")
	s.NotContains(preamble, "OLD SEED BLOB")
	s.NotContains(preamble, "HANDOFF META")
	s.Equal(2, stats.skippedForkMarkers)
	s.False(stats.truncated)
	s.True(strings.HasSuffix(preamble, "The user's message follows.\n\n"))
}

// A spec task keeps its header, with the original prompt in full.
func (s *WebSocketSyncSuite) TestThreadReseedPreamble_SpecTaskHeader() {
	session := &types.Session{ID: "ses_seed", Metadata: types.SessionMetadata{SpecTaskID: "spt_x"}}
	longPrompt := strings.Repeat("review the container hardening ", 100)
	s.store.EXPECT().GetSpecTask(gomock.Any(), "spt_x").Return(&types.SpecTask{
		ID: "spt_x", Name: "Audit the security findings", OriginalPrompt: longPrompt,
		BranchName: "feature/000386-audit", DesignDocPath: "design/tasks/000386_audit",
	}, nil).AnyTimes()
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil)

	preamble, _ := s.server.buildThreadReseedPreamble(context.Background(), session, "")

	s.Contains(preamble, "Audit the security findings")
	s.Contains(preamble, longPrompt)
	s.Contains(preamble, "Branch: feature/000386-audit")
	s.Contains(preamble, "Design docs: design/tasks/000386_audit")
}

// Over budget, the oldest turns go and the agent is told so.
func (s *WebSocketSyncSuite) TestThreadReseedPreamble_TruncatesOldestFirstWithNotice() {
	session := &types.Session{ID: "ses_seed"}
	big := strings.Repeat("x", maxTranscriptBytes/2)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return([]*types.Interaction{
		completeTurn("int-1", "OLDEST "+big, "ok"),
		completeTurn("int-2", "MIDDLE "+big, "ok"),
		completeTurn("int-3", "NEWEST "+big, "ok"),
	}, int64(3), nil)

	preamble, stats := s.server.buildThreadReseedPreamble(context.Background(), session, "int-current")

	s.True(stats.truncated)
	s.LessOrEqual(stats.finalBytes, maxTranscriptBytes)
	s.Contains(preamble, transcriptTruncationNotice)
	s.NotContains(preamble, "OLDEST")
	s.Contains(preamble, "NEWEST")
}

func (s *WebSocketSyncSuite) TestMaybePrependTranscript_ReseedsOnlyTheNewThread() {
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return([]*types.Interaction{
		completeTurn("int-1", "my codeword is PELICAN", "noted"),
	}, int64(1), nil)

	lost := &types.Session{ID: "ses_seed", Metadata: types.SessionMetadata{ThreadReseedFrom: "thread-gone"}}
	got := s.server.maybePrependTranscript(context.Background(), lost, "what is my codeword?", "int-current")
	s.Contains(got, "PELICAN")
	s.True(strings.HasSuffix(got, "what is my codeword?"))

	// Once a thread exists, the message goes as-is — no store reads.
	live := &types.Session{ID: "ses_seed", Metadata: types.SessionMetadata{ZedThreadID: "thread-new", ThreadReseedFrom: "thread-gone"}}
	s.Equal("follow-up", s.server.maybePrependTranscript(context.Background(), live, "follow-up", "int-next"))
}

// Every thread the agent can no longer load — missing from its store, or an
// agent that cannot reopen sessions at all — is replayed into a new thread
// that carries the transcript.
func (s *WebSocketSyncSuite) TestThreadLoadError_ReplaysIntoSeededThread() {
	for name, errMsg := range map[string]string{
		"unrestorable": "Failed to load thread: Agent does not support session loading",
		"missing":      "Failed to load thread: no rollout found for thread id thread-x",
	} {
		s.Run(name, func() {
			s.SetupTest()
			const threadID = "thread-x"
			s.server.contextMappings[routeKey("ses_seed", threadID)] = "ses_seed"
			s.server.requestToInteractionMapping["req-x"] = "int-current"
			sendChan := make(chan types.ExternalAgentCommand, 2)
			s.server.externalAgentWSManager.registerConnection("ses_seed", &ExternalAgentWSConnection{SessionID: "ses_seed", SendChan: sendChan})

			session := &types.Session{ID: "ses_seed", Metadata: types.SessionMetadata{ZedThreadID: threadID, ZedAgentName: "opencode"}}
			pending := &types.Interaction{ID: "int-current", SessionID: "ses_seed", State: types.InteractionStateWaiting, PromptMessage: "what is my codeword?"}

			s.store.EXPECT().GetSession(gomock.Any(), "ses_seed").Return(session, nil).AnyTimes()
			s.store.EXPECT().UpdateSessionMetadata(gomock.Any(), "ses_seed", gomock.Any()).DoAndReturn(
				func(_ context.Context, _ string, metadata types.SessionMetadata) error {
					s.Empty(metadata.ZedThreadID)
					s.Equal(threadID, metadata.ThreadReseedFrom)
					return nil
				},
			)
			s.store.EXPECT().GetInteraction(gomock.Any(), "int-current").Return(pending, nil)
			s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return([]*types.Interaction{
				completeTurn("int-1", "my codeword is PELICAN", "noted"),
				pending,
			}, int64(2), nil)

			s.NoError(s.server.handleThreadLoadError("ses_seed", &types.SyncMessage{
				EventType: "thread_load_error",
				Data:      map[string]interface{}{"acp_thread_id": threadID, "request_id": "req-x", "error": errMsg},
			}))

			select {
			case command := <-sendChan:
				message, _ := command.Data["message"].(string)
				_, hasThreadID := command.Data["acp_thread_id"]
				s.False(hasThreadID, "replayed as the first message of a new thread")
				s.Contains(message, "PELICAN")
				s.True(strings.HasSuffix(message, "what is my codeword?"), message)
				s.Equal(1, strings.Count(message, "what is my codeword?"))
			default:
				s.Fail("missing replay command")
			}
		})
	}
}

// With no turn in flight, the dead thread is still cleared; the reseed flag is
// what makes the user's next message carry the transcript.
func (s *WebSocketSyncSuite) TestThreadLoadError_NoWaitingTurnFlagsNextMessageForReseed() {
	const threadID = "thread-idle"
	s.server.contextMappings[routeKey("ses_idle", threadID)] = "ses_idle"
	session := &types.Session{ID: "ses_idle", Metadata: types.SessionMetadata{ZedThreadID: threadID, ZedAgentName: "dsh"}}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_idle").Return(session, nil).AnyTimes()
	s.store.EXPECT().GetInteraction(gomock.Any(), "req-idle").Return(
		&types.Interaction{ID: "req-idle", SessionID: "ses_idle", State: types.InteractionStateComplete}, nil,
	)
	s.store.EXPECT().UpdateSessionMetadata(gomock.Any(), "ses_idle", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, metadata types.SessionMetadata) error {
			s.Empty(metadata.ZedThreadID)
			s.Equal(threadID, metadata.ThreadReseedFrom)
			return nil
		},
	)

	s.NoError(s.server.handleThreadLoadError("ses_idle", &types.SyncMessage{
		EventType: "thread_load_error",
		Data: map[string]interface{}{
			"acp_thread_id": threadID,
			"request_id":    "req-idle",
			"error":         "Failed to load thread: Agent does not support session loading",
		},
	}))
}
