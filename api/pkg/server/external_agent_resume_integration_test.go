package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
)

// Reconnect resume, driven over a real WebSocket through the production
// handler. This is the regression harness for the 2026-08-16 incident: an API
// restart during a live turn re-sent the prompt into the running ACP session,
// and the agent's rejection — carrying the same request_id — killed the turn it
// duplicated. See design/2026-08-16-api-restart-live-turn-reconnect.md.

type resumeTestAgent struct {
	t      *testing.T
	conn   *websocket.Conn
	server *httptest.Server
}

func (a *resumeTestAgent) close() {
	_ = a.conn.Close()
	a.server.Close()
}

// send emits a sync event from the agent to Helix.
func (a *resumeTestAgent) send(eventType string, data map[string]interface{}) {
	a.t.Helper()
	require.NoError(a.t, a.conn.WriteJSON(types.SyncMessage{EventType: eventType, Data: data}))
}

// awaitCommand reads commands until it sees one of cmdType, or the deadline
// passes. Returns nil when nothing matching arrived — which is the assertion
// most of these tests actually care about.
func (a *resumeTestAgent) awaitCommand(cmdType string, within time.Duration) map[string]interface{} {
	a.t.Helper()
	deadline := time.Now().Add(within)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil
		}
		require.NoError(a.t, a.conn.SetReadDeadline(time.Now().Add(remaining)))
		_, raw, err := a.conn.ReadMessage()
		if err != nil {
			return nil // deadline or close: nothing matching arrived
		}
		var cmd struct {
			Type string                 `json:"type"`
			Data map[string]interface{} `json:"data"`
		}
		if json.Unmarshal(raw, &cmd) != nil {
			continue
		}
		if cmd.Type == cmdType {
			return cmd.Data
		}
	}
}

// startResumeTest stands up the server with one session whose turn is Waiting
// and already dispatched — i.e. exactly the state an API restart leaves behind
// when the agent was mid-turn — then connects an agent to it.
func startResumeTest(t *testing.T) (*HelixAPIServer, *memorystore.MemoryStore, *resumeTestAgent, *types.Interaction) {
	t.Helper()

	mem := memorystore.New()
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	srv := NewTestServer(mem, ps)

	ctx := context.Background()
	sessionID := "ses_resume_" + randSuffix()
	session, err := mem.CreateSession(ctx, types.Session{
		ID:        sessionID,
		Owner:     "usr_resume",
		OwnerType: types.OwnerTypeUser,
		Type:      types.SessionTypeText,
		Mode:      types.SessionModeInference,
		Metadata: types.SessionMetadata{
			AgentType:    "zed_external",
			ZedThreadID:  "thread_resume",
			ZedAgentName: "claude",
		},
	})
	require.NoError(t, err)

	dispatchedAt := time.Now().Add(-2 * time.Minute)
	interaction, err := mem.CreateInteraction(ctx, &types.Interaction{
		ID:                        "int_resume_" + randSuffix(),
		SessionID:                 session.ID,
		GenerationID:              session.GenerationID,
		State:                     types.InteractionStateWaiting,
		PromptMessage:             "continue",
		ExternalAgentRequestID:    "req_resume",
		ExternalAgentDispatchedAt: &dispatchedAt,
		Created:                   dispatchedAt,
		Updated:                   dispatchedAt,
	})
	require.NoError(t, err)

	httpSrv := httptest.NewServer(srv.ExternalAgentSyncHandler())
	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "?session_id=" + session.ID
	token, err := srv.generateExternalAgentToken(session.ID)
	require.NoError(t, err)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Authorization": []string{"Bearer " + token},
	})
	require.NoError(t, err)

	agent := &resumeTestAgent{t: t, conn: conn, server: httpSrv}
	t.Cleanup(agent.close)

	// open_thread is written on connect, before the readiness gate. Drain it so
	// the chat_message assertions below are unambiguous.
	require.NotNil(t, agent.awaitCommand("open_thread", 3*time.Second),
		"reconnect must re-open the session's thread")

	return srv, mem, agent, interaction
}

