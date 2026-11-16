package server

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

// newE2EHarnessServer mirrors the Zed repo's helix-ws-test-server main():
// NewTestServer over an in-memory store with one seeded session whose owner
// has no credential of its own.
func newE2EHarnessServer(t *testing.T) (*HelixAPIServer, *memorystore.MemoryStore, string) {
	t.Helper()
	mem := memorystore.New()
	seed := types.Session{
		ID: "ses_e2e-test-session-001", Name: "E2E Test Seed Session", Created: time.Now(), Updated: time.Now(),
		Owner: "e2e-test-user", Mode: types.SessionModeInference, Type: types.SessionTypeText,
	}
	_, err := mem.CreateSession(context.Background(), seed)
	require.NoError(t, err)
	return NewTestServer(mem, pubsub.NewNoop()), mem, seed.ID
}

// TestE2EHarnessQueuesFirstTurn is the plan lane's phase 101: a raw
// chat_message for a new thread, queued before any thread exists.
func TestE2EHarnessQueuesFirstTurn(t *testing.T) {
	srv, _, sessionID := newE2EHarnessServer(t)
	agent := make(chan types.ExternalAgentCommand, 8)
	srv.externalAgentWSManager.registerConnection(sessionID, &ExternalAgentWSConnection{SessionID: sessionID, SendChan: agent})

	ok := srv.QueueCommand(sessionID, types.ExternalAgentCommand{Type: "chat_message", Data: map[string]interface{}{
		"message": "Publish the deterministic first-turn plan.", "request_id": "req-phase101", "agent_name": "plan-test-agent",
	}})
	require.True(t, ok, "the harness must be able to send the first turn")
	select {
	case cmd := <-agent:
		require.Equal(t, "chat_message", cmd.Type)
		require.Equal(t, "req-phase101", cmd.Data["request_id"])
	case <-time.After(3 * time.Second):
		t.Fatal("the first turn never reached the agent connection")
	}
}

// TestE2EHarnessTurnsCarryHarnessProvenance covers the harness's follow-up
// and queue paths: both record a valid system provenance rather than
// depending on the seeded user's (absent) credential.
func TestE2EHarnessTurnsCarryHarnessProvenance(t *testing.T) {
	srv, mem, sessionID := newE2EHarnessServer(t)
	ctx := context.Background()

	_ = srv.SendChatMessage(sessionID, "What is 7 + 7?", "req-follow-up")
	interactions, _, err := mem.ListInteractions(ctx, &types.ListInteractionsQuery{SessionID: sessionID, PerPage: 10})
	require.NoError(t, err)
	require.Len(t, interactions, 1, "the follow-up turn must create its interaction")
	require.Equal(t, testHarnessTurn, interactions[0].Provenance)

	promptID, err := srv.EnqueueQueuedPrompt(sessionID, "Reply with one sentence.", false)
	require.NoError(t, err, "the harness queue path must not be refused")
	prompt, err := mem.GetPromptHistoryEntry(ctx, promptID)
	require.NoError(t, err)
	require.Equal(t, testHarnessTurn, prompt.Provenance)
}

// TestSendChatMessageRefusesInvalidProvenanceLoudly: a send that cannot
// record who caused it fails with an error and creates nothing, instead of
// dispatching a turn whose reply the caller then waits on forever.
func TestSendChatMessageRefusesInvalidProvenanceLoudly(t *testing.T) {
	srv, mem, sessionID := newE2EHarnessServer(t)
	_, err := srv.sendChatMessageToExternalAgent(sessionID, "hi", "req-bad", false, types.TurnProvenance{})
	require.Error(t, err)
	interactions, _, listErr := mem.ListInteractions(context.Background(), &types.ListInteractionsQuery{SessionID: sessionID, PerPage: 10})
	require.NoError(t, listErr)
	require.Empty(t, interactions)
}
