package server

import (
	"context"
	"encoding/json"
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