func TestResumeOnReconnect_AgentOwnsTurn_DoesNotResend(t *testing.T) {
	_, mem, agent, interaction := startResumeTest(t)

	// The agent reports it is still running this turn. Re-sending would push a
	// second prompt into the live ACP session — the incident.
	agent.send("agent_ready", map[string]interface{}{
		"agent_name": "claude",
		"active_turns": []interface{}{
			map[string]interface{}{
				"request_id":    "req_resume",
				"acp_thread_id": "thread_resume",
				"state":         "running",
			},
		},
	})

	require.Nil(t, agent.awaitCommand("chat_message", 2*time.Second),
		"a turn the agent reports running must never be re-sent")

	reloaded, err := mem.GetInteraction(context.Background(), interaction.ID)
	require.NoError(t, err)
	require.Equal(t, types.InteractionStateWaiting, reloaded.State,
		"attaching must leave the turn waiting for the agent's own output")
}

func TestResumeOnReconnect_AgentReportsNothingRunning_Resends(t *testing.T) {
	_, _, agent, _ := startResumeTest(t)

	// An EMPTY report is authoritative: the agent restarted and genuinely lost
	// the turn, so Helix must deliver it. This is the case only the handshake
	// can decide — the dispatch marker alone would say "assume it is alive".
	agent.send("agent_ready", map[string]interface{}{
		"agent_name":   "claude",
		"active_turns": []interface{}{},
	})

	cmd := agent.awaitCommand("chat_message", 5*time.Second)
	require.NotNil(t, cmd, "a turn the agent authoritatively does not have must be delivered")
	require.Equal(t, "req_resume", cmd["request_id"])
	require.Equal(t, "thread_resume", cmd["acp_thread_id"],
		"delivery must target the session's existing thread, not open a second one")
}

func TestResumeOnReconnect_LegacyAgentCannotReport_DoesNotResend(t *testing.T) {
	_, mem, agent, interaction := startResumeTest(t)

	// An agent build predating active_turns. Absence is not an empty report:
	// Helix cannot tell, so it attaches and lets verifyResumedTurn correct it
	// later rather than risking a duplicate now.
	// (Remove with https://github.com/helixml/helix/issues/3047.)
	agent.send("agent_ready", map[string]interface{}{"agent_name": "claude"})

	require.Nil(t, agent.awaitCommand("chat_message", 2*time.Second),
		"without a report, an already-dispatched turn must not be re-sent immediately")

	reloaded, err := mem.GetInteraction(context.Background(), interaction.ID)
	require.NoError(t, err)
	require.Equal(t, types.InteractionStateWaiting, reloaded.State)
}

// A session with no Zed thread gets no_open_thread on connect, so Zed reports
// agent_ready at once instead of after its 5-second open_thread timer.
func TestConnect_NoThread_SendsNoOpenThread(t *testing.T) {
	mem := memorystore.New()
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	srv := NewTestServer(mem, ps)

	session, err := mem.CreateSession(context.Background(), types.Session{
		ID:        "ses_fresh_" + randSuffix(),
		Owner:     "usr_fresh",
		OwnerType: types.OwnerTypeUser,
		Type:      types.SessionTypeText,
		Mode:      types.SessionModeInference,
		Metadata:  types.SessionMetadata{AgentType: "zed_external"},
	})
	require.NoError(t, err)

	httpSrv := httptest.NewServer(srv.ExternalAgentSyncHandler())
	token, err := srv.generateExternalAgentToken(session.ID)
	require.NoError(t, err)
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(httpSrv.URL, "http")+"?session_id="+session.ID,
		http.Header{"Authorization": []string{"Bearer " + token}},
	)
	require.NoError(t, err)
	agent := &resumeTestAgent{t: t, conn: conn, server: httpSrv}
	t.Cleanup(agent.close)

	data := agent.awaitCommand("no_open_thread", 3*time.Second)
	require.NotNil(t, data, "a session with no thread must get no_open_thread on connect")
	require.Equal(t, session.ID, data["session_id"])
}

