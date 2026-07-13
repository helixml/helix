package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func callSwitchAgentHTTP(t *testing.T, srv *HelixAPIServer, user *types.User, sessionID string, body SwitchAgentRequest) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/switch-agent", bytes.NewReader(raw))
	req = mux.SetURLVars(req, map[string]string{"id": sessionID})
	req = req.WithContext(setRequestUser(req.Context(), *user))
	rr := httptest.NewRecorder()
	system.Wrapper(srv.switchAgent)(rr, req)
	return rr
}

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

func TestSessionUsesAgentRuntime_RejectsStaleAgentName(t *testing.T) {
	session := newTestParentSession("user_a")
	session.Metadata.CodeAgentRuntime = types.CodeAgentRuntimeCodexCLI
	session.Metadata.ZedAgentName = types.CodeAgentRuntimeClaudeCode.ZedAgentName()

	assert.False(t, sessionUsesAgentRuntime(session, types.CodeAgentRuntimeCodexCLI),
		"a session bound to claude-acp must not be treated as already switched to codex")

	session.Metadata.ZedAgentName = types.CodeAgentRuntimeCodexCLI.ZedAgentName()
	assert.True(t, sessionUsesAgentRuntime(session, types.CodeAgentRuntimeCodexCLI))
}

func TestSwitchAgent_RepairsStaleAgentNameForCurrentApp(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()
	user := &types.User{ID: "user_a", Type: types.OwnerTypeUser}

	mem.SeedApp(&types.App{ID: "app_target", Config: types.AppConfig{Helix: types.AppHelixConfig{
		Assistants: []types.AssistantConfig{{
			AgentType: types.AgentTypeZedExternal, CodeAgentRuntime: types.CodeAgentRuntimeCodexCLI,
		}},
	}}})
	session := newTestParentSession(user.ID)
	session.ParentApp = "app_target"
	session.Metadata.CodeAgentRuntime = types.CodeAgentRuntimeCodexCLI
	session.Metadata.ZedAgentName = types.CodeAgentRuntimeClaudeCode.ZedAgentName()
	session.Metadata.ZedThreadID = "thread_from_claude"
	seedParentWithInteractions(t, mem, session, 1)

	rr := callSwitchAgentHTTP(t, srv, user, session.ID, SwitchAgentRequest{HelixAppID: "app_target"})
	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	updated, err := mem.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, types.CodeAgentRuntimeCodexCLI.ZedAgentName(), updated.Metadata.ZedAgentName)
	assert.Empty(t, updated.Metadata.ZedThreadID)
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

// Regression for the opus→sonnet "model didn't actually switch" bug: getZedConfig
// resolves the claude_code model (managed-settings.json) from specTask.HelixAppID
// FIRST, so an in-place switch MUST repoint the spec task too — not just
// session.ParentApp. This asserts on the resolved config source, which is the
// real switch signal, rather than trusting the agent's self-report (it parrots
// the handoff text and will claim the new model regardless).
func TestSwitchAgentInPlace_RepointsSpecTaskHelixAppID(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()

	mem.SeedApp(&types.App{ID: "app_opus", Config: types.AppConfig{Helix: types.AppHelixConfig{
		Name: "Opus",
		Assistants: []types.AssistantConfig{{
			AgentType: types.AgentTypeZedExternal, CodeAgentRuntime: types.CodeAgentRuntimeClaudeCode, Model: "claude-opus-4-5",
		}},
	}}})
	mem.SeedApp(&types.App{ID: "app_sonnet", Config: types.AppConfig{Helix: types.AppHelixConfig{
		Name: "Sonnet",
		Assistants: []types.AssistantConfig{{
			AgentType: types.AgentTypeZedExternal, CodeAgentRuntime: types.CodeAgentRuntimeClaudeCode, Model: "claude-sonnet-4-5",
		}},
	}}})

	session := newTestParentSession("user_a")
	session.ParentApp = "app_opus"
	session.Metadata.SpecTaskID = "spt_test"
	session.Metadata.CodeAgentRuntime = types.CodeAgentRuntimeClaudeCode
	seedParentWithInteractions(t, mem, session, 1)
	mem.SeedSpecTask(&types.SpecTask{ID: "spt_test", HelixAppID: "app_opus", PlanningSessionID: session.ID})

	httpErr := srv.switchAgentInPlace(ctx, session, types.CodeAgentRuntimeClaudeCode, "app_sonnet")
	require.Nil(t, httpErr)

	updated, err := mem.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, "app_sonnet", updated.ParentApp, "session must repoint to the new app")

	task, err := mem.GetSpecTask(ctx, "spt_test")
	require.NoError(t, err)
	assert.Equal(t, "app_sonnet", task.HelixAppID,
		"spec task HelixAppID must repoint — otherwise getZedConfig keeps resolving the claude_code model from the OLD app and the underlying model never switches")
}
