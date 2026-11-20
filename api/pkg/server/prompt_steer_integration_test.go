package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
)

// steerTestSession builds a session whose agent can steer, with a live agent
// connection, optionally mid-turn.
func steerTestSession(t *testing.T, busy bool) (*HelixAPIServer, *memorystore.MemoryStore, *ExternalAgentWSConnection, string) {
	t.Helper()
	mem := memorystore.New()
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	srv := NewTestServer(mem, ps)

	ctx := context.Background()
	session, err := mem.CreateSession(ctx, types.Session{
		ID:        "ses_steer_" + randSuffix(),
		Owner:     "usr_steer",
		OwnerType: types.OwnerTypeUser,
		Type:      types.SessionTypeText,
		Mode:      types.SessionModeInference,
		Metadata: types.SessionMetadata{
			AgentType:              "zed_external",
			ZedThreadID:            "thread_steer",
			ZedAgentName:           "claude",
			AgentSteeringSupported: true,
		},
	})
	require.NoError(t, err)

	if busy {
		_, err = mem.CreateInteraction(ctx, &types.Interaction{
			ID:            "int_running_" + randSuffix(),
			SessionID:     session.ID,
			GenerationID:  session.GenerationID,
			State:         types.InteractionStateWaiting,
			PromptMessage: "long task",
			Created:       time.Now().Add(-time.Minute),
			Updated:       time.Now(),
		})
		require.NoError(t, err)
	}

	conn := &ExternalAgentWSConnection{SessionID: session.ID, SendChan: make(chan types.ExternalAgentCommand, 8)}
	srv.externalAgentWSManager.registerConnection(session.ID, conn)
	return srv, mem, conn, session.ID
}

func steerInteraction(t *testing.T, mem *memorystore.MemoryStore, promptID string) *types.Interaction {
	t.Helper()
	for _, in := range mem.GetAllInteractions() {
		if in.PromptID == promptID {
			return in
		}
	}
	t.Fatalf("no interaction was created for prompt %s", promptID)
	return nil
}

// A steer delivered into a running turn is dispatched at once (no cancel), is
// marked Steered so the chat can label it, and tells Zed to steer.
func TestSteerPrompt_MidTurn_DispatchesSteeredInteraction(t *testing.T) {
	srv, mem, conn, sessionID := steerTestSession(t, true)

	promptID, err := srv.EnqueueSteerPrompt(sessionID, "change of plan")
	require.NoError(t, err)

	in := steerInteraction(t, mem, promptID)
	require.True(t, in.Steered, "a steer into a running turn must be marked Steered")

	select {
	case cmd := <-conn.SendChan:
		require.Equal(t, "chat_message", cmd.Type)
		require.Equal(t, true, cmd.Data["steer"])
		require.Equal(t, false, cmd.Data["interrupt"])
	case <-time.After(2 * time.Second):
		t.Fatal("steer was not sent to the agent")
	}
}

// Steering an idle agent is an ordinary turn: nothing to label.
func TestSteerPrompt_Idle_IsNotMarkedSteered(t *testing.T) {
	srv, mem, _, sessionID := steerTestSession(t, false)

	promptID, err := srv.EnqueueSteerPrompt(sessionID, "hello")
	require.NoError(t, err)

	require.False(t, steerInteraction(t, mem, promptID).Steered)
}

// A steer saved before Zed reports the agent's steering capability waits like
// a queued prompt. When the capability arrives it is delivered straight away.
func TestSteerPrompt_CapabilityArrival_DeliversWaitingSteer(t *testing.T) {
	srv, mem, conn, sessionID := steerTestSession(t, true)
	ctx := context.Background()
	session, err := mem.GetSession(ctx, sessionID)
	require.NoError(t, err)
	session.Metadata.AgentSteeringSupported = false
	require.NoError(t, mem.UpdateSessionMetadata(ctx, sessionID, session.Metadata))

	_, err = srv.EnqueueSteerPrompt(sessionID, "change of plan")
	require.NoError(t, err)
	select {
	case cmd := <-conn.SendChan:
		t.Fatalf("steer was sent before the agent could steer: %+v", cmd)
	case <-time.After(200 * time.Millisecond):
	}

	srv.contextMappings[routeKey(sessionID, "thread_steer")] = sessionID
	require.NoError(t, srv.processExternalAgentSyncMessage(sessionID, &types.SyncMessage{
		EventType: "agent_capabilities",
		Data:      map[string]interface{}{"acp_thread_id": "thread_steer", "steering_supported": true},
	}))

	select {
	case cmd := <-conn.SendChan:
		require.Equal(t, "chat_message", cmd.Type)
		require.Equal(t, true, cmd.Data["steer"])
	case <-time.After(2 * time.Second):
		t.Fatal("the waiting steer was not delivered when the capability arrived")
	}
}

// A steer whose send failed is re-claimed for retry. If its interaction is
// still Waiting it is already in flight and must not be dispatched again, even
// when a later steer has started a newer turn since.
func TestSteerPrompt_RetryAfterFailedSend_DoesNotDispatchTwice(t *testing.T) {
	srv, mem, conn, sessionID := steerTestSession(t, true)
	ctx := context.Background()

	promptID, err := srv.EnqueueSteerPrompt(sessionID, "change of plan")
	require.NoError(t, err)
	<-conn.SendChan
	_, err = srv.EnqueueSteerPrompt(sessionID, "and another thing")
	require.NoError(t, err)
	<-conn.SendChan

	require.NoError(t, mem.MarkPromptAsFailed(ctx, promptID, "send channel full"))
	srv.processPendingPromptsForSession(ctx, sessionID)

	select {
	case cmd := <-conn.SendChan:
		t.Fatalf("an in-flight steer was dispatched again: %+v", cmd)
	case <-time.After(200 * time.Millisecond):
	}
	count := 0
	for _, in := range mem.GetAllInteractions() {
		if in.PromptID == promptID {
			count++
		}
	}
	require.Equal(t, 1, count, "a retried steer must not create a second interaction")
}