func TestResumeOnReconnect_WaitingTurnBeyondHistoryPageIsDelivered(t *testing.T) {
	mem := memorystore.New()
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	srv := NewTestServer(mem, ps)

	ctx := context.Background()
	session, err := mem.CreateSession(ctx, types.Session{
		ID:        "ses_long_history_" + randSuffix(),
		Owner:     "usr_resume",
		OwnerType: types.OwnerTypeUser,
		Type:      types.SessionTypeText,
		Mode:      types.SessionModeInference,
		Metadata:  types.SessionMetadata{AgentType: "zed_external"},
	})
	require.NoError(t, err)

	for i := 0; i < 1001; i++ {
		_, err = mem.CreateInteraction(ctx, &types.Interaction{
			ID:           fmt.Sprintf("int_history_%04d", i),
			SessionID:    session.ID,
			GenerationID: session.GenerationID,
			State:        types.InteractionStateComplete,
		})
		require.NoError(t, err)
	}
	waiting, err := mem.CreateInteraction(ctx, &types.Interaction{
		ID:                     "int_waiting_1_" + randSuffix(),
		SessionID:              session.ID,
		GenerationID:           session.GenerationID,
		State:                  types.InteractionStateWaiting,
		PromptMessage:          "resume me",
		ExternalAgentRequestID: "req_waiting",
	})
	require.NoError(t, err)
	newerWaiting, err := mem.CreateInteraction(ctx, &types.Interaction{
		ID:                     "int_waiting_2_" + randSuffix(),
		SessionID:              session.ID,
		GenerationID:           session.GenerationID,
		State:                  types.InteractionStateWaiting,
		PromptMessage:          "resume me later",
		ExternalAgentRequestID: "req_waiting_later",
	})
	require.NoError(t, err)

	httpSrv := httptest.NewServer(srv.ExternalAgentSyncHandler())
	token, err := srv.generateExternalAgentToken(session.ID)
	require.NoError(t, err)
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(httpSrv.URL, "http")+"?session_id="+session.ID,
		http.Header{"Authorization": []string{"Bearer " + token}},
	)
	require.NoError(t, err)
	agent := &resumeTestAgent{t: t, conn: conn, server: httpSrv}
	t.Cleanup(agent.close)

	require.NotNil(t, agent.awaitCommand("no_open_thread", 3*time.Second))
	agent.send("agent_ready", map[string]interface{}{
		"agent_name":   "claude",
		"active_turns": []interface{}{},
	})

	cmd := agent.awaitCommand("chat_message", 3*time.Second)
	require.NotNil(t, cmd, "the waiting turn must be found beyond the history page")
	require.Equal(t, waiting.ID, cmd["interaction_id"])
	require.Equal(t, "req_waiting", cmd["request_id"])

	reloaded, err := mem.GetInteraction(ctx, waiting.ID)
	require.NoError(t, err)
	require.NotNil(t, reloaded.ExternalAgentDispatchedAt)
	require.Equal(t, newerWaiting.ID, srv.newestWaitingInteraction(ctx, session).ID)
}

func TestMessageCompleted_FallbackFindsWaitingTurnBeyondHistoryPage(t *testing.T) {
	mem := memorystore.New()
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	srv := NewTestServer(mem, ps)

	ctx := context.Background()
	session, err := mem.CreateSession(ctx, types.Session{
		ID:        "ses_complete_long_history_" + randSuffix(),
		Owner:     "usr_complete",
		OwnerType: types.OwnerTypeUser,
		Type:      types.SessionTypeText,
		Mode:      types.SessionModeInference,
		Metadata:  types.SessionMetadata{AgentType: "zed_external"},
	})
	require.NoError(t, err)

	for i := 0; i < 1001; i++ {
		_, err = mem.CreateInteraction(ctx, &types.Interaction{
			ID:           fmt.Sprintf("int_complete_history_%04d", i),
			SessionID:    session.ID,
			GenerationID: session.GenerationID,
			State:        types.InteractionStateComplete,
		})
		require.NoError(t, err)
	}
	waiting, err := mem.CreateInteraction(ctx, &types.Interaction{
		ID:              "int_complete_waiting_" + randSuffix(),
		SessionID:       session.ID,
		GenerationID:    session.GenerationID,
		State:           types.InteractionStateWaiting,
		ResponseMessage: "finished response",
	})
	require.NoError(t, err)
	srv.contextMappings[routeKey(session.ID, "thread-complete-long-history")] = session.ID

	err = srv.handleMessageCompleted(session.ID, &types.SyncMessage{
		EventType: "message_completed",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-complete-long-history",
		},
	})
	require.NoError(t, err)

	reloaded, err := mem.GetInteraction(ctx, waiting.ID)
	require.NoError(t, err)
	require.Equal(t, types.InteractionStateComplete, reloaded.State)
}

