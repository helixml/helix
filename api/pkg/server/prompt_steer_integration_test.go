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
