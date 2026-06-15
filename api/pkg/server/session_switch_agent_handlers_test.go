package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// switchAgentInPlace mutates the SAME session: repoints the agent, clears the
// Zed thread binding, sets AgentSwitchedAt, and seeds a fork_seed + Waiting
// handoff. No new session is created.
func TestSwitchAgentInPlace_MutatesSessionAndSeeds(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	session := newTestParentSession("user_a")
	session.Metadata.ZedThreadID = "ctx_old_thread" // pretend a thread is open
	seedParentWithInteractions(t, mem, session, 2)

	httpErr := srv.switchAgentInPlace(ctx, session, types.CodeAgentRuntimeQwenCode, "app_target")
	require.Nil(t, httpErr)

	updated, err := mem.GetSession(ctx, session.ID)
	require.NoError(t, err)

	// Same session id — no fork.
	assert.Equal(t, session.ID, updated.ID, "switch must keep the same session id")
	// Agent repointed.
	assert.Equal(t, "app_target", updated.ParentApp)
	assert.Equal(t, types.CodeAgentRuntimeQwenCode, updated.Metadata.CodeAgentRuntime)
	assert.Equal(t, types.CodeAgentRuntimeQwenCode.ZedAgentName(), updated.Metadata.ZedAgentName)
	// Thread binding cleared so the next message opens a new thread.
	assert.Equal(t, "", updated.Metadata.ZedThreadID, "ZedThreadID must be cleared")
	// Switch marker set so maybePrependTranscript fires.
	assert.False(t, updated.Metadata.AgentSwitchedAt.IsZero(), "AgentSwitchedAt must be set")
	// Not paused — the session stays live.
	assert.False(t, updated.Metadata.Paused, "in-place switch must not pause the session")

	// A fork_seed (transcript) and a Waiting handoff must exist on THIS session.
	interactions, _, err := mem.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    session.ID,
		GenerationID: session.GenerationID,
		PerPage:      1000,
	})
	require.NoError(t, err)
	var seed, handoff *types.Interaction
	for _, in := range interactions {
		switch in.Trigger {
		case types.InteractionTriggerForkSeed:
			seed = in
		case types.InteractionTriggerForkHandoff:
			handoff = in
		}
	}
	require.NotNil(t, seed, "fork_seed interaction must be created")
	assert.NotEmpty(t, seed.ResponseMessage, "fork_seed must carry the serialized transcript")
	require.NotNil(t, handoff, "handoff interaction must be created")
	assert.Equal(t, types.InteractionStateWaiting, handoff.State, "handoff must be Waiting so pickupWaitingInteraction delivers it on reconnect")
}

// maybePrependTranscript must seed the new thread on an in-place switch (where
// ParentSessionID is empty but AgentSwitchedAt is set).
func TestMaybePrependTranscript_PrependsAfterInPlaceSwitch(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	session := newTestParentSession("user_a")
	session.Metadata.ZedThreadID = "ctx_old_thread"
	seedParentWithInteractions(t, mem, session, 2)

	httpErr := srv.switchAgentInPlace(ctx, session, types.CodeAgentRuntimeQwenCode, "app_target")
	require.Nil(t, httpErr)

	updated, err := mem.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, "", updated.Metadata.ZedThreadID)

	got := srv.maybePrependTranscript(ctx, updated, "continue please")
	assert.NotEqual(t, "continue please", got, "transcript should be prepended after a switch")
	assert.Contains(t, got, "continue please", "the user message must still be present")
}