func TestStreamingContext_FindsTurnBeyondHistoryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		durable bool
		legacy  bool
	}{
		{name: "in-memory mapping"},
		{name: "durable request ID", durable: true},
		{name: "legacy newest waiting", legacy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mem := memorystore.New()
			ps, err := pubsub.NewInMemoryNats()
			require.NoError(t, err)
			srv := NewTestServer(mem, ps)

			ctx := context.Background()
			session, err := mem.CreateSession(ctx, types.Session{
				ID:        "ses_stream_long_history_" + randSuffix(),
				Owner:     "usr_stream",
				OwnerType: types.OwnerTypeUser,
				Type:      types.SessionTypeText,
				Mode:      types.SessionModeInference,
				Metadata:  types.SessionMetadata{AgentType: "zed_external"},
			})
			require.NoError(t, err)

			var olderWaiting *types.Interaction
			if tc.legacy {
				olderWaiting, err = mem.CreateInteraction(ctx, &types.Interaction{
					ID:           "int_stream_0000_old_waiting_" + randSuffix(),
					SessionID:    session.ID,
					GenerationID: session.GenerationID,
					State:        types.InteractionStateWaiting,
				})
				require.NoError(t, err)
			}
			for i := 0; i < 1001; i++ {
				historical := &types.Interaction{
					ID:           fmt.Sprintf("int_stream_history_%04d", i),
					SessionID:    session.ID,
					GenerationID: session.GenerationID,
					State:        types.InteractionStateComplete,
				}
				if tc.durable && i == 0 {
					historical.ExternalAgentRequestID = "req-stream"
				}
				_, err = mem.CreateInteraction(ctx, historical)
				require.NoError(t, err)
			}
			waiting := &types.Interaction{
				ID:           "int_stream_waiting_" + randSuffix(),
				SessionID:    session.ID,
				GenerationID: session.GenerationID,
				State:        types.InteractionStateWaiting,
			}
			if tc.durable {
				waiting.ExternalAgentRequestID = "req-stream"
			} else if !tc.legacy {
				srv.requestToInteractionMapping["req-stream"] = waiting.ID
			}
			_, err = mem.CreateInteraction(ctx, waiting)
			require.NoError(t, err)
			if tc.durable {
				_, err = mem.CreateInteraction(ctx, &types.Interaction{
					ID:           "int_stream_zzzz_unrelated_" + randSuffix(),
					SessionID:    session.ID,
					GenerationID: session.GenerationID,
					State:        types.InteractionStateWaiting,
				})
				require.NoError(t, err)
			}
			srv.contextMappings[routeKey(session.ID, "thread-stream-long-history")] = session.ID

			data := map[string]interface{}{
				"acp_thread_id": "thread-stream-long-history",
				"message_id":    "message-1",
				"role":          "assistant",
				"content":       "streamed token",
			}
			if !tc.legacy {
				data["request_id"] = "req-stream"
			}
			err = srv.handleMessageAdded(session.ID, &types.SyncMessage{
				EventType: "message_added",
				Data:      data,
			})
			require.NoError(t, err)

			reloaded, err := mem.GetInteraction(ctx, waiting.ID)
			require.NoError(t, err)
			require.Equal(t, "streamed token", reloaded.ResponseMessage)
			if olderWaiting != nil {
				reloadedOlder, err := mem.GetInteraction(ctx, olderWaiting.ID)
				require.NoError(t, err)
				require.Empty(t, reloadedOlder.ResponseMessage)
			}
		})
	}
}

func TestStreamingContext_RejectsDirectLookupOutsideSessionGeneration(t *testing.T) {
	for _, tc := range []struct {
		name       string
		durable    bool
		wrongScope func(*types.Interaction)
	}{
		{name: "mapped other session", wrongScope: func(i *types.Interaction) { i.SessionID = "ses_other" }},
		{name: "mapped other generation", wrongScope: func(i *types.Interaction) { i.GenerationID = 1 }},
		{name: "durable other session", durable: true, wrongScope: func(i *types.Interaction) { i.SessionID = "ses_other" }},
		{name: "durable other generation", durable: true, wrongScope: func(i *types.Interaction) { i.GenerationID = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mem := memorystore.New()
			ps, err := pubsub.NewInMemoryNats()
			require.NoError(t, err)
			srv := NewTestServer(mem, ps)

			ctx := context.Background()
			session, err := mem.CreateSession(ctx, types.Session{
				ID:           "ses_stream_scope_" + randSuffix(),
				GenerationID: 2,
				Owner:        "usr_stream",
				OwnerType:    types.OwnerTypeUser,
				Type:         types.SessionTypeText,
				Mode:         types.SessionModeInference,
				Metadata:     types.SessionMetadata{AgentType: "zed_external"},
			})
			require.NoError(t, err)

			for i := 0; i < 1000; i++ {
				_, err = mem.CreateInteraction(ctx, &types.Interaction{
					ID:           fmt.Sprintf("int_stream_scope_history_%04d", i),
					SessionID:    session.ID,
					GenerationID: session.GenerationID,
					State:        types.InteractionStateComplete,
				})
				require.NoError(t, err)
			}
			candidate := &types.Interaction{
				ID:           "int_stream_scope_" + randSuffix(),
				SessionID:    session.ID,
				GenerationID: session.GenerationID,
				State:        types.InteractionStateWaiting,
			}
			tc.wrongScope(candidate)
			if tc.durable {
				candidate.ExternalAgentRequestID = "req-stream-scope"
			} else {
				srv.requestToInteractionMapping["req-stream-scope"] = candidate.ID
			}
			_, err = mem.CreateInteraction(ctx, candidate)
			require.NoError(t, err)

			sctx := srv.getOrCreateStreamingContext(ctx, session.ID, "req-stream-scope")
			require.NotNil(t, sctx)
			require.Nil(t, sctx.interaction)
		})
	}
}

func TestStreamingContext_RecoversNewestInterruptedTurnBeyondHistoryPage(t *testing.T) {
	mem := memorystore.New()
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	srv := NewTestServer(mem, ps)

	ctx := context.Background()
	session, err := mem.CreateSession(ctx, types.Session{
		ID:        "ses_interrupted_long_history_" + randSuffix(),
		Owner:     "usr_stream",
		OwnerType: types.OwnerTypeUser,
		Type:      types.SessionTypeText,
		Mode:      types.SessionModeInference,
		Metadata:  types.SessionMetadata{AgentType: "zed_external"},
	})
	require.NoError(t, err)

	for i := 0; i < 1001; i++ {
		_, err = mem.CreateInteraction(ctx, &types.Interaction{
			ID:           fmt.Sprintf("int_interrupted_history_%04d", i),
			SessionID:    session.ID,
			GenerationID: session.GenerationID,
			State:        types.InteractionStateComplete,
		})
		require.NoError(t, err)
	}
	interrupted, err := mem.CreateInteraction(ctx, &types.Interaction{
		ID:           "int_interrupted_latest_" + randSuffix(),
		SessionID:    session.ID,
		GenerationID: session.GenerationID,
		State:        types.InteractionStateError,
		Error:        "Interrupted",
	})
	require.NoError(t, err)

	sctx := srv.getOrCreateStreamingContext(ctx, session.ID, "")
	require.NotNil(t, sctx)
	require.Equal(t, interrupted.ID, sctx.interactionID)

	reloaded, err := mem.GetInteraction(ctx, interrupted.ID)
	require.NoError(t, err)
	require.Equal(t, types.InteractionStateWaiting, reloaded.State)
}
